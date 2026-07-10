package gwsession

import (
	"bytes"
	"testing"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// Structural re-parse of joinPacket — the house rule for moved wire
// composition (an extraction once dropped a single AppendString and broke
// every real client). Walks the exact 770 Login (play) layout.
func TestJoinPacketReparse(t *testing.T) {
	b := joinPacket(77, 1, 6)
	r := bytes.NewReader(b)

	i32 := func(what string) int32 {
		var v int32
		for i := 0; i < 4; i++ {
			c, err := r.ReadByte()
			if err != nil {
				t.Fatalf("%s: %v", what, err)
			}
			v = v<<8 | int32(c)
		}
		return v
	}
	u8 := func(what string) byte {
		c, err := r.ReadByte()
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		return c
	}
	vi := func(what string) int32 {
		v, err := protocol.ReadVarInt(r)
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		return v
	}
	str := func(what string) string {
		s, err := protocol.ReadString(r)
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		return s
	}

	if eid := i32("eid"); eid != 77 {
		t.Errorf("eid = %d", eid)
	}
	u8("hardcore")
	if n := vi("dim count"); n != 3 {
		t.Fatalf("dim count = %d", n)
	}
	for i := 0; i < 3; i++ {
		str("dim name")
	}
	vi("max players")
	if v := vi("view distance"); v != 6 {
		t.Errorf("view distance = %d", v)
	}
	vi("sim distance")
	u8("reduced debug")
	u8("respawn screen")
	u8("limited crafting")
	vi("dimension id")
	if s := str("dimension name"); s != "minecraft:overworld" {
		t.Errorf("dimension = %q", s)
	}
	for i := 0; i < 8; i++ {
		u8("hashed seed")
	}
	if gm := u8("gamemode"); gm != 1 {
		t.Errorf("gamemode = %d", gm)
	}
	u8("prev gamemode")
	u8("debug")
	u8("flat")
	u8("death location")
	vi("portal cooldown")
	if sl := vi("sea level"); sl != 63 {
		t.Errorf("sea level = %d", sl)
	}
	u8("secure chat")
	if r.Len() != 0 {
		t.Errorf("%d trailing bytes after join packet", r.Len())
	}
}

// The two position-sync bodies must differ ONLY in the angle fields and the
// flag word: absolute camera + zero flags for a server teleport, zero-delta
// relative camera + relative-velocity flags (0xF8) for a silent shard crossing.
func TestSyncPositionBodies(t *testing.T) {
	p := attach.Pos{X: 1, Y: 64, Z: -3, Yaw: 90, Pitch: 10}

	abs := syncPositionBody(p)
	keep := syncPositionKeepView(p)
	// teleport id (varint, differs) | 3×f64 pos | 3×f64 vel | 2×f32 angles | i32 flags
	const tail = 3*8 + 3*8 + 2*4 + 4
	if len(abs) < tail+1 || len(keep) < tail+1 {
		t.Fatalf("bodies too short: %d, %d", len(abs), len(keep))
	}
	flagsOf := func(b []byte) int32 {
		f := b[len(b)-4:]
		return int32(f[0])<<24 | int32(f[1])<<16 | int32(f[2])<<8 | int32(f[3])
	}
	if f := flagsOf(abs); f != 0 {
		t.Errorf("absolute sync flags = %#x, want 0", f)
	}
	if f := flagsOf(keep); f != 0x08|0x10|0x20|0x40|0x80 {
		t.Errorf("keep-view sync flags = %#x, want 0xf8", f)
	}
	// Both carry the same absolute x/y/z right after the teleport id.
	if !bytes.Equal(abs[len(abs)-tail:len(abs)-tail+24], keep[len(keep)-tail:len(keep)-tail+24]) {
		t.Error("keep-view sync does not carry the same absolute position")
	}
}

// appendHeightmap packs 256 9-bit values, 7 per long → 37 longs, prefixed by
// entry count 1 + MOTION_BLOCKING + long count.
func TestHeightmapShape(t *testing.T) {
	hm := make([]int16, 256)
	for i := range hm {
		hm[i] = 64
	}
	b := appendHeightmap(nil, hm)
	r := bytes.NewReader(b)
	if n, _ := protocol.ReadVarInt(r); n != 1 {
		t.Fatalf("entry count = %d", n)
	}
	if k, _ := protocol.ReadVarInt(r); k != heightmapMotionBlocking {
		t.Fatalf("heightmap kind = %d", k)
	}
	n, _ := protocol.ReadVarInt(r)
	if n != 37 {
		t.Fatalf("long count = %d, want 37", n)
	}
	if r.Len() != 37*8 {
		t.Fatalf("payload = %d bytes, want %d", r.Len(), 37*8)
	}
}
