package protocol

import "bytes"

// update_attributes carries attribute registry ids, which shift between
// served versions, so the renderer writes canonical ids
// (canonicalAttributeIDs, generated from the canonical registry) and
// remapClientboundIDs rewrites them per client version. The table was
// hand-kept in 1.21.11's order once; 26.3 renumbers nearly every entry.

// AttributeID is the canonical registry id of an attribute name.
func AttributeID(name string) (int32, bool) {
	id, ok := canonicalAttributeIDs[name]
	return id, ok
}

// canonUpdateAttributes is clientbound update_attributes at canonical-770 ids.
const canonUpdateAttributes = 0x7c

// remapUpdateAttributes rewrites every attribute's registry id from canonical
// to the client version, and drops the attributes the version does not have
// (1.21.5 has no camera_distance or waypoint ranges). A malformed body comes
// back unchanged (don't-guess rule).
func remapUpdateAttributes(version int32, body []byte) []byte {
	r := bytes.NewReader(body)
	eid, err := ReadVarInt(r)
	if err != nil {
		return body
	}
	n, err := ReadVarInt(r)
	if err != nil || n < 0 {
		return body
	}
	out := AppendVarInt(make([]byte, 0, len(body)), eid)
	var entries [][]byte
	for i := int32(0); i < n; i++ {
		id, e1 := ReadVarInt(r)
		if e1 != nil {
			return body
		}
		var base [8]byte
		if _, e := r.Read(base[:]); e != nil {
			return body
		}
		m, e2 := ReadVarInt(r)
		if e2 != nil || m < 0 {
			return body
		}
		entry := AppendVarInt(nil, RemapID(RegAttribute, version, id))
		entry = append(entry, base[:]...)
		entry = AppendVarInt(entry, m)
		for j := int32(0); j < m; j++ {
			s, e3 := ReadString(r)
			if e3 != nil {
				return body
			}
			var amount [8]byte
			if _, e := r.Read(amount[:]); e != nil {
				return body
			}
			op, e4 := ReadVarInt(r)
			if e4 != nil {
				return body
			}
			entry = AppendString(entry, s)
			entry = append(entry, amount[:]...)
			entry = AppendVarInt(entry, op)
		}
		if IDPresent(RegAttribute, version, id) {
			entries = append(entries, entry)
		}
	}
	if r.Len() != 0 {
		return body
	}
	out = AppendVarInt(out, int32(len(entries)))
	for _, e := range entries {
		out = append(out, e...)
	}
	return out
}
