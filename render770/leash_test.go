package render770

import (
	"bytes"
	"encoding/binary"
	"testing"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// readI32BE decodes independently of the writer's helpers, so this is an
// oracle for the layout rather than a round trip through the same code.
func readI32BE(t *testing.T, r *bytes.Reader) int32 {
	t.Helper()
	var b [4]byte
	if _, err := r.Read(b[:]); err != nil {
		t.Fatalf("short read: %v", err)
	}
	return int32(binary.BigEndian.Uint32(b[:]))
}

// set_entity_link is two big-endian int32s: the leashed entity, then its
// holder. Exactly eight bytes, no VarInts anywhere.
func TestEntityLinkLayout(t *testing.T) {
	pkt := EntityLink(attach.EntityLink{Leashed: 4242, Holder: 7})
	if pkt.ID != IDSetEntityLink {
		t.Fatalf("packet id %#x, want %#x", pkt.ID, IDSetEntityLink)
	}
	if len(pkt.Body) != 8 {
		t.Fatalf("body %d bytes, want 8 (two int32s): % x", len(pkt.Body), pkt.Body)
	}
	r := bytes.NewReader(pkt.Body)
	if got := readI32BE(t, r); got != 4242 {
		t.Errorf("source = %d, want the LEASHED entity 4242", got)
	}
	if got := readI32BE(t, r); got != 7 {
		t.Errorf("dest = %d, want the HOLDER 7", got)
	}
	if r.Len() != 0 {
		t.Errorf("%d bytes left over", r.Len())
	}
}

// A big entity id must survive intact. A VarInt here would encode 4242 in two
// bytes and desynchronise every packet after it, so this pins the width.
func TestEntityLinkUsesFixedWidthInts(t *testing.T) {
	pkt := EntityLink(attach.EntityLink{Leashed: 1, Holder: 2})
	if len(pkt.Body) != 8 {
		t.Fatalf("small ids encoded in %d bytes; the fields are fixed-width, not VarInt", len(pkt.Body))
	}
	want := []byte{0, 0, 0, 1, 0, 0, 0, 2}
	if !bytes.Equal(pkt.Body, want) {
		t.Errorf("body % x, want % x", pkt.Body, want)
	}
}

// Cutting a leash is the same packet with a zero holder — vanilla's null
// destEntity. It must still be a full eight bytes.
func TestEntityLinkCutSendsZeroHolder(t *testing.T) {
	pkt := EntityLink(attach.EntityLink{Leashed: 99})
	r := bytes.NewReader(pkt.Body)
	if got := readI32BE(t, r); got != 99 {
		t.Errorf("source = %d, want 99", got)
	}
	if got := readI32BE(t, r); got != 0 {
		t.Errorf("dest = %d, want 0 — the sentinel for an untied leash", got)
	}
}

// Negative ids round-trip as two's complement rather than being clamped.
func TestEntityLinkHandlesNegativeIDs(t *testing.T) {
	pkt := EntityLink(attach.EntityLink{Leashed: -1, Holder: -2})
	r := bytes.NewReader(pkt.Body)
	if got := readI32BE(t, r); got != -1 {
		t.Errorf("source = %d, want -1", got)
	}
	if got := readI32BE(t, r); got != -2 {
		t.Errorf("dest = %d, want -2", got)
	}
}

// The canonical-770 id is translated per client version on its way out
// (gwsession's cc.send runs the chain). 26.2 numbers this packet differently,
// and getting it wrong desynchronises the whole stream, so both ends are
// pinned here: 0x5d canonically, 0x64 once translated for 776.
func TestEntityLinkIDTranslatesForNewerClients(t *testing.T) {
	cases := []struct {
		proto int32
		want  int32
	}{
		{770, 0x5d}, {771, 0x5d}, {772, 0x5d}, {776, 0x64},
	}
	pkt := EntityLink(attach.EntityLink{Leashed: 1, Holder: 2})
	for _, c := range cases {
		tr := protocol.TranslatorFor(c.proto)
		got, body, drop := tr.Clientbound(protocol.StatePlay, pkt.ID, pkt.Body)
		if drop {
			t.Errorf("proto %d: the leash packet was dropped", c.proto)
			continue
		}
		if got != c.want {
			t.Errorf("proto %d: id %#x, want %#x", c.proto, got, c.want)
		}
		if len(body) != 8 {
			t.Errorf("proto %d: body %d bytes, want the 8 to pass through", c.proto, len(body))
		}
	}
}
