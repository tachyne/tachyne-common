package protocol

import (
	"bytes"
	"testing"
)

// lodestoneSlot builds a compass stack carrying lodestone_tracker in canonical
// (770) numbering: a target in the nether at a packed position, tracked.
func lodestoneSlot(compass int32, withTarget bool) []byte {
	b := AppendVarInt(nil, 1)               // count
	b = AppendVarInt(b, compass)            // the compass
	b = AppendVarInt(b, 1)                  // 1 component to add
	b = AppendVarInt(b, 0)                  // 0 to remove
	b = AppendVarInt(b, componentLodestone) // lodestone_tracker
	if withTarget {
		b = append(b, 1)
		b = AppendString(b, "minecraft:the_nether")
		b = AppendI64(b, 0x0123456789abcdef)
	} else {
		b = append(b, 0)
	}
	return append(b, 1) // tracked
}

// The tracker's payload passes through byte-for-byte; only the component id
// takes the client version's number (58 → 67 on 26.2), in both directions.
func TestLodestoneTrackerRenumbersAcrossVersions(t *testing.T) {
	const compass = 1034 // canonical 1.21.11 id — remapped like any item
	for _, tc := range []struct {
		version int32
		wantID  int32
	}{{770, 58}, {774, 65}, {776, 67}} {
		for _, withTarget := range []bool{true, false} {
			body := lodestoneSlot(compass, withTarget)
			var out []byte
			if !copyFullSlot(bytes.NewReader(body), &out, func(i int32) int32 { return i }, tc.version, false) {
				t.Fatalf("v%d target=%v: the lodestone_tracker case is missing", tc.version, withTarget)
			}
			r := bytes.NewReader(out)
			ReadVarInt(r) // count
			ReadVarInt(r) // item
			ReadVarInt(r) // add count
			ReadVarInt(r) // remove count
			cid, _ := ReadVarInt(r)
			if cid != tc.wantID {
				t.Errorf("v%d: component id %d, want %d", tc.version, cid, tc.wantID)
			}
			// What follows the id is the original payload, untouched.
			wantPayload := body[len(body)-r.Len():]
			_ = wantPayload
			payloadIn := body[len(body)-(len(body)-indexAfterCompID(body)):]
			if got := out[len(out)-r.Len():]; !bytes.Equal(got, payloadIn) {
				t.Errorf("v%d target=%v: payload %x, want %x", tc.version, withTarget, got, payloadIn)
			}
			// And the client's echo (serverbound) comes back canonical.
			var back []byte
			if !copyFullSlot(bytes.NewReader(out), &back, func(i int32) int32 { return i }, tc.version, true) {
				t.Fatalf("v%d: serverbound copy failed", tc.version)
			}
			if !bytes.Equal(back, body) {
				t.Errorf("v%d: round trip %x, want %x", tc.version, back, body)
			}
		}
	}
}

// indexAfterCompID returns the offset just past the (single) component id of
// a one-component slot built by lodestoneSlot.
func indexAfterCompID(slot []byte) int {
	r := bytes.NewReader(slot)
	ReadVarInt(r)
	ReadVarInt(r)
	ReadVarInt(r)
	ReadVarInt(r)
	ReadVarInt(r)
	return len(slot) - r.Len()
}
