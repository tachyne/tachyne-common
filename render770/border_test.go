package render770

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// readF64BE and readVarLong decode independently of the writer's helpers, so
// this really is an oracle for the layout rather than a round trip through the
// same code.
func readF64BE(t *testing.T, r *bytes.Reader) float64 {
	t.Helper()
	var b [8]byte
	if _, err := r.Read(b[:]); err != nil {
		t.Fatalf("short read: %v", err)
	}
	return math.Float64frombits(binary.BigEndian.Uint64(b[:]))
}

func readVarLong(t *testing.T, r *bytes.Reader) int64 {
	t.Helper()
	var v int64
	for shift := 0; ; shift += 7 {
		c, err := r.ReadByte()
		if err != nil {
			t.Fatalf("short varlong: %v", err)
		}
		v |= int64(c&0x7f) << shift
		if c&0x80 == 0 {
			return v
		}
	}
}

// The border's wire layout: two doubles of centre, the old and new diameters,
// a VarLong of lerp milliseconds, then the absolute max and the two warning
// VarInts.
func TestWorldBorderReparse(t *testing.T) {
	pkt := WorldBorder(attach.WorldBorder{
		CenterX: -12.5, CenterZ: 640.25,
		Size: 1000, Target: 250, LerpMs: 30000,
		WarnBlocks: 7, WarnTime: 22,
	})
	if pkt.ID != IDInitBorder {
		t.Fatalf("packet id %#x, want %#x", pkt.ID, IDInitBorder)
	}
	r := bytes.NewReader(pkt.Body)
	if got := readF64BE(t, r); got != -12.5 {
		t.Errorf("centre x %v", got)
	}
	if got := readF64BE(t, r); got != 640.25 {
		t.Errorf("centre z %v", got)
	}
	if got := readF64BE(t, r); got != 1000 {
		t.Errorf("old size %v", got)
	}
	if got := readF64BE(t, r); got != 250 {
		t.Errorf("new size %v", got)
	}
	if got := readVarLong(t, r); got != 30000 {
		t.Errorf("lerp ms %v", got)
	}
	if got, _ := protocol.ReadVarInt(r); got != borderAbsoluteMax {
		t.Errorf("absolute max %v", got)
	}
	if got, _ := protocol.ReadVarInt(r); got != 7 {
		t.Errorf("warn blocks %v", got)
	}
	if got, _ := protocol.ReadVarInt(r); got != 22 {
		t.Errorf("warn time %v", got)
	}
	if r.Len() != 0 {
		t.Errorf("%d trailing bytes", r.Len())
	}
}

// A stationary border must still be a well-formed packet: the client reads a
// target either way, so it has to equal the current size rather than zero —
// a zero would tell the client the world is collapsing to nothing.
func TestStationaryBorderTargetsItsOwnSize(t *testing.T) {
	pkt := WorldBorder(attach.WorldBorder{CenterX: 0, CenterZ: 0, Size: 6000})
	r := bytes.NewReader(pkt.Body)
	readF64BE(t, r)
	readF64BE(t, r)
	old := readF64BE(t, r)
	target := readF64BE(t, r)
	if old != 6000 || target != 6000 {
		t.Errorf("stationary border rendered old=%v target=%v, want both 6000", old, target)
	}
	if ms := readVarLong(t, r); ms != 0 {
		t.Errorf("stationary border has lerp %v, want 0", ms)
	}
}
