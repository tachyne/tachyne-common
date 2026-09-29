package protocol

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

// Expected bytes are written by hand from 26.2's and 26.3's
// ClientboundSetTimePacket.STREAM_CODEC: LONG game time, then a map —
// VarInt count, and per entry the WorldClock holder (a VarInt registry id)
// and ClockNetworkState (VAR_LONG total, FLOAT partial tick, FLOAT rate).

// The canonical body with clock states: game time 100, day time 6500
// (30500 mod a day), the overworld clock paused, then two clocks.
var setTimeWithClocks = []byte{
	0, 0, 0, 0, 0, 0, 0, 0x64, // game time 100
	0, 0, 0, 0, 0, 0, 0x19, 0x64, // day time 6500
	0,                                                          // tickDayTime: the overworld clock is paused
	2,                                                          // two clocks
	0, 0, 0, 0, 0, 0, 0, 0x77, 0x24, 0x3f, 0, 0, 0, 0, 0, 0, 0, // overworld: 30500, 0.5, rate 0
	1, 0, 0, 0, 0, 0, 0x03, 0x0d, 0x40, 0, 0, 0, 0, 0x40, 0, 0, 0, // the End: 200000, 0, rate 2
}

var setTime26xWithClocks = []byte{
	0, 0, 0, 0, 0, 0, 0, 0x64, // game time 100
	2,                                              // map size
	0, 0xa4, 0xee, 0x01, 0x3f, 0, 0, 0, 0, 0, 0, 0, // overworld: VarLong 30500, 0.5, 0
	1, 0xc0, 0x9a, 0x0c, 0, 0, 0, 0, 0x40, 0, 0, 0, // the_end: VarLong 200000, 0, 2
}

func TestSetTime26xCarriesEveryClock(t *testing.T) {
	for _, v := range []int32{776, 777} {
		_, out, drop := TranslatorFor(v).Clientbound(StatePlay, canonUpdateTime, append([]byte(nil), setTimeWithClocks...))
		if drop {
			t.Fatalf("v%d: set_time dropped", v)
		}
		if !bytes.Equal(out, setTime26xWithClocks) {
			t.Errorf("v%d:\n got %x\nwant %x", v, out, setTime26xWithClocks)
		}
		// Round trip: read it back as the client's codec does.
		gt, clocks, ok := readSetTime26x(out)
		if !ok || gt != 100 || len(clocks) != 2 {
			t.Fatalf("v%d: read back %d %v %v", v, gt, clocks, ok)
		}
		want := []clockState{{0, 30500, 0.5, 0}, {1, 200000, 0, 2}}
		for i := range want {
			if clocks[i] != want[i] {
				t.Errorf("v%d clock %d: %+v, want %+v", v, i, clocks[i], want[i])
			}
		}
	}
}

// Without clock states (an older engine) the overworld clock alone goes out
// at the day time, rate 1.
func TestSetTime26xWithoutClocks(t *testing.T) {
	body := []byte{0, 0, 0, 0, 0, 0, 0, 0x64, 0, 0, 0, 0, 0, 0, 0x19, 0x64, 1}
	want := []byte{0, 0, 0, 0, 0, 0, 0, 0x64, 1, 0, 0xe4, 0x32, 0, 0, 0, 0, 0x3f, 0x80, 0, 0}
	for _, v := range []int32{776, 777} {
		_, out, _ := TranslatorFor(v).Clientbound(StatePlay, canonUpdateTime, append([]byte(nil), body...))
		if !bytes.Equal(out, want) {
			t.Errorf("v%d:\n got %x\nwant %x", v, out, want)
		}
	}
}

// A 770-form client never sees the clock states.
func TestSetTimeClocksStrippedBefore26x(t *testing.T) {
	out, _ := remapClientboundIDs(770, canonUpdateTime, append([]byte(nil), setTimeWithClocks...))
	if !bytes.Equal(out, setTimeWithClocks[:17]) {
		t.Errorf("got %x, want %x", out, setTimeWithClocks[:17])
	}
}

type clockState struct {
	id      int32
	total   int64
	partial float32
	rate    float32
}

func readSetTime26x(b []byte) (int64, []clockState, bool) {
	if len(b) < 8 {
		return 0, nil, false
	}
	gt := int64(binary.BigEndian.Uint64(b))
	r := bytes.NewReader(b[8:])
	n, err := ReadVarInt(r)
	if err != nil {
		return 0, nil, false
	}
	var out []clockState
	for i := int32(0); i < n; i++ {
		id, err := ReadVarInt(r)
		if err != nil {
			return 0, nil, false
		}
		var total uint64
		for shift := uint(0); ; shift += 7 {
			c, err := r.ReadByte()
			if err != nil || shift > 63 {
				return 0, nil, false
			}
			total |= uint64(c&0x7f) << shift
			if c&0x80 == 0 {
				break
			}
		}
		var f [8]byte
		if _, err := r.Read(f[:]); err != nil {
			return 0, nil, false
		}
		out = append(out, clockState{id, int64(total),
			math.Float32frombits(binary.BigEndian.Uint32(f[:4])),
			math.Float32frombits(binary.BigEndian.Uint32(f[4:]))})
	}
	return gt, out, r.Len() == 0
}
