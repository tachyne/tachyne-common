package render770

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// A respawn carrying the last death location writes the optional GlobalPos:
// present, the dimension key, then the packed position.
func TestRespawnDeathLocation(t *testing.T) {
	plain := Respawn(attach.Dimension{Dim: 0, Gamemode: 0})
	withDeath := Respawn(attach.Dimension{Dim: 1, Gamemode: 0, Death: &attach.DeathPos{Dim: 1, X: 10, Y: 64, Z: -3}})
	if len(withDeath.Body) <= len(plain.Body) {
		t.Fatal("the death location should lengthen the packet")
	}
	want := protocol.AppendBool(nil, true)
	want = protocol.AppendString(want, "minecraft:the_nether")
	want = protocol.AppendPosition(want, 10, 64, -3)
	if !bytes.Contains(withDeath.Body, want) {
		t.Fatalf("death location bytes missing: %x", withDeath.Body)
	}
	if !bytes.Contains(plain.Body, []byte{0, 0, 63}) { // absent, cooldown 0, sea level 63
		t.Fatal("plain respawn should write an absent death location")
	}
}

// 1.21.9 rebuilt set_default_spawn_position around LevelData.RespawnData: a
// GlobalPos (dimension key + block) and a yaw AND pitch, where 1.21.5 had a
// bare block position and one angle. The chain has no body rewriter for it, so
// a 26.x client fed the old body throws a DecoderException on the packet and
// never finishes joining — which is exactly what happened in the wild.
func TestDefaultSpawnBodyChangesAt773(t *testing.T) {
	e := attach.DefaultSpawn{X: 100, Y: 64, Z: -200, Angle: 90}

	old := DefaultSpawnData(e, 770)
	if old.ID != IDSetDefaultSpawn {
		t.Fatalf("packet id = %#x, want %#x", old.ID, IDSetDefaultSpawn)
	}
	if len(old.Body) != 12 { // a packed position (8) + a float (4)
		t.Fatalf("770 body is %d bytes, want 12", len(old.Body))
	}

	for _, v := range []int32{773, 774, 776, 777} {
		p := DefaultSpawnData(e, v)
		r := bytes.NewReader(p.Body)
		dim, err := protocol.ReadString(r)
		if err != nil {
			t.Fatalf("proto %d: no dimension key: %v", v, err)
		}
		if dim != "minecraft:overworld" {
			t.Errorf("proto %d: dimension = %q, want minecraft:overworld", v, dim)
		}
		// GlobalPos writes the key first, then the block; then yaw, then pitch.
		if r.Len() != 16 {
			t.Fatalf("proto %d: %d bytes after the key, want 16 (pos 8 + yaw 4 + pitch 4)", v, r.Len())
		}
		var pos int64
		if err := binary.Read(r, binary.BigEndian, &pos); err != nil {
			t.Fatal(err)
		}
		if got := protocol.AppendPosition(nil, e.X, e.Y, e.Z); int64(binary.BigEndian.Uint64(got)) != pos {
			t.Errorf("proto %d: the block position does not round-trip", v)
		}
		var yaw, pitch float32
		binary.Read(r, binary.BigEndian, &yaw)
		binary.Read(r, binary.BigEndian, &pitch)
		if yaw != 90 || pitch != 0 {
			t.Errorf("proto %d: yaw/pitch = %v/%v, want 90/0", v, yaw, pitch)
		}
	}

	// An explicit dimension is carried through.
	p := DefaultSpawnData(attach.DefaultSpawn{Dim: "minecraft:the_nether"}, 776)
	if dim, _ := protocol.ReadString(bytes.NewReader(p.Body)); dim != "minecraft:the_nether" {
		t.Errorf("dimension = %q, want minecraft:the_nether", dim)
	}
}
