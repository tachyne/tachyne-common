package render770

import (
	"bytes"
	"testing"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// block_event: position, action, param, block id — a bell struck from the
// south (direction id 3), block id 42.
func TestBlockEventLayout(t *testing.T) {
	p := BlockEvent(attach.BlockEvent{X: 5, Y: 64, Z: -9, Action: 1, Param: 3, Block: 42})
	if p.ID != IDBlockEvent {
		t.Fatalf("packet id %#x, want %#x", p.ID, IDBlockEvent)
	}
	want := protocol.AppendPosition(nil, 5, 64, -9)
	want = append(want, 1, 3)
	want = protocol.AppendVarInt(want, 42)
	if !bytes.Equal(p.Body, want) {
		t.Fatalf("body %x, want %x", p.Body, want)
	}
}
