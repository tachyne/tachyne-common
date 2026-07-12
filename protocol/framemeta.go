package protocol

import "bytes"

// Item-frame entity metadata across versions. Canonically (770) the framed
// item is a Slot at index 8 and the rotation a VarInt at index 9; 26.x
// inserted HangingEntity's synced DIRECTION at index 8, shifting both by
// one — the same shift that moved the painting variant (paintingmeta.go).
// The value bytes stay canonical here: the chain's generic metadata walk
// remaps the Slot's item/component ids afterwards.

// frameIndexShift is how far the frame's own metadata indices move at a
// client version.
func frameIndexShift(version int32) byte {
	if version >= 775 {
		return 1 // 26.x: DIRECTION took index 8
	}
	return 0
}

// FixItemFrameMeta rewrites an item frame's canonical metadata body for the
// client version: the entry indices shift past the inserted DIRECTION
// field. Returns the input unchanged when nothing shifts or the body isn't
// the shape our engine composes (optional Slot entry at 8, VarInt rotation
// at 9).
func FixItemFrameMeta(version int32, body []byte) []byte {
	shift := frameIndexShift(version)
	if shift == 0 {
		return body
	}
	r := bytes.NewReader(body)
	eid, err := ReadVarInt(r)
	if err != nil {
		return body
	}
	out := AppendVarInt(nil, eid)
	for {
		idx, err := r.ReadByte()
		if err != nil {
			return body
		}
		if idx == 0xff {
			out = append(out, 0xff)
			break
		}
		typ, err := ReadVarInt(r)
		if err != nil {
			return body
		}
		out = append(out, idx+shift)
		out = AppendVarInt(out, typ)
		switch {
		case idx == 8 && typ == 7: // the framed item: copy the Slot verbatim
			if !copyFullSlot(r, &out, func(i int32) int32 { return i }, 770, false) {
				return body
			}
		case idx == 9 && typ == 1: // rotation: one varint
			rot, err := ReadVarInt(r)
			if err != nil {
				return body
			}
			out = AppendVarInt(out, rot)
		default:
			return body // not a shape we compose — leave untouched
		}
	}
	if r.Len() != 0 {
		return body
	}
	return out
}
