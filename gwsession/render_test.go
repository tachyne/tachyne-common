package gwsession

import (
	"bytes"
	"encoding/binary"
	"io"
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
	b := appendHeightmap(nil, hm, attach.Sections)
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

// Tall-world shapes: an 108-section chunk needs 11-bit heightmap entries
// (5 per long → 52 longs) and multi-long light masks (110 bits → 2 longs).
func TestTallHeightmapAndMasks(t *testing.T) {
	hm := make([]int16, 256)
	for i := range hm {
		hm[i] = 1100
	}
	b := appendHeightmap(nil, hm, 108)
	r := bytes.NewReader(b)
	protocol.ReadVarInt(r) // entry count
	protocol.ReadVarInt(r) // kind
	n, _ := protocol.ReadVarInt(r)
	if n != 52 {
		t.Fatalf("long count = %d, want 52 (11 bits, 5/long)", n)
	}
	if r.Len() != 52*8 {
		t.Fatalf("payload = %d bytes", r.Len())
	}

	m := appendFullMask(nil, 110)
	mr := bytes.NewReader(m)
	if n, _ := protocol.ReadVarInt(mr); n != 2 {
		t.Fatalf("mask longs = %d, want 2", n)
	}
	var lo, hi int64
	binary.Read(mr, binary.BigEndian, &lo)
	binary.Read(mr, binary.BigEndian, &hi)
	if lo != -1 || hi != int64(1)<<46-1 {
		t.Fatalf("mask = %x,%x", lo, hi)
	}
	// The vanilla-height mask must stay the single full-26-bit long.
	m = appendFullMask(nil, 26)
	mr = bytes.NewReader(m)
	if n, _ := protocol.ReadVarInt(mr); n != 1 {
		t.Fatalf("24-section mask longs = %d, want 1", n)
	}
	binary.Read(mr, binary.BigEndian, &lo)
	if lo != int64(1)<<26-1 {
		t.Fatalf("24-section mask = %x", lo)
	}
}

// TestTrimmedLightSections: overworld chunks ship sky-light arrays only up to
// one section above the terrain (the client infers full-bright above — a
// decompiled-source fact), and block-light arrays only where lit. This is
// what keeps tall-world chunks inside a stock client heap.
func TestTrimmedLightSections(t *testing.T) {
	const sec = 108
	blocks := sec * 4096
	h := attach.ChunkHeader{CX: 1, CZ: 2, Dim: 0, Sections: sec, Biomes: make([]string, sec)}
	body := &attach.ChunkBody{
		BlockStates: make([]uint32, blocks),
		Heightmap:   make([]int16, 256),
		SkyLight:    make([]uint8, blocks),
		BlockLight:  make([]uint8, blocks),
	}
	for i := range body.Heightmap {
		body.Heightmap[i] = 100 // surface at y=100 → section 10
	}
	// One torch high up: block section 60 must be included.
	body.BlockLight[60*4096+123] = 14

	pkt := chunkPacket(h, body)
	r := bytes.NewReader(pkt)
	// Skip cx, cz, heightmap, col, block entities.
	r.Seek(8, 1)
	protocol.ReadVarInt(r) // heightmap entry count = 1
	protocol.ReadVarInt(r) // kind
	n, _ := protocol.ReadVarInt(r)
	r.Seek(int64(n)*8, 1)
	colLen, _ := protocol.ReadVarInt(r)
	r.Seek(int64(colLen), 1)
	protocol.ReadVarInt(r) // block entities = 0

	readMaskBits := func() int {
		n, _ := protocol.ReadVarInt(r)
		bits := 0
		for i := int32(0); i < n; i++ {
			var l int64
			binary.Read(r, binary.BigEndian, &l)
			for ; l != 0; l &= l - 1 {
				bits++
			}
		}
		return bits
	}
	skyBits := readMaskBits()
	blkBits := readMaskBits()
	protocol.ReadVarInt(r) // empty sky mask
	protocol.ReadVarInt(r) // empty block mask

	// Surface section 10 → topLit 11 → +below-world = 13 sky arrays.
	if skyBits != 13 {
		t.Fatalf("sky mask bits = %d, want 13 (trim above terrain)", skyBits)
	}
	if blkBits != 1 {
		t.Fatalf("block mask bits = %d, want 1 (only the lit section)", blkBits)
	}
	skyN, _ := protocol.ReadVarInt(r)
	if int(skyN) != skyBits {
		t.Fatalf("sky array count %d != mask bits %d", skyN, skyBits)
	}
	for i := int32(0); i < skyN; i++ {
		l, _ := protocol.ReadVarInt(r)
		r.Seek(int64(l), 1)
	}
	blkN, _ := protocol.ReadVarInt(r)
	if int(blkN) != 1 {
		t.Fatalf("block array count = %d", blkN)
	}
	l, _ := protocol.ReadVarInt(r)
	arr := make([]byte, l)
	io.ReadFull(r, arr)
	if arr[123/2]>>4 != 14 { // odd index → high nibble
		t.Fatalf("torch nibble lost: %x", arr[123/2])
	}
	if r.Len() != 0 {
		t.Fatalf("%d trailing bytes", r.Len())
	}
}
