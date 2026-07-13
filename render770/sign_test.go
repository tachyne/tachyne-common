package render770

import (
	"bytes"
	"encoding/binary"
	"testing"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

func testSignText() attach.SignText {
	return attach.SignText{
		X: 100, Y: 65, Z: -200,
		Front: attach.SignSide{Lines: [4]string{"WELCOME", "to", "tachyne", ""}, Color: "red", Glow: true},
		Back:  attach.SignSide{Lines: [4]string{"", "", "", ""}},
		Waxed: true,
	}
}

// reparseSignNBT walks the sign update tag structurally and returns the two
// sides' fields — a strict re-parser for the NBT the composer writes.
type parsedSide struct {
	lines [4]string
	color string
	glow  bool
}

func reparseSignNBT(t *testing.T, br *bytes.Reader) (front, back parsedSide, waxed bool) {
	t.Helper()
	readByte := func() byte {
		b, err := br.ReadByte()
		if err != nil {
			t.Fatalf("nbt truncated: %v", err)
		}
		return b
	}
	readName := func() string {
		var l [2]byte
		if _, err := br.Read(l[:]); err != nil {
			t.Fatalf("nbt name truncated: %v", err)
		}
		n := int(binary.BigEndian.Uint16(l[:]))
		s := make([]byte, n)
		if _, err := br.Read(s); err != nil {
			t.Fatalf("nbt name body truncated: %v", err)
		}
		return string(s)
	}
	if typ := readByte(); typ != 0x0a {
		t.Fatalf("root tag = 0x%x, want compound", typ)
	}
	readSide := func(want string) parsedSide {
		if typ := readByte(); typ != 0x0a {
			t.Fatalf("side tag = 0x%x, want compound", typ)
		}
		if n := readName(); n != want {
			t.Fatalf("side name %q, want %q", n, want)
		}
		var s parsedSide
		if typ := readByte(); typ != 0x09 {
			t.Fatalf("messages tag = 0x%x, want list", typ)
		}
		if n := readName(); n != "messages" {
			t.Fatalf("list name %q", n)
		}
		if et := readByte(); et != 0x08 {
			t.Fatalf("list element type 0x%x, want string", et)
		}
		var cnt [4]byte
		br.Read(cnt[:])
		if c := binary.BigEndian.Uint32(cnt[:]); c != 4 {
			t.Fatalf("messages count %d, want 4", c)
		}
		for i := range s.lines {
			s.lines[i] = readName() // same u16-prefixed layout as names
		}
		if typ := readByte(); typ != 0x08 {
			t.Fatalf("color tag 0x%x", typ)
		}
		if n := readName(); n != "color" {
			t.Fatalf("color name %q", n)
		}
		s.color = readName()
		if typ := readByte(); typ != 0x01 {
			t.Fatalf("glow tag 0x%x", typ)
		}
		if n := readName(); n != "has_glowing_text" {
			t.Fatalf("glow name %q", n)
		}
		s.glow = readByte() != 0
		if end := readByte(); end != 0x00 {
			t.Fatalf("side not closed: 0x%x", end)
		}
		return s
	}
	front = readSide("front_text")
	back = readSide("back_text")
	if typ := readByte(); typ != 0x01 {
		t.Fatalf("is_waxed tag 0x%x", typ)
	}
	if n := readName(); n != "is_waxed" {
		t.Fatalf("is_waxed name %q", n)
	}
	waxed = readByte() != 0
	if end := readByte(); end != 0x00 {
		t.Fatalf("root not closed: 0x%x", end)
	}
	return front, back, waxed
}

func TestSignDataReparse(t *testing.T) {
	e := testSignText()
	pkt := SignData(e)
	if pkt.ID != IDBlockEntityData {
		t.Fatalf("id 0x%x, want 0x%x", pkt.ID, IDBlockEntityData)
	}
	x, y, z := protocol.ReadPosition(pkt.Body[:8])
	if x != 100 || y != 65 || z != -200 {
		t.Fatalf("pos %d,%d,%d", x, y, z)
	}
	br := bytes.NewReader(pkt.Body[8:])
	typ, err := protocol.ReadVarInt(br)
	if err != nil || typ != beTypeSign {
		t.Fatalf("be type %d (err %v), want %d", typ, err, beTypeSign)
	}
	front, back, waxed := reparseSignNBT(t, br)
	if front.lines != [4]string{"WELCOME", "to", "tachyne", ""} || front.color != "red" || !front.glow {
		t.Fatalf("front side wrong: %+v", front)
	}
	if back.lines != [4]string{"", "", "", ""} || back.color != "black" || back.glow {
		t.Fatalf("back side wrong (default color must materialize as black): %+v", back)
	}
	if !waxed {
		t.Fatal("waxed lost")
	}
	if br.Len() != 0 {
		t.Fatalf("%d trailing bytes", br.Len())
	}

	// the hanging variant only changes the block-entity type
	e.Hanging = true
	br2 := bytes.NewReader(SignData(e).Body[8:])
	if typ, _ := protocol.ReadVarInt(br2); typ != beTypeHangingSign {
		t.Fatalf("hanging be type %d, want %d", typ, beTypeHangingSign)
	}
}

func TestSignEditor(t *testing.T) {
	pkt := SignEditor(attach.SignEditor{X: -5, Y: 70, Z: 12, Front: true})
	if pkt.ID != IDOpenSignEditor {
		t.Fatalf("id 0x%x", pkt.ID)
	}
	x, y, z := protocol.ReadPosition(pkt.Body[:8])
	if x != -5 || y != 70 || z != 12 {
		t.Fatalf("pos %d,%d,%d", x, y, z)
	}
	if len(pkt.Body) != 9 || pkt.Body[8] != 1 {
		t.Fatalf("front flag body %v", pkt.Body[8:])
	}
}

func TestParseSignUpdate(t *testing.T) {
	b := protocol.AppendPosition(nil, 100, 65, -200)
	b = append(b, 1)
	for _, s := range []string{"line one", "", "§ccolored", "four"} {
		b = protocol.AppendString(b, s)
	}
	e, ok := ParseSignUpdate(b)
	if !ok {
		t.Fatal("parse failed")
	}
	if e.X != 100 || e.Y != 65 || e.Z != -200 || !e.Front {
		t.Fatalf("header wrong: %+v", e)
	}
	if e.Lines != [4]string{"line one", "", "§ccolored", "four"} {
		t.Fatalf("lines wrong: %v", e.Lines)
	}
	if _, ok := ParseSignUpdate(b[:12]); ok {
		t.Fatal("truncated packet accepted")
	}
}

// TestSignChainTo776 lifts both clientbound sign packets through the full
// 770→776 chain: ids must land on the 26.2 registrations
// (block_entity_data 0x06, open_sign_editor 0x3c — derived from the vanilla
// 26.2 game-protocol registration order) with the bodies untouched, since
// the sign NBT codec is identical through 26.2.
func TestSignChainTo776(t *testing.T) {
	tr := protocol.TranslatorFor(776)
	if tr == nil {
		t.Fatal("no 776 translator")
	}
	data := SignData(testSignText())
	id, body, drop := tr.Clientbound(protocol.StatePlay, data.ID, data.Body)
	if drop || id != 0x06 || !bytes.Equal(body, data.Body) {
		t.Fatalf("block_entity_data: id 0x%x drop %v bodyChanged %v", id, drop, !bytes.Equal(body, data.Body))
	}
	ed := SignEditor(attach.SignEditor{X: 1, Y: 2, Z: 3, Front: false})
	id, body, drop = tr.Clientbound(protocol.StatePlay, ed.ID, ed.Body)
	if drop || id != 0x3c || !bytes.Equal(body, ed.Body) {
		t.Fatalf("open_sign_editor: id 0x%x drop %v", id, drop)
	}
	// serverbound: the 26.2 update_sign (0x3d) must back-translate to
	// canonical 0x3a and still parse.
	sb := protocol.AppendPosition(nil, 1, 2, 3)
	sb = append(sb, 0)
	for i := 0; i < 4; i++ {
		sb = protocol.AppendString(sb, "x")
	}
	id, body, drop = tr.Serverbound(protocol.StatePlay, 0x3d, sb)
	if drop || id != SIDSignUpdate {
		t.Fatalf("update_sign back-translation: id 0x%x drop %v, want 0x%x", id, drop, SIDSignUpdate)
	}
	if _, ok := ParseSignUpdate(body); !ok {
		t.Fatal("back-translated update_sign no longer parses")
	}
}

// TestCampfireDataReparse re-parses the campfire update tag structurally:
// position, type, then the Items list (empties omitted, Slot preserved).
func TestCampfireDataReparse(t *testing.T) {
	pkt := CampfireData(attach.CampfireItems{X: 10, Y: 64, Z: -3,
		Items: [4]string{"minecraft:cod", "", "minecraft:beef", ""}})
	if pkt.ID != IDBlockEntityData {
		t.Fatalf("id 0x%x", pkt.ID)
	}
	x, y, z := protocol.ReadPosition(pkt.Body[:8])
	if x != 10 || y != 64 || z != -3 {
		t.Fatalf("pos %d,%d,%d", x, y, z)
	}
	br := bytes.NewReader(pkt.Body[8:])
	if typ, err := protocol.ReadVarInt(br); err != nil || typ != beTypeCampfire {
		t.Fatalf("be type %d (%v)", typ, err)
	}
	// Root compound → list "Items" (compound elems).
	mustByte := func(want byte, what string) {
		b, _ := br.ReadByte()
		if b != want {
			t.Fatalf("%s: 0x%02x want 0x%02x", what, b, want)
		}
	}
	readStr := func() string {
		var n [2]byte
		br.Read(n[:])
		buf := make([]byte, int(n[0])<<8|int(n[1]))
		br.Read(buf)
		return string(buf)
	}
	mustByte(0x0a, "root compound")
	mustByte(0x09, "list tag")
	if name := readStr(); name != "Items" {
		t.Fatalf("list name %q", name)
	}
	mustByte(0x0a, "list elem type")
	var cnt [4]byte
	br.Read(cnt[:])
	if n := int(cnt[3]); n != 2 {
		t.Fatalf("count %d, want 2 (empties omitted)", n)
	}
	wantSlots := []int8{0, 2}
	wantIDs := []string{"minecraft:cod", "minecraft:beef"}
	for i := 0; i < 2; i++ {
		mustByte(0x01, "Slot tag")
		if readStr() != "Slot" {
			t.Fatal("Slot name")
		}
		s, _ := br.ReadByte()
		if int8(s) != wantSlots[i] {
			t.Fatalf("slot %d want %d", s, wantSlots[i])
		}
		mustByte(0x08, "id tag")
		if readStr() != "id" {
			t.Fatal("id name")
		}
		if got := readStr(); got != wantIDs[i] {
			t.Fatalf("id %q want %q", got, wantIDs[i])
		}
		mustByte(0x03, "count tag")
		if readStr() != "count" {
			t.Fatal("count name")
		}
		var c [4]byte
		br.Read(c[:])
		if c[3] != 1 {
			t.Fatalf("count %d", c[3])
		}
		mustByte(0x00, "entry end")
	}
	mustByte(0x00, "root end")
	if br.Len() != 0 {
		t.Fatalf("%d trailing bytes", br.Len())
	}
}
