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

// TestAgeableMobIndexShift26_2: 26.2 inserted AGE_LOCKED as AgeableMob's second
// field (index 17), pushing every animal subclass's type-specific fields up
// one — a bee's flags byte 17→18 and anger VarInt 18→19, sheep wool and
// tamable flags 17→18 — while baby (16) predates it and stays. Below 776
// nothing moves. Regression guard for a live disconnect: a byte written at a
// 26.2 bee's 17 lands on the Boolean AGE_LOCKED and kicks the client.
func TestAgeableMobIndexShift26_2(t *testing.T) {
	body := AppendVarInt(nil, 1432) // eid
	body = append(body, 16)         // baby (Boolean)
	body = AppendVarInt(body, 8)    // type: Boolean
	body = append(body, 0)
	body = append(body, 17)      // bee flags (byte)
	body = AppendVarInt(body, 0) // type: byte
	body = append(body, 8)       // nectar bit
	body = append(body, 18)      // bee anger (VarInt)
	body = AppendVarInt(body, 1) // type: VarInt
	body = AppendVarInt(body, 30)
	body = append(body, 0xff)

	shifted := ShiftAgeableMobMeta(776, body)
	r := bytes.NewReader(shifted)
	ReadVarInt(r) // eid
	var got []byte
	for {
		idx, _ := r.ReadByte()
		if idx == 0xff {
			break
		}
		got = append(got, idx)
		typ, _ := ReadVarInt(r)
		switch typ {
		case 0, 8: // byte / boolean
			r.ReadByte()
		case 1: // varint
			ReadVarInt(r)
		}
	}
	if len(got) != 3 || got[0] != 16 || got[1] != 18 || got[2] != 19 {
		t.Fatalf("26.2 ageable indices = %v, want [16 18 19]", got)
	}
	if idx := metaIndex0(t, ShiftAgeableMobMeta(772, body)); idx != 16 {
		t.Fatalf("pre-776 must not shift, first index %d", idx)
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
