package protocol

import (
	"bytes"
	"math"
	"testing"
)

// decodeLpVec3 reads the low-precision vector per the 1.21.9+ wire spec —
// the test-side mirror of appendLpVec3.
func decodeLpVec3(t *testing.T, r *bytes.Reader) (x, y, z float64) {
	t.Helper()
	first, err := r.ReadByte()
	if err != nil {
		t.Fatalf("lpVec3 first byte: %v", err)
	}
	if first == 0 {
		return 0, 0, 0
	}
	second, _ := r.ReadByte()
	var u32 [4]byte
	if _, err := r.Read(u32[:]); err != nil {
		t.Fatalf("lpVec3 u32: %v", err)
	}
	remaining := uint64(u32[0])<<24 | uint64(u32[1])<<16 | uint64(u32[2])<<8 | uint64(u32[3])
	packed := remaining<<16 | uint64(second)<<8 | uint64(first)
	scale := int64(first & 3)
	if first&4 != 0 {
		more, err := ReadVarInt(r)
		if err != nil {
			t.Fatalf("lpVec3 scale continuation: %v", err)
		}
		scale |= int64(uint32(more)) << 2
	}
	unpack := func(v uint64) float64 {
		clamped := math.Min(float64(v&0x7fff), 32766)
		return clamped*2/32766 - 1
	}
	return unpack(packed>>3) * float64(scale), unpack(packed>>18) * float64(scale), unpack(packed>>33) * float64(scale)
}

func TestLpVec3RoundTrip(t *testing.T) {
	cases := [][3]float64{
		{0, 0, 0},
		{0.42, 0.36, 0},      // zombie knockback
		{-0.42, 0.36, 0.297}, // diagonal knockback
		{3.9, -2.5, 1.0},     // explosion-scale
		{17, 4, -9},          // scale needs the continuation varint
	}
	for _, c := range cases {
		b := appendLpVec3(nil, c[0], c[1], c[2])
		r := bytes.NewReader(b)
		x, y, z := decodeLpVec3(t, r)
		if r.Len() != 0 {
			t.Fatalf("%v: %d leftover bytes", c, r.Len())
		}
		scale := math.Max(1, math.Ceil(math.Max(math.Abs(c[0]), math.Max(math.Abs(c[1]), math.Abs(c[2])))))
		tol := scale * 2 / 32766 * 1.5 // quantization step with margin
		if math.Abs(x-c[0]) > tol || math.Abs(y-c[1]) > tol || math.Abs(z-c[2]) > tol {
			t.Fatalf("round-trip %v -> (%v,%v,%v), tol %v", c, x, y, z, tol)
		}
	}
}

func TestRewriteEntityVelocity773(t *testing.T) {
	// Canonical body: eid 7, velocity (0.42, 0.36, 0) as i16 1/8000ths.
	body := AppendVarInt(nil, 7)
	for _, v := range []int16{int16(0.42 * 8000), int16(0.36 * 8000), 0} {
		body = AppendI16(body, v)
	}
	out := rewriteEntityVelocity773(body)
	r := bytes.NewReader(out)
	eid, err := ReadVarInt(r)
	if err != nil || eid != 7 {
		t.Fatalf("eid: %d %v", eid, err)
	}
	x, y, z := decodeLpVec3(t, r)
	if math.Abs(x-0.42) > 0.01 || math.Abs(y-0.36) > 0.01 || math.Abs(z) > 0.01 {
		t.Fatalf("velocity round-trip: (%v,%v,%v)", x, y, z)
	}
	if r.Len() != 0 {
		t.Fatalf("%d leftover bytes", r.Len())
	}
	// Zero velocity → single 0x00 after the eid.
	zero := AppendVarInt(nil, 7)
	zero = append(zero, 0, 0, 0, 0, 0, 0)
	if out := rewriteEntityVelocity773(zero); len(out) != 2 || out[1] != 0 {
		t.Fatalf("zero velocity should be eid + 0x00, got % x", out)
	}
}
