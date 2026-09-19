package protocol

import (
	"bytes"
	"testing"
)

// A decorated shield's base_color renumbers with the client version (64 →
// 71 at 1.21.11 → 73 at 26.2) and its dye enum rides through unchanged;
// serverbound the client's id maps back to canonical.
func TestBaseColorComponentRenumbers(t *testing.T) {
	slot := AppendVarInt(nil, 1) // count
	slot = AppendVarInt(slot, 5) // some item
	slot = AppendVarInt(slot, 1) // one component added
	slot = AppendVarInt(slot, 0) // none removed
	slot = AppendVarInt(slot, componentBaseColor)
	slot = AppendVarInt(slot, 14) // red
	for _, tc := range []struct {
		version int32
		want    int32
	}{{770, 64}, {772, 64}, {774, 71}, {775, 73}, {776, 73}} {
		var out []byte
		if !copyFullSlot(bytes.NewReader(slot), &out, func(id int32) int32 { return id }, tc.version, false) {
			t.Fatalf("v%d: walker refused the shield slot", tc.version)
		}
		r := bytes.NewReader(out)
		ReadVarInt(r) // count
		ReadVarInt(r) // item
		ReadVarInt(r) // added
		ReadVarInt(r) // removed
		if id, _ := ReadVarInt(r); id != tc.want {
			t.Errorf("v%d: base_color id = %d, want %d", tc.version, id, tc.want)
		}
		if dye, _ := ReadVarInt(r); dye != 14 {
			t.Errorf("v%d: dye = %d, want 14", tc.version, dye)
		}
	}
	sb := AppendVarInt(nil, 1)
	sb = AppendVarInt(sb, 5)
	sb = AppendVarInt(sb, 1)
	sb = AppendVarInt(sb, 0)
	sb = AppendVarInt(sb, 73)
	sb = AppendVarInt(sb, 14)
	var out []byte
	if !copyFullSlot(bytes.NewReader(sb), &out, func(id int32) int32 { return id }, 776, true) {
		t.Fatal("serverbound shield slot refused")
	}
	r := bytes.NewReader(out)
	ReadVarInt(r)
	ReadVarInt(r)
	ReadVarInt(r)
	ReadVarInt(r)
	if id, _ := ReadVarInt(r); id != componentBaseColor {
		t.Errorf("serverbound base_color id = %d, want %d", id, componentBaseColor)
	}
}
