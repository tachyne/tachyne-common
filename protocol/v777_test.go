package protocol

import (
	"bytes"
	"testing"
)

// The 26.3 rewriters, each checked against the layout the 26.3 packet
// classes read: spawn-info previous game mode as an optional varint, the
// moves' properties varint, the position sync's linear path, the particle
// packet's new field order, the animate renumbering and the swing
// retarget, and the three serverbound reshapes.
func TestRespawnPrevGameMode777(t *testing.T) {
	body := AppendVarInt(nil, 0)                     // dimension type
	body = AppendString(body, "minecraft:overworld") // dimension
	body = append(body, 1, 2, 3, 4, 5, 6, 7, 8)      // seed
	body = append(body, 0)                           // survival
	body = append(body, 0xff)                        // no previous mode
	body = append(body, 0, 0, 0, 0, 0, 0x3f, 1)      // debug, flat, no death loc, portal cooldown 0, sea level 63, keep byte
	out := rewriteRespawnPrevGameMode777(StatePlay, body)
	want := append(append([]byte(nil), body[:len(body)-8]...), 0)
	want = append(want, body[len(body)-7:]...)
	if !bytes.Equal(out, want) {
		t.Fatalf("no previous mode should become varint 0:\n got %x\nwant %x", out, want)
	}
	body[len(body)-8] = 1 // previous = creative
	out = rewriteRespawnPrevGameMode777(StatePlay, body)
	if out[len(out)-8] != 2 {
		t.Fatalf("a previous mode of 1 is varint 2, got %x", out)
	}
	// Login: the same spawn info after the login prefix.
	login := append([]byte{0, 0, 0, 7, 0}, AppendVarInt(nil, 1)...)
	login = AppendString(login, "minecraft:overworld")
	login = append(login, 20, 12, 12, 0, 1, 0)
	login = append(login, body...)
	login = append(login, 0, 0) // online mode, enforces secure chat
	out = rewriteLoginPrevGameMode777(StatePlay, login)
	if len(out) != len(login) || out[len(out)-10] != 2 {
		t.Fatalf("login previous mode: got %x", out[len(out)-12:])
	}
}

func TestMoveEntity777(t *testing.T) {
	pos := append(AppendVarInt(nil, 300), 0, 10, 0, 0, 0xff, 0xf0, 1)
	if got := rewriteMoveEntity777(false)(StatePlay, pos); !bytes.Equal(got, append(AppendVarInt(nil, 300), 1, 0, 10, 0, 0, 0xff, 0xf0)) {
		t.Fatalf("pos: %x", got)
	}
	rot := append(AppendVarInt(nil, 300), 0, 10, 0, 0, 0xff, 0xf0, 0x40, 0x10, 0)
	if got := rewriteMoveEntity777(true)(StatePlay, rot); !bytes.Equal(got, append(AppendVarInt(nil, 300), 0, 0, 10, 0, 0, 0xff, 0xf0, 0x40, 0x10)) {
		t.Fatalf("pos_rot: %x", got)
	}
	sync := AppendVarInt(nil, 5)
	for i := 0; i < 24; i++ {
		sync = append(sync, byte(i)) // position
	}
	for i := 0; i < 24; i++ {
		sync = append(sync, 0xee) // delta movement, dropped
	}
	sync = append(sync, 1, 2, 3, 4, 5, 6, 7, 8, 1)
	got := rewriteEntityPositionSync777(StatePlay, sync)
	want := append(AppendVarInt(nil, 5), 0)
	want = append(want, sync[1:25]...)
	want = append(want, 1, 2, 3, 4, 5, 6, 7, 8, 1)
	if !bytes.Equal(got, want) {
		t.Fatalf("position sync:\n got %x\nwant %x", got, want)
	}
}

func TestLevelParticles777(t *testing.T) {
	body := []byte{1, 0}
	for i := 0; i < 24; i++ {
		body = append(body, byte(0x40+i)) // position
	}
	body = append(body, 0xa, 0xa, 0xa, 0xa, 0xb, 0xb, 0xb, 0xb, 0xc, 0xc, 0xc, 0xc) // offsets
	body = append(body, 0x3e, 0, 0, 0)                                              // speed
	body = append(body, 0, 0, 0, 9)                                                 // count
	body = AppendVarInt(body, 68)                                                   // note (26.3 id)
	got := rewriteLevelParticles777(StatePlay, body)
	want := AppendVarInt(nil, 68)
	want = append(want, body[:2+24+12]...)
	want = append(want, 0x3e, 0, 0, 0, 0x3e, 0, 0, 0, 0x3e, 0, 0, 0, 0, 0, 0, 9, 0)
	if !bytes.Equal(got, want) {
		t.Fatalf("particles:\n got %x\nwant %x", got, want)
	}
}

func TestAnimate777(t *testing.T) {
	for _, tc := range [][2]byte{{2, 0}, {4, 1}, {5, 2}} {
		got := rewriteAnimateActions777(StatePlay, append(AppendVarInt(nil, 9), tc[0]))
		if got[len(got)-1] != tc[1] {
			t.Errorf("action %d → %d, got %d", tc[0], tc[1], got[len(got)-1])
		}
	}
	if _, ok := swingAnimation777(append(AppendVarInt(nil, 9), 2)); ok {
		t.Fatal("a wake-up is not a swing")
	}
	sw, ok := swingAnimation777(append(AppendVarInt(nil, 9), 3))
	if !ok || !bytes.Equal(sw, []byte{9, 1, 1, 6}) {
		t.Fatalf("off-hand swing → entity, hand 1, WHACK, 6: %x", sw)
	}
	tr := TranslatorFor(777)
	id, body, drop := tr.Clientbound(StatePlay, canonAnimate, append(AppendVarInt(nil, 9), 0))
	if drop || id != SwingAnimation777 || !bytes.Equal(body, []byte{9, 0, 1, 6}) {
		t.Fatalf("the chain retargets a swing to swing_animation %d: id %d body %x", SwingAnimation777, id, body)
	}
	id, _, _ = tr.Clientbound(StatePlay, canonAnimate, append(AppendVarInt(nil, 9), 4))
	if id != 2 {
		t.Fatalf("a critical hit stays an animate (id 2 at 26.3), got %d", id)
	}
}

func TestServerbound777(t *testing.T) {
	tr := TranslatorFor(777)
	// punch (46 at 26.3) → swing with the main hand, down to canonical.
	id, body, drop := tr.Serverbound(StatePlay, 46, nil)
	sid, _, _ := TranslatorFor(776).Serverbound(StatePlay, 63, []byte{0})
	if drop || id != sid || !bytes.Equal(body, []byte{0}) {
		t.Fatalf("punch → swing(main): id %d (want %d) body %x", id, sid, body)
	}
	// sign_update: pos, four lines, slot → pos, front bool, four lines.
	sign := append([]byte{1, 2, 3, 4, 5, 6, 7, 8}, AppendString(nil, "a")...)
	sign = AppendString(sign, "b")
	sign = AppendString(sign, "c")
	sign = AppendString(sign, "d")
	sign = AppendVarInt(sign, 1) // FRONT
	got := rewriteSignUpdate777(StatePlay, sign)
	want := append([]byte{1, 2, 3, 4, 5, 6, 7, 8, 1}, sign[8:len(sign)-1]...)
	if !bytes.Equal(got, want) {
		t.Fatalf("sign update:\n got %x\nwant %x", got, want)
	}
	// accept_teleportation keeps the id only.
	acc := append(AppendVarInt(nil, 300), make([]byte, 32)...)
	if got := rewriteAcceptTeleportation777(StatePlay, acc); !bytes.Equal(got, AppendVarInt(nil, 300)) {
		t.Fatalf("accept teleportation: %x", got)
	}
}

func TestKnownPacks777(t *testing.T) {
	tr := TranslatorFor(777)
	body := AppendVarInt(nil, 1)
	body = AppendString(body, "minecraft")
	body = AppendString(body, "core")
	body = AppendString(body, "1.21.5")
	_, out, _ := tr.Clientbound(StateConfiguration, cfgKnownPacksID, body)
	if !bytes.Contains(out, []byte("26.3")) {
		t.Fatalf("known packs should carry 26.3: %q", out)
	}
}
