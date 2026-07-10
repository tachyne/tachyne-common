package protocol

import "testing"

// TestUnmapWindowClick776 proves a 26.2 client's click (item ids in the 776
// space) is rewritten to canonical 770 ids, byte-exact around the rewrite.
func TestUnmapWindowClick776(t *testing.T) {
	// Find an item whose id actually shifts between 770 and 776.
	var canon, shifted int32 = -1, -1
	for id := int32(0); id < 2000; id++ {
		if m := RemapID(RegItem, 776, id); m != id {
			canon, shifted = id, m
			break
		}
	}
	if canon < 0 {
		t.Skip("no shifting item id in the 776 table")
	}

	// window 5, state 9, slot 1, button 0, mode 0, one changed slot carrying the
	// SHIFTED (client-space) id, empty cursor.
	b := AppendVarInt(nil, 5)
	b = AppendVarInt(b, 9)
	b = append(b, 0, 1, 0) // slot i16 + button
	b = AppendVarInt(b, 0) // mode
	b = AppendVarInt(b, 1) // changed count
	b = append(b, 0, 1)    // location i16
	b = append(b, 1)       // present
	b = AppendVarInt(b, shifted)
	b = AppendVarInt(b, 3) // count
	b = AppendVarInt(b, 0) // components added
	b = AppendVarInt(b, 0) // components removed
	b = append(b, 0)       // cursor absent

	out := unmapWindowClick(776, b)
	// Re-read the changed slot's item id from the rewritten body.
	want := AppendVarInt(nil, 5)
	want = AppendVarInt(want, 9)
	want = append(want, 0, 1, 0)
	want = AppendVarInt(want, 0)
	want = AppendVarInt(want, 1)
	want = append(want, 0, 1)
	want = append(want, 1)
	want = AppendVarInt(want, canon)
	want = AppendVarInt(want, 3)
	want = AppendVarInt(want, 0)
	want = AppendVarInt(want, 0)
	want = append(want, 0)
	if string(out) != string(want) {
		t.Fatalf("unmapWindowClick mismatch:\n got %v\nwant %v", out, want)
	}
}
