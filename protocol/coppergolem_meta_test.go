package protocol

import (
	"bytes"
	"testing"
)

// TestFixCopperGolemMeta: the INT-placeholder oxidation state at index 16 is
// restored to WEATHERING_COPPER_STATE (id 38) for a 26.2 client, value unchanged.
func TestFixCopperGolemMeta(t *testing.T) {
	// eid=7, then {index16, INT(1), VarInt(2)}, terminator 0xff.
	body := AppendVarInt(nil, 7)
	body = AppendU8(body, 16)
	body = AppendVarInt(body, metaTypeVarInt) // INT placeholder
	body = AppendVarInt(body, 2)              // WEATHERED
	body = AppendU8(body, 0xff)

	out := FixCopperGolemMeta(776, body)
	r := bytes.NewReader(out)
	if eid, _ := ReadVarInt(r); eid != 7 {
		t.Fatalf("eid = %d", eid)
	}
	idx, _ := r.ReadByte()
	typ, _ := ReadVarInt(r)
	val, _ := ReadVarInt(r)
	if idx != 16 || typ != 38 || val != 2 {
		t.Fatalf("got index=%d type=%d val=%d, want 16/38/2", idx, typ, val)
	}
	// Unknown/older versions leave it untouched.
	if got := FixCopperGolemMeta(999, body); !bytes.Equal(got, body) {
		t.Error("unknown version should pass through unchanged")
	}
}
