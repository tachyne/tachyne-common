package protocol

import "testing"

func TestBlockEntityType(t *testing.T) {
	// Bed (red_bed 1955..1970) and chest (3786..3809, 1.21.11) render via a block
	// entity. Type ids are vanilla REGISTRATION order (ViaVersion mappings): bed=25,
	// chest=1 — NOT mcmeta's alphabetical position (that ordering bug shipped once).
	if typ, ok := BlockEntityType(1955); !ok || typ != 25 {
		t.Errorf("red_bed should be block_entity_type bed(25): typ=%d ok=%v", typ, ok)
	}
	if typ, ok := BlockEntityType(3786); !ok || typ != 1 {
		t.Errorf("chest should be block_entity_type chest(1): typ=%d ok=%v", typ, ok)
	}
	// Plain stone (1) has none.
	if _, ok := BlockEntityType(1); ok {
		t.Error("stone should not have a block entity")
	}
}
