package render770

import (
	"bytes"
	"testing"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// The ack is a bare VarInt sequence on packet 0x04.
func TestBlockChangedAck(t *testing.T) {
	p := BlockChangedAck(attach.BlockAck{Seq: 300})
	if p.ID != 0x04 {
		t.Fatalf("id = %#x, want 0x04", p.ID)
	}
	r := bytes.NewReader(p.Body)
	seq, err := protocol.ReadVarInt(r)
	if err != nil {
		t.Fatalf("sequence did not decode: %v", err)
	}
	if seq != 300 {
		t.Fatalf("sequence = %d, want 300", seq)
	}
	if r.Len() != 0 {
		t.Fatalf("%d bytes left over", r.Len())
	}
}
