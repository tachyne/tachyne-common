package render770

import (
	"bytes"
	"testing"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

func TestWaypointBody(t *testing.T) {
	uuid := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	// UPDATE with a position, no color.
	body := WaypointBody(attach.Waypoint{Op: 2, UUID: uuid, X: 100, Y: 64, Z: -200})
	br := bytes.NewReader(body)
	if op, _ := protocol.ReadVarInt(br); op != 2 {
		t.Fatalf("op %d", op)
	}
	if b, _ := br.ReadByte(); b != 1 {
		t.Fatal("expected Either.left tag")
	}
	got := make([]byte, 16)
	br.Read(got)
	if !bytes.Equal(got, uuid[:]) {
		t.Fatal("uuid mismatch")
	}
	if s, _ := protocol.ReadString(br); s != "minecraft:default" {
		t.Fatalf("style %q", s)
	}
	if c, _ := br.ReadByte(); c != 0 {
		t.Fatal("expected no color")
	}
	if ty, _ := protocol.ReadVarInt(br); ty != 1 {
		t.Fatalf("type %d, want VEC3I", ty)
	}
	x, _ := protocol.ReadVarInt(br)
	y, _ := protocol.ReadVarInt(br)
	z, _ := protocol.ReadVarInt(br)
	if x != 100 || y != 64 || z != -200 {
		t.Fatalf("pos %d,%d,%d", x, y, z)
	}
	if br.Len() != 0 {
		t.Fatalf("%d trailing", br.Len())
	}

	// UNTRACK is empty (type 0, no contents).
	u := WaypointBody(attach.Waypoint{Op: 1, UUID: uuid})
	ur := bytes.NewReader(u)
	protocol.ReadVarInt(ur) // op
	ur.ReadByte()           // tag
	ur.Read(make([]byte, 16))
	protocol.ReadString(ur) // style
	ur.ReadByte()           // color present
	if ty, _ := protocol.ReadVarInt(ur); ty != 0 {
		t.Fatalf("untrack type %d, want EMPTY", ty)
	}
	if ur.Len() != 0 {
		t.Fatalf("untrack %d trailing", ur.Len())
	}
}
