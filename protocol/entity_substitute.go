package protocol

// Backward entity-type substitution — the ViaBackwards `mapEntityTypeWithData`
// idea, reduced to what our engine needs.
//
// The canonical id space is 1.21.11 (proto 774). A client older than the
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
	Fallback   int32 // canonical (1.21.11) id to render as for older clients
}

// entitySubstitutions is keyed by canonical (1.21.11) entity-type id. Add an
// entry for every entity introduced after 1.21.5 that the engine can emit.
var entitySubstitutions = map[int32]entitySub{
	58:  {AddedProto: 771, Fallback: 57},  // happy_ghast  -> ghast        (1.21.6)
	28:  {AddedProto: 773, Fallback: 55},  // copper_golem -> frog         (1.21.9)
	83:  {AddedProto: 773, Fallback: 5},   // mannequin    -> armor_stand  (1.21.9)
	20:  {AddedProto: 774, Fallback: 19},  // camel_husk   -> camel        (1.21.11)
	88:  {AddedProto: 774, Fallback: 127}, // nautilus     -> squid        (1.21.11)
	152: {AddedProto: 774, Fallback: 61},  // zombie_nautilus -> glow_squid (1.21.11)
	97:  {AddedProto: 774, Fallback: 115}, // parched      -> skeleton     (1.21.11)
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
