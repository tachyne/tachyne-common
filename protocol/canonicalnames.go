package protocol

// Canonical ids by name, from canonicalnames_gen.go. Code and tests that need
// a particular entity, item or block name it here: numbers written in by hand
// silently re-point at something else whenever the canonical version moves.
// Each panics on an unknown name — naming something the canonical registry
// lacks is a bug, not a lookup miss.

// CanonicalEntity is the canonical id of the named entity type (no namespace).
func CanonicalEntity(name string) int32 {
	id, ok := canonicalEntityIDs[name]
	if !ok {
		panic("protocol.CanonicalEntity: no entity " + name)
	}
	return id
}

// CanonicalItem is the canonical id of the named item.
func CanonicalItem(name string) int32 {
	id, ok := canonicalItemIDs[name]
	if !ok {
		panic("protocol.CanonicalItem: no item " + name)
	}
	return id
}

// CanonicalBlockState is the canonical default state of the named block.
func CanonicalBlockState(name string) int32 {
	id, ok := canonicalBlockStates[name]
	if !ok {
		panic("protocol.CanonicalBlockState: no block " + name)
	}
	return id
}

// ClientEntity is the named entity type's id in a 26.2 or 26.3 client's own
// numbering (the registries those clients are sent), for checking that a
// canonical id lands on the entity it should.
func ClientEntity(version int32, name string) (int32, bool) {
	var m map[string]int32
	switch {
	case version >= 777:
		m = entity263ID
	case version == 776:
		m = entity26xID
	default:
		return 0, false
	}
	id, ok := m["minecraft:"+name]
	if !ok {
		id, ok = m[name]
	}
	return id, ok
}

// ClientItems is a 26.2 or 26.3 client's own item ids by name ("minecraft:"
// prefixed), or nil for any other version.
func ClientItems(version int32) map[string]int32 {
	switch {
	case version >= 777:
		return item263ID
	case version == 776:
		return item26xID
	}
	return nil
}
