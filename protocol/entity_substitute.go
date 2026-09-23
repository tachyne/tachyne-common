package protocol

// Backward entity-type substitution — the ViaBackwards `mapEntityTypeWithData`
// idea, reduced to what our engine needs.
//
// A client older than the
// version that introduced an entity has NO id for it, so the pure range-shift
// in translationTables[RegEntity] would map the new id onto whatever unrelated
// entity happens to sit at that numeric slot in the old registry — a wrong
// mob, and (because entity metadata is decoded against the wrong type) a likely
// client disconnect. ViaBackwards avoids this by substituting the nearest
// existing older entity ("render a Happy Ghast as a Ghast") instead of shifting.
//
// We do the same: for a client whose protocol predates an entity, emit a
// hand-picked FALLBACK that exists downstream. The fallback is itself a
// canonical id, so it range-maps to the client's numbering through the normal
// table — we reuse the existing machinery and only override the id being fed
// into it.
//
// Fallbacks follow ViaBackwards' rule: the closest vanilla mob with the right
// body-plan and movement. The fallbacks below are exactly ViaBackwards' choices
// where it has them (happy_ghast→ghast, copper_golem→frog, nautilus→squid,
// zombie_nautilus→glow_squid, camel_husk→camel, parched→skeleton), and a
// simple static stand-in for the mannequin (ViaBackwards emulates it as a fake
// player, which we do not attempt). All fallbacks exist in 1.21.5 (770), so the
// direct 774→770 hop is safe.
//
// This only prevents the OLD-client crash; a 26.2 client (proto ≥ the entity's
// introduction) receives the real entity untouched. Metadata sanitizing for a
// substituted entity (dropping indices the old renderer can't parse) is a
// stateful, per-eid gateway concern — see docs/VIABACKWARDS.md §"entity guard";
// our engine sidesteps most of it by emitting only baseline metadata for these
// mobs.
type entitySub struct {
	AddedProto int32 // first protocol version that has this entity natively
	Fallback   int32 // canonical id to render as for older clients
}

// entityStandIns names each entity introduced after 1.21.5 that the engine
// can emit, the first protocol that has it, and what an older client gets
// instead. Names, not ids: resolved against the canonical registry at start
// (canonicalEntityIDs), so a canonical version change cannot re-point them.
var entityStandIns = []struct {
	Entity     string
	AddedProto int32
	Fallback   string
}{
	{"happy_ghast", 771, "ghast"},                // 1.21.6
	{"copper_golem", 773, "frog"},                // 1.21.9
	{"mannequin", 773, "armor_stand"},            // 1.21.9
	{"camel_husk", 774, "camel"},                 // 1.21.11
	{"nautilus", 774, "squid"},                   // 1.21.11
	{"zombie_nautilus", 774, "glow_squid"},       // 1.21.11
	{"parched", 774, "skeleton"},                 // 1.21.11
	{"poplar_boat", 777, "oak_boat"},             // 26.3
	{"poplar_chest_boat", 777, "oak_chest_boat"}, // 26.3
	{"cushion", 777, "armor_stand"},              // 26.3, a static stand-in as for the mannequin
}

// entitySubstitutions is keyed by canonical entity-type id. An entity the
// canonical registry does not have yet is skipped: nothing can emit it.
var entitySubstitutions = func() map[int32]entitySub {
	m := map[int32]entitySub{}
	for _, s := range entityStandIns {
		id, ok := canonicalEntityIDs[s.Entity]
		if !ok {
			continue
		}
		fb, ok := canonicalEntityIDs[s.Fallback]
		if !ok {
			panic("entity_substitute: stand-in " + s.Fallback + " is not a canonical entity")
		}
		m[id] = entitySub{AddedProto: s.AddedProto, Fallback: fb}
	}
	return m
}()

// CanonicalEntity is the canonical id of the named entity type (no
// namespace). It panics on an unknown name: a table naming an entity the
// canonical registry lacks is a bug, not a lookup miss.
func CanonicalEntity(name string) int32 {
	id, ok := canonicalEntityIDs[name]
	if !ok {
		panic("protocol.CanonicalEntity: no entity " + name)
	}
	return id
}

// substituteEntityType returns the canonical entity-type id to actually feed to
// the range-shift for a given client version: the fallback when the client
// predates the entity, else the id unchanged.
func substituteEntityType(version, canonical int32) int32 {
	if s, ok := entitySubstitutions[canonical]; ok && version < s.AddedProto {
		return s.Fallback
	}
	return canonical
}

// IsSubstituted reports whether a canonical entity type is rendered as a
// different stand-in for this client version (i.e. the client predates it). A
// gateway uses this to sanitize entity metadata: the substitute (e.g. a Ghast
// standing in for a Happy Ghast) has a different metadata schema, so
// type-specific entity-data for the original must be dropped rather than decoded
// against the wrong type. Baseline/shared indices below the entity-specific
// range are fine, but the safe default is to drop the metadata entirely.
func IsSubstituted(version, canonical int32) bool {
	s, ok := entitySubstitutions[canonical]
	return ok && version < s.AddedProto
}
