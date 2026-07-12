package protocol

import (
	"bytes"
	"testing"
)

// The canonical frame body (Slot at 8, rotation at 9) shifts +1 at 26.x
// clients; the Slot value bytes ride through untouched.
func TestFixItemFrameMeta(t *testing.T) {
	body := AppendVarInt(nil, 42) // eid
	body = append(body, 8)        // item entry
	body = AppendVarInt(body, 7)  // Slot type
	body = AppendVarInt(body, 1)  // count
	body = AppendVarInt(body, 1104)
	body = AppendVarInt(body, 1) // one component: map_id
	body = AppendVarInt(body, 0)
	body = AppendVarInt(body, 37)
	body = AppendVarInt(body, 5)
	body = append(body, 9) // rotation entry
	body = AppendVarInt(body, 1)
	body = AppendVarInt(body, 3)
	body = append(body, 0xff)

	// 770-773: untouched.
	if !bytes.Equal(FixItemFrameMeta(770, body), body) {
		t.Fatal("770 body changed")
	}

	out := FixItemFrameMeta(776, body)
	r := bytes.NewReader(out)
	if eid, _ := ReadVarInt(r); eid != 42 {
		t.Fatalf("eid %d", eid)
	}
	idx, _ := r.ReadByte()
	typ, _ := ReadVarInt(r)
	if idx != 9 || typ != 7 {
		t.Fatalf("item entry %d/%d", idx, typ)
	}
	if n, _ := ReadVarInt(r); n != 1 {
		t.Fatalf("count %d", n)
	}
	if item, _ := ReadVarInt(r); item != 1104 {
		t.Fatalf("item %d", item)
	}
	ReadVarInt(r) // addC
	ReadVarInt(r) // remC
	if cid, _ := ReadVarInt(r); cid != 37 {
		t.Fatalf("component %d (must stay canonical for the chain)", cid)
	}
	ReadVarInt(r) // map number
	idx, _ = r.ReadByte()
	typ, _ = ReadVarInt(r)
	rot, _ := ReadVarInt(r)
	if idx != 10 || typ != 1 || rot != 3 {
		t.Fatalf("rotation entry %d/%d=%d", idx, typ, rot)
	}
	if end, _ := r.ReadByte(); end != 0xff || r.Len() != 0 {
		t.Fatal("terminator/trailing mismatch")
	}

	// Empty frame (rotation only) also shifts.
	empty := AppendVarInt(nil, 7)
	empty = append(empty, 9)
	empty = AppendVarInt(empty, 1)
	empty = AppendVarInt(empty, 0)
	empty = append(empty, 0xff)
	out = FixItemFrameMeta(776, empty)
	r = bytes.NewReader(out)
	ReadVarInt(r)
	if idx, _ := r.ReadByte(); idx != 10 {
		t.Fatalf("empty-frame rotation index %d", idx)
	}
}
