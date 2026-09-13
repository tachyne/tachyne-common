package render770

import (
	"bytes"
	"testing"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

func TestBlockDestructionBytes(t *testing.T) {
	p := BlockDestruction(attach.BlockBreakProgress{EID: 9, X: 1, Y: 64, Z: -3, Progress: 4})
	want := protocol.AppendVarInt(nil, 9)
	want = protocol.AppendPosition(want, 1, 64, -3)
	want = protocol.AppendU8(want, 4)
	if p.ID != IDBlockDestruction || !bytes.Equal(p.Body, want) {
		t.Fatalf("id %#x body %x want %x", p.ID, p.Body, want)
	}
	if c := BlockDestruction(attach.BlockBreakProgress{EID: 9, Progress: -1}); c.Body[len(c.Body)-1] != 0xff {
		t.Fatal("clearing writes -1")
	}
}
