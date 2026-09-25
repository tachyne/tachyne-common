package render770

import (
	"bytes"
	"testing"

	"github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// Every boss used to be drawn purple and solid, so the dragon, the wither and
// a raid looked the same at the top of the screen. Each carries its own now.
func TestBossBarCarriesItsOwnLook(t *testing.T) {
	e := attach.BossBar{Op: attach.BossBarAdd, Title: "Ender Dragon", Health: 0.5,
		Color: attach.BossPink, Overlay: attach.BossProgress,
		Flags: attach.BossMusic | attach.BossWorldFog}
	p := BossBar(e)
	if p.ID != IDBossBar {
		t.Fatalf("packet id %#x, want %#x", p.ID, IDBossBar)
	}
	r := bytes.NewReader(p.Body[16:]) // past the uuid
	if op, _ := protocol.ReadVarInt(r); op != 0 {
		t.Fatalf("op %d, want add (0)", op)
	}
	// Skip the title NBT by finding the health float that follows it: simpler
	// to re-render with a known title and compare the tail.
	tail := p.Body[len(p.Body)-6:] // health(4) is before colour/overlay/flags varints
	_ = tail
	// colour, overlay, flags are the last three fields.
	n := len(p.Body)
	if p.Body[n-1] != attach.BossMusic|attach.BossWorldFog {
		t.Errorf("flags %#x, want music+fog", p.Body[n-1])
	}
	if p.Body[n-2] != attach.BossProgress {
		t.Errorf("overlay %d, want progress", p.Body[n-2])
	}
	if p.Body[n-3] != attach.BossPink {
		t.Errorf("colour %d, want pink", p.Body[n-3])
	}
}

// A raid is red and notched into ten; the wither is purple and darkens the
// screen. The point is that they differ.
func TestBossBarsDiffer(t *testing.T) {
	raid := BossBar(attach.BossBar{Op: attach.BossBarAdd, Title: "Raid",
		Color: attach.BossRed, Overlay: attach.BossNotched10})
	wither := BossBar(attach.BossBar{Op: attach.BossBarAdd, Title: "Raid",
		Color: attach.BossPurple, Overlay: attach.BossProgress, Flags: attach.BossDarkenScreen})
	if bytes.Equal(raid.Body, wither.Body) {
		t.Error("a raid bar and a wither bar render identically")
	}
	n := len(raid.Body)
	if raid.Body[n-3] != attach.BossRed || raid.Body[n-2] != attach.BossNotched10 {
		t.Errorf("the raid bar is colour %d overlay %d, want red/notched-10", raid.Body[n-3], raid.Body[n-2])
	}
}

// ClientboundBossEventPacket's in-place updates: a rename, a restyle and a
// flag change each travel as their own operation, so the bar stays put.
func TestBossBarUpdateOps(t *testing.T) {
	var u [16]byte
	u[0] = 7
	name := append(append([]byte(nil), u[:]...), protocol.AppendVarInt(nil, 3)...)
	name = append(name, oracleChatNBT("Renamed")...)
	eq(t, "bossbar name", BossBar(attach.BossBar{UUID: u, Op: attach.BossBarTitle, Title: "Renamed"}), IDBossBar, name)

	style := append(append([]byte(nil), u[:]...), protocol.AppendVarInt(nil, 4)...)
	style = protocol.AppendVarInt(style, attach.BossRed)
	style = protocol.AppendVarInt(style, attach.BossNotched10)
	eq(t, "bossbar style", BossBar(attach.BossBar{UUID: u, Op: attach.BossBarStyle, Color: attach.BossRed, Overlay: attach.BossNotched10}), IDBossBar, style)

	flags := append(append([]byte(nil), u[:]...), protocol.AppendVarInt(nil, 5)...)
	flags = protocol.AppendU8(flags, attach.BossDarkenScreen)
	eq(t, "bossbar flags", BossBar(attach.BossBar{UUID: u, Op: attach.BossBarFlags, Flags: attach.BossDarkenScreen}), IDBossBar, flags)
}
