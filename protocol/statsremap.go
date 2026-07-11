package protocol

import "bytes"

// award_stats registry translation. The packet is VarInt count ×
// (VarInt statType, VarInt keyId, VarInt value); the key id lives in a
// per-type registry that shifts across versions: mined → block registry,
// crafted/used/broken/picked_up/dropped → items, killed/killed_by →
// entities, custom → the custom_stat registry. Stat-type ids themselves are
// identical on every served version (datagen-verified 770/774/776).

// canonAwardStats is the canonical-770 clientbound award_stats id.
const canonAwardStats = 0x03

// statKeyRegistry maps a stat-type id to the IDSpace its keys live in.
func statKeyRegistry(statType int32) (IDSpace, bool) {
	switch statType {
	case 0: // mined
		return RegBlock, true
	case 1, 2, 3, 4, 5: // crafted, used, broken, picked_up, dropped
		return RegItem, true
	case 6, 7: // killed, killed_by
		return RegEntity, true
	case 8: // custom
		return RegCustomStat, true
	}
	return 0, false
}

// remapAwardStats rewrites every entry's key id from canonical 774 to the
// client version. Unknown stat types or malformed bodies return the packet
// unchanged (don't-guess rule).
func remapAwardStats(version int32, body []byte) []byte {
	r := bytes.NewReader(body)
	n, err := ReadVarInt(r)
	if err != nil || n < 0 {
		return body
	}
	out := AppendVarInt(make([]byte, 0, len(body)), n)
	for i := int32(0); i < n; i++ {
		typ, e1 := ReadVarInt(r)
		key, e2 := ReadVarInt(r)
		val, e3 := ReadVarInt(r)
		if e1 != nil || e2 != nil || e3 != nil {
			return body
		}
		reg, ok := statKeyRegistry(typ)
		if !ok {
			return body
		}
		out = AppendVarInt(out, typ)
		out = AppendVarInt(out, RemapID(reg, version, key))
		out = AppendVarInt(out, val)
	}
	if r.Len() != 0 {
		return body
	}
	return out
}
