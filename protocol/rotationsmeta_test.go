package protocol

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

// An armor stand's poses are ROTATIONS (serializer 9 on 770, 776 and 777:
// three floats). A body with a pose BEFORE other fields must come through
// whole — the pose copied as twelve bytes, the pose after it renumbered —
// rather than bailing out and leaving the canonical serializer ids in place.
func TestRotationsMetaBeforeOtherFields(t *testing.T) {
	rot := func(b []byte, x, y, z float32) []byte {
		for _, f := range []float32{x, y, z} {
			b = binary.BigEndian.AppendUint32(b, math.Float32bits(f))
		}
		return b
	}
	body := AppendVarInt(nil, 7)
	body = append(body, 16) // head pose
	body = AppendVarInt(body, metaTypeRotations)
	body = rot(body, -10, 20, 0)
	body = append(body, 6) // then the pose: canonical 21, 20 on 26.x
	body = AppendVarInt(body, metaTypePose)
	body = AppendVarInt(body, 5)
	body = append(body, 0xff)

	want := AppendVarInt(nil, 7)
	want = append(want, 16, 9)
	want = rot(want, -10, 20, 0)
	want = append(want, 6, 20, 5, 0xff)
	for _, v := range []int32{776, 777} {
		if got := remapEntityMeta(v, body); !bytes.Equal(got, want) {
			t.Errorf("%d: got %x want %x", v, got, want)
		}
	}
	if got := remapEntityMeta(770, body); !bytes.Equal(got, body) {
		t.Errorf("770: got %x want the canonical body", got)
	}
	// A truncated pose is malformed: the body comes back untouched.
	short := append(AppendVarInt(nil, 7), 16, 9, 0, 0)
	if got := remapEntityMeta(777, short); !bytes.Equal(got, short) {
		t.Errorf("truncated: got %x", got)
	}
}
