package render770

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// A title is up to three packets, and vanilla sends the timing FIRST so the
// very first frame is already shown at the right speed.
func TestTitlePacketOrder(t *testing.T) {
	got := TitlePackets(attach.Title{Title: "Welcome", Subtitle: "to tachyne",
		FadeIn: 10, Stay: 70, FadeOut: 20})
	if len(got) != 3 {
		t.Fatalf("%d packets, want 3", len(got))
	}
	want := []int32{IDSetTitleAnimation, IDSetSubtitleText, IDSetTitleText}
	for i, id := range want {
		if got[i].ID != id {
			t.Errorf("packet %d is %#x, want %#x", i, got[i].ID, id)
		}
	}
	b := got[0].Body
	if len(b) != 12 {
		t.Fatalf("the timing packet is %d bytes, want 12", len(b))
	}
	for i, want := range []int32{10, 70, 20} {
		if v := int32(binary.BigEndian.Uint32(b[i*4:])); v != want {
			t.Errorf("timing field %d = %d, want %d", i, v, want)
		}
	}
	if !bytes.Contains(got[2].Body, []byte("Welcome")) {
		t.Error("the title packet does not carry the title")
	}
	if !bytes.Contains(got[1].Body, []byte("to tachyne")) {
		t.Error("the subtitle packet does not carry the subtitle")
	}
}

// Setting one of the pair leaves the other alone: no packet is sent for an
// empty field, so a subtitle can be changed under a standing title.
func TestTitleSendsOnlyWhatIsSet(t *testing.T) {
	only := TitlePackets(attach.Title{Title: "Round 2"})
	if len(only) != 1 || only[0].ID != IDSetTitleText {
		t.Fatalf("got %d packets (first %#x), want just the title", len(only), only[0].ID)
	}
	sub := TitlePackets(attach.Title{Subtitle: "go"})
	if len(sub) != 1 || sub[0].ID != IDSetSubtitleText {
		t.Fatalf("got %d packets, want just the subtitle", len(sub))
	}
	// Timing on its own is legal too — it is how you set the speed ahead.
	times := TitlePackets(attach.Title{Stay: 100})
	if len(times) != 1 || times[0].ID != IDSetTitleAnimation {
		t.Fatalf("got %d packets, want just the timing", len(times))
	}
}

// Clear takes them off screen; reset also forgets the timing.
func TestTitleClear(t *testing.T) {
	for _, tc := range []struct {
		reset bool
		want  byte
	}{{false, 0}, {true, 1}} {
		got := TitlePackets(attach.Title{Clear: true, Reset: tc.reset})
		if len(got) != 1 || got[0].ID != IDClearTitles {
			t.Fatalf("reset=%v: got %d packets, want one clear", tc.reset, len(got))
		}
		if got[0].Body[0] != tc.want {
			t.Errorf("reset=%v: body %d, want %d", tc.reset, got[0].Body[0], tc.want)
		}
	}
	// A clear ignores any text set alongside it rather than sending both.
	if got := TitlePackets(attach.Title{Clear: true, Title: "ignored"}); len(got) != 1 {
		t.Errorf("a clear sent %d packets, want 1", len(got))
	}
}

// A kick shows a screen with the reason on it. Closing the socket instead
// leaves the player at "connection lost", which says nothing about why.
func TestDisconnectCarriesTheReason(t *testing.T) {
	p := Disconnect(attach.Disconnect{Reason: "Kicked: building in spawn"})
	if p.ID != IDDisconnect {
		t.Fatalf("packet id %#x, want %#x", p.ID, IDDisconnect)
	}
	if !bytes.Contains(p.Body, []byte("Kicked: building in spawn")) {
		t.Errorf("the packet does not carry the reason: %x", p.Body)
	}
	// It is a text component, like every other reason-bearing packet here.
	if p.Body[0] != 0x08 {
		t.Errorf("body starts %#x, want a nameless TAG_String", p.Body[0])
	}
}

// /title's text is a component: its colour and weight reach the screen.
func TestTitleComponent(t *testing.T) {
	raw := []byte(`{"text":"Hi","color":"red","bold":true}`)
	ps := TitlePackets(attach.Title{Title: "Hi", TitleJSON: raw})
	nbt, ok := protocol.TextComponentNBT(raw)
	if !ok || len(ps) != 1 || !bytes.Equal(ps[0].Body, nbt) {
		t.Fatalf("title body %x, want the component %x", ps[0].Body, nbt)
	}
}
