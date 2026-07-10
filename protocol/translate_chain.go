package protocol

// Chained, data-driven version translation. The canonical core speaks Target
// (770); a client several versions newer is reached by composing single-version
// "steps", each renumbering packet IDs (and, where field layouts changed, running
// a body rewriter) between two adjacent protocol versions. This mirrors how
// ViaVersion stacks per-version protocols.
//
//   - protoSteps (generated in protomap_gen.go) supplies the ID remap per step.
//   - stepBody (hand-written, below) supplies body rewriters for the few packets
//     whose field layout changed in a step. Pure-ID-remap steps need none.
//
// A clientbound packet is produced at canonical 770 and walked UP through the
// steps to the client's version; a serverbound packet arrives at the client's
// version and is walked DOWN to canonical.

// MaxTranslated is the highest client protocol version the chain currently serves
// end to end. It is raised toward 776 as each step's body rewriters are implemented
// and verified. Versions above it are rejected at login (clear message) rather than
// served a half-translated stream. 770→774 is machine-derived (minecraft-data);
// 774→775→776 (26.1/26.2) is derived from ViaVersion (packet IDs + registry deltas)
// plus the chunk fluid-count and Join-boolean rewriters — so 1.21.5 through 26.2
// (proto 776) are served. 26.x is newly wired and may need iteration against a
// real client.
const MaxTranslated = 776

// idRemap is one step's packet-ID remap between adjacent versions lower→upper.
// Maps contain only IDs that differ (identity otherwise). Field shape MUST match
// the literals emitted by gen_protomap.py.
type idRemap struct {
	lower, upper int32
	cbUp         map[State]map[int32]int32 // clientbound: lower id → upper id
	cbDrop       map[State]map[int32]bool  // clientbound lower ids with no upper equivalent
	sbDown       map[State]map[int32]int32 // serverbound: upper id → lower id
	sbDrop       map[State]map[int32]bool  // serverbound upper ids with no lower equivalent
}

// bodyFn rewrites a packet body between two adjacent versions. It must not retain
// the input slice; return a fresh slice when the bytes change, else the input.
type bodyFn func(state State, body []byte) []byte

// bodyRewriters holds a step's body rewriters, keyed by the LOWER (canonical-side)
// packet ID so they compose consistently regardless of ID renumbering.
type bodyRewriters struct {
	cbUp   map[State]map[int32]bodyFn // clientbound lower→upper body change
	sbDown map[State]map[int32]bodyFn // serverbound upper→lower body change
}

// stepBody is populated by per-step files (e.g. v773.go) for steps that change
// field layouts. Absent = the step is a pure ID remap. Keyed by upper version.
var stepBody = map[int32]bodyRewriters{}

// up converts a clientbound packet from this step's lower version to its upper:
// rewrite the body (still in lower form), then renumber the ID.
func (s idRemap) up(state State, id int32, body []byte) (int32, []byte, bool) {
	if s.cbDrop[state][id] {
		return id, body, true
	}
	if rw := stepBody[s.upper].cbUp[state][id]; rw != nil {
		body = rw(state, body)
	}
	if nid, ok := s.cbUp[state][id]; ok {
		id = nid
	}
	return id, body, false
}

// down converts a serverbound packet from this step's upper version to its lower:
// renumber the ID to canonical-side, then rewrite the body to lower form.
func (s idRemap) down(state State, id int32, body []byte) (int32, []byte, bool) {
	if s.sbDrop[state][id] {
		return id, body, true
	}
	lid := id
	if x, ok := s.sbDown[state][id]; ok {
		lid = x
	}
	if rw := stepBody[s.upper].sbDown[state][lid]; rw != nil {
		body = rw(state, body)
	}
	return lid, body, false
}

// chainTranslator composes ordered steps (low→high) into one Translator.
type chainTranslator struct {
	version int32
	steps   []idRemap // ordered Target→version
}

func (c chainTranslator) Version() int32 { return c.version }

func (c chainTranslator) Clientbound(state State, id int32, body []byte) (int32, []byte, bool) {
	// Block-state IDs first, at canonical (770) packet IDs, before the per-step
	// renumbering. Newer versions shifted block-state IDs, so chunk palettes and
	// block updates must be translated or the client renders the wrong blocks.
	if state == StatePlay {
		body = remapClientboundIDs(c.version, id, body)
	}
	for _, s := range c.steps { // canonical → client version
		var drop bool
		if id, body, drop = s.up(state, id, body); drop {
			return id, body, true
		}
	}
	return id, body, false
}

func (c chainTranslator) Serverbound(state State, id int32, body []byte) (int32, []byte, bool) {
	// 26.x (proto ≥775) split use_entity into separate Attack and Interact packets.
	// Fold them back into the canonical use_entity here, before the chained id remap,
	// so player→mob combat works on 26.x clients (left-click attacks).
	if state == StatePlay && c.version >= 775 {
		if nid, nbody, ok := rewrite26xEntity(id, body); ok {
			return nid, nbody, false
		}
	}
	for i := len(c.steps) - 1; i >= 0; i-- { // client version → canonical
		var drop bool
		if id, body, drop = c.steps[i].down(state, id, body); drop {
			return id, body, true
		}
	}
	// id is now canonical; translate any client-version registry IDs in the body
	// back to canonical (e.g. the item ID in a creative-mode slot).
	if state == StatePlay {
		body = unmapServerboundIDs(c.version, id, body)
	}
	return id, body, false
}

// chainFor builds the translator for a client version by composing the LAYOUT
// steps from Target(770)+1 up to version (a version==Target client gets zero
// steps but still runs the id-remap in Clientbound/Serverbound). Returns nil if
// version is outside the supported range or any step is missing (a gap we can't
// bridge). Layout base is 770; id canonical is 1.21.11(774) via translationTables.
func chainFor(version int32) Translator {
	if version < Target || version > MaxTranslated {
		return nil
	}
	var steps []idRemap
	for v := int32(Target) + 1; v <= version; v++ {
		s, ok := protoSteps[v]
		if !ok {
			return nil
		}
		steps = append(steps, s)
	}
	return chainTranslator{version: version, steps: steps}
}
