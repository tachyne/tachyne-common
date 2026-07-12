package render770

import (
	"bytes"
	"io"
	"testing"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// Re-parse the composed map_item_data byte-for-byte against the 770 layout
// (varint id, i8 scale, bool locked, option decoration array, then the
// color patch behind the columns!=0 sentinel).
func TestMapItemDataReparse(t *testing.T) {
	pkt := MapItemData(attach.MapData{
		MapID: 7, Scale: 2, Locked: true,
		HasDecor: true,
		Decor: []attach.MapDecoration{
			{Type: 0, X: -10, Z: 20, Rot: 5},
			{Type: 1, X: 3, Z: -4, Rot: 19, Name: "Home"}, // rot masks to 3
		},
		X: 16, Y: 32, Width: 2, Height: 3,
		Colors: []byte{1, 2, 3, 4, 5, 6},
	})
	if pkt.ID != IDMapItemData {
		t.Fatalf("packet id %#x", pkt.ID)
	}
	r := bytes.NewReader(pkt.Body)
	if id, _ := protocol.ReadVarInt(r); id != 7 {
		t.Fatalf("map id %d", id)
	}
	scale, _ := r.ReadByte()
	locked, _ := r.ReadByte()
	if scale != 2 || locked != 1 {
		t.Fatalf("scale %d locked %d", scale, locked)
	}
	if present, _ := r.ReadByte(); present != 1 {
		t.Fatal("decorations absent")
	}
	if n, _ := protocol.ReadVarInt(r); n != 2 {
		t.Fatalf("decoration count %d", n)
	}
	// First decoration: player marker, no name.
	if typ, _ := protocol.ReadVarInt(r); typ != 0 {
		t.Fatalf("decor[0] type %d", typ)
	}
	x, _ := r.ReadByte()
	z, _ := r.ReadByte()
	rot, _ := r.ReadByte()
	hasName, _ := r.ReadByte()
	if int8(x) != -10 || int8(z) != 20 || rot != 5 || hasName != 0 {
		t.Fatalf("decor[0] %d %d %d %d", int8(x), int8(z), rot, hasName)
	}
	// Second: frame marker with a name, rotation masked to 0-15.
	if typ, _ := protocol.ReadVarInt(r); typ != 1 {
		t.Fatalf("decor[1] type %d", typ)
	}
	x, _ = r.ReadByte()
	z, _ = r.ReadByte()
	rot, _ = r.ReadByte()
	if int8(x) != 3 || int8(z) != -4 || rot != 3 {
		t.Fatalf("decor[1] %d %d %d", int8(x), int8(z), rot)
	}
	if hasName, _ = r.ReadByte(); hasName != 1 {
		t.Fatal("decor[1] name missing")
	}
	tag, _ := r.ReadByte()
	if tag != 0x08 {
		t.Fatalf("name tag %#x", tag)
	}
	hi, _ := r.ReadByte()
	lo, _ := r.ReadByte()
	name := make([]byte, int(hi)<<8|int(lo))
	io.ReadFull(r, name)
	if string(name) != "Home" {
		t.Fatalf("name %q", name)
	}
	// Patch: columns, rows, x, y, then the buffer.
	cols, _ := r.ReadByte()
	rows, _ := r.ReadByte()
	px, _ := r.ReadByte()
	py, _ := r.ReadByte()
	if cols != 2 || rows != 3 || px != 16 || py != 32 {
		t.Fatalf("patch %d %d %d %d", cols, rows, px, py)
	}
	n, _ := protocol.ReadVarInt(r)
	buf := make([]byte, n)
	io.ReadFull(r, buf)
	if !bytes.Equal(buf, []byte{1, 2, 3, 4, 5, 6}) {
		t.Fatalf("patch data %v", buf)
	}
	if r.Len() != 0 {
		t.Fatalf("%d trailing bytes", r.Len())
	}
}

// A decorations-only update ends right after the columns==0 sentinel, and
// an empty-but-present decoration list clears the client's markers.
func TestMapItemDataDecorOnly(t *testing.T) {
	pkt := MapItemData(attach.MapData{MapID: 3, HasDecor: true})
	r := bytes.NewReader(pkt.Body)
	protocol.ReadVarInt(r) // id
	r.ReadByte()           // scale
	r.ReadByte()           // locked
	present, _ := r.ReadByte()
	n, _ := protocol.ReadVarInt(r)
	if present != 1 || n != 0 {
		t.Fatalf("decor present=%d n=%d", present, n)
	}
	cols, _ := r.ReadByte()
	if cols != 0 || r.Len() != 0 {
		t.Fatalf("cols %d, %d trailing", cols, r.Len())
	}

	// And a patch-only update carries no decoration flag payload.
	pkt = MapItemData(attach.MapData{MapID: 3, X: 1, Y: 2, Width: 1, Height: 1, Colors: []byte{9}})
	r = bytes.NewReader(pkt.Body)
	protocol.ReadVarInt(r)
	r.ReadByte()
	r.ReadByte()
	if present, _ := r.ReadByte(); present != 0 {
		t.Fatal("decorations should be absent")
	}
	if cols, _ := r.ReadByte(); cols != 1 {
		t.Fatalf("cols %d", cols)
	}
}
