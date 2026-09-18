package render770

import (
	"bytes"
	"testing"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

func TestContainerCloseBytes(t *testing.T) {
	p := ContainerClose(attach.WindowCloseServer{ID: 7})
	if want := protocol.AppendVarInt(nil, 7); p.ID != IDContainerClose || !bytes.Equal(p.Body, want) {
		t.Fatalf("id %#x body %x want %x", p.ID, p.Body, want)
	}
}
