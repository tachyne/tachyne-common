package protocol

import "testing"

// TestCreativeSlotChain776Repro drives a 26.2 client's set_creative_mode_slot
// for the "match" painting preset through the REAL serverbound chain, exactly
// as gwsession would, and prints what reaches the canonical parser.
func TestCreativeSlotChain776Repro(t *testing.T) {
	matchIdx := PaintingVariantIndex("match")
	body := []byte{0, 36}                 // slot 36 (i16)
	body = AppendVarInt(body, 1)          // count
	body = AppendVarInt(body, 1013)       // 26.2 painting item id
	body = AppendVarInt(body, 1)          // components added
	body = AppendVarInt(body, 0)          // components removed
	body = AppendVarInt(body, 103)        // minecraft:painting/variant @ 26.2
	body = AppendVarInt(body, matchIdx+1) // Holder reference
	tr := TranslatorFor(776)
	id, out, drop := tr.Serverbound(StatePlay, 0x38, body)
	t.Logf("match index=%d", matchIdx)
	t.Logf("in : id=0x38 body=% x", body)
	t.Logf("out: id=0x%x drop=%v body=% x", id, drop, out)
	if id != 0x36 {
		t.Fatalf("canonical id 0x%x, want 0x36 (set_creative_mode_slot)", id)
	}
}
