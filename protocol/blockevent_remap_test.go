package protocol

import (
	"bytes"
	"testing"
)

// The trailing block id of block_event takes the client's block numbering;
// the position and the two action bytes are untouched.
func TestBlockEventRemapsBlockID(t *testing.T) {
	body := AppendPosition(nil, 1, 2, 3)
	body = append(body, 1, 3)
	const bell = 700 // any id: we only check the remap was applied
	body = AppendVarInt(body, bell)
	out := remapClientboundIDs(776, canonBlockEvent, body)
	if !bytes.Equal(out[:10], body[:10]) {
		t.Fatal("position/action bytes changed")
	}
	r := bytes.NewReader(out[10:])
	id, _ := ReadVarInt(r)
	if want := RemapID(RegBlock, 776, bell); id != want {
		t.Errorf("block id %d, want %d", id, want)
	}
	// 1.21.5 clients renumber blocks too (the canonical registry is 1.21.11's).
	got := remapClientboundIDs(770, canonBlockEvent, body)
	r = bytes.NewReader(got[10:])
	if id, _ := ReadVarInt(r); id != RemapID(RegBlock, 770, bell) {
		t.Errorf("770 block id %d, want %d", id, RemapID(RegBlock, 770, bell))
	}
}
