package render770

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// The world spawn is what a plain compass points at. Re-parse the packet
// rather than trusting the writer: a packed position and an angle, which is
// the shape up to and including 1.21.8 (the 773+ shape is in misc_test.go).
func TestDefaultSpawnData(t *testing.T) {
	p := DefaultSpawnData(attach.DefaultSpawn{X: -1533, Y: 300, Z: 4175, Angle: 90}, 770)
	if p.ID != IDSetDefaultSpawn {
		t.Fatalf("packet id %#x, want %#x", p.ID, IDSetDefaultSpawn)
	}
	if len(p.Body) != 12 { // 8-byte packed position + 4-byte float
		t.Fatalf("body is %d bytes, want 12", len(p.Body))
	}
	x, y, z := protocol.ReadPosition(p.Body[:8])
	if x != -1533 || y != 300 || z != 4175 {
		t.Errorf("spawn %d,%d,%d, want -1533,300,4175", x, y, z)
	}
	if a := math.Float32frombits(binary.BigEndian.Uint32(p.Body[8:])); a != 90 {
		t.Errorf("angle %v, want 90", a)
	}
}
