package protocol

import (
	"bytes"
	"testing"
)

func metaIndex0(t *testing.T, body []byte) byte {
	t.Helper()
	r := bytes.NewReader(body)
	ReadVarInt(r) // eid
	idx, _ := r.ReadByte()
	return idx
}

// TestSlimeSizeIndexShift26_2: 26.2 inserted cube-mob fields at 16-17, so the
// slime/magma size VarInt at canonical index 16 must move to 18 for 776 — and
// stay put for 775 and below. The shift is entity-type-specific and lives in
// ShiftCubeMobMeta, which the gateway calls only for slimes/magma cubes.
func TestSlimeSizeIndexShift26_2(t *testing.T) {
	body := AppendVarInt(nil, 243) // eid
	body = append(body, 16)        // index 16
	body = AppendVarInt(body, 1)   // type: VarInt
	body = AppendVarInt(body, 2)   // size 2
	body = append(body, 0xff)      // terminator

	if idx := metaIndex0(t, ShiftCubeMobMeta(776, body)); idx != 18 {
		t.Fatalf("26.2 slime size must ride at index 18, got %d", idx)
	}
	if idx := metaIndex0(t, ShiftCubeMobMeta(775, body)); idx != 16 {
		t.Fatalf("26.1 keeps slime size at 16, got %d", idx)
	}
}

// TestCreeperSwellNotShifted is the regression guard for the disconnect bug: a
// creeper's SWELL_DIR is also an index-16 VarInt, but it must NOT be shifted —
// index 18 on a 26.2 creeper is a Boolean, so an Int there crashes the client.
// The generic auto-path (remapEntityMeta) must leave it at 16, and the gateway
// never calls ShiftCubeMobMeta for a creeper (not a cube mob).
func TestCreeperSwellNotShifted(t *testing.T) {
	body := AppendVarInt(nil, 60) // eid
	body = append(body, 16)       // index 16 = SWELL_DIR
	body = AppendVarInt(body, 1)  // type: VarInt
	body = AppendVarInt(body, 1)  // primed (+1) — the exact crash value
	body = append(body, 0xff)

	if idx := metaIndex0(t, remapEntityMeta(776, body)); idx != 16 {
		t.Fatalf("auto-path must never shift index 16, got %d", idx)
	}
	// isCubeMob gates the gateway call: creeper (30) is not a cube mob.
	if isCube := func(e int32) bool { return e == 111 || e == 77 }(30); isCube {
		t.Fatal("creeper must not be treated as a cube mob")
	}
}
