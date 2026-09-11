package protocol

import (
	"bytes"
	"testing"
)

// TestUpdateAttributesRemap: the body's canonical attribute ids become the
// client version's, and an attribute the version lacks is dropped.
func TestUpdateAttributesRemap(t *testing.T) {
	body := AppendVarInt(nil, 3)
	body = AppendVarInt(body, 3)
	body = AppendVarInt(body, 19) // max_health
	body = AppendF64(body, 20)
	body = AppendVarInt(body, 1)
	body = AppendString(body, "tachyne:x")
	body = AppendF64(body, 1)
	body = AppendVarInt(body, 2)
	body = AppendVarInt(body, 8) // camera_distance: absent in 1.21.5
	body = AppendF64(body, 4)
	body = AppendVarInt(body, 0)
	body = AppendVarInt(body, 22) // movement_speed
	body = AppendF64(body, 0.1)
	body = AppendVarInt(body, 0)

	for _, c := range []struct {
		version         int32
		maxHealth, move int32
		count           int32
	}{{770, 18, 21, 2}, {774, 19, 22, 3}, {776, 23, 26, 3}} {
		got, drop := remapClientboundIDs(c.version, canonUpdateAttributes, body)
		if drop {
			t.Fatalf("v%d: dropped", c.version)
		}
		r := bytes.NewReader(got)
		eid, _ := ReadVarInt(r)
		n, _ := ReadVarInt(r)
		first, _ := ReadVarInt(r)
		if eid != 3 || n != c.count || first != c.maxHealth {
			t.Fatalf("v%d: eid %d count %d first id %d, want 3 %d %d", c.version, eid, n, first, c.count, c.maxHealth)
		}
		// Skip base + one modifier, then read the next id (camera_distance on
		// versions that have it, else movement_speed).
		var f [8]byte
		r.Read(f[:])
		ReadVarInt(r)
		ReadString(r)
		r.Read(f[:])
		ReadVarInt(r)
		next, _ := ReadVarInt(r)
		if c.count == 2 && next != c.move {
			t.Fatalf("v%d: second id %d, want movement_speed %d", c.version, next, c.move)
		}
	}
	// Garbage comes back untouched.
	if got, _ := remapClientboundIDs(770, canonUpdateAttributes, []byte{1, 2}); !bytes.Equal(got, []byte{1, 2}) {
		t.Fatal("a malformed body must pass through unchanged")
	}
}
