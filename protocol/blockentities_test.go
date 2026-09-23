package protocol

import "testing"

func TestBlockEntityType(t *testing.T) {
	// A chest and a shulker box render via a block entity. Type ids are vanilla
	// REGISTRATION order: chest=1, shulker_box=24, in every version served —
	// NOT mcmeta's alphabetical position (that ordering bug shipped once).
	// (Not a bed: 26.x has no bed block entity.)
	if typ, ok := BlockEntityType(uint32(CanonicalBlockState("chest"))); !ok || typ != 1 {
		t.Errorf("chest should be block_entity_type chest(1): typ=%d ok=%v", typ, ok)
	}
	if typ, ok := BlockEntityType(uint32(CanonicalBlockState("shulker_box"))); !ok || typ != 24 {
		t.Errorf("shulker_box should be block_entity_type shulker_box(24): typ=%d ok=%v", typ, ok)
	}
	// Plain stone has none.
	if _, ok := BlockEntityType(uint32(CanonicalBlockState("stone"))); ok {
		t.Error("stone should not have a block entity")
	}
}
