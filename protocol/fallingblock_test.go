package protocol

import (
	"bytes"
	"testing"
)

// A falling block's spawn carries its block state as objectData; a 26.2
// client's state ids differ, so the data is remapped with the type.
func TestFallingBlockSpawnRemapsItsState(t *testing.T) {
	state := int32(-1)
	for id := int32(1); id < 30000; id++ {
		if RemapID(RegBlockState, 776, id) != id {
			state = id
			break
		}
	}
	if state < 0 {
		t.Skip("no state shifts at 776")
	}
	spawn := func(typ, data int32) []byte {
		b := AppendVarInt(nil, 7)          // entity id
		b = append(b, make([]byte, 16)...) // uuid
		b = AppendVarInt(b, typ)
		b = append(b, make([]byte, 24)...) // x, y, z
		b = append(b, 1, 2, 3)             // angles
		b = AppendVarInt(b, data)
		return append(b, 0, 0, 0, 0, 0, 0) // velocity
	}
	out := remapSpawnEntityType(776, spawn(fallingBlockEntity, state))
	want := spawn(RemapID(RegEntity, 776, fallingBlockEntity), RemapID(RegBlockState, 776, state))
	if !bytes.Equal(out, want) {
		t.Fatalf("falling block spawn %x, want %x", out, want)
	}
	other := CanonicalEntity("snowball")
	if got := remapSpawnEntityType(776, spawn(other, state)); !bytes.Equal(got, spawn(RemapID(RegEntity, 776, other), state)) {
		t.Fatalf("a snowball's data was touched: %x", got)
	}
}
