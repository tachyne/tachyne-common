package protocol

import "bytes"

// Attribute registry (canonical 1.21.11 order). update_attributes carries
// registry ids, which shift between served versions (26.2 inserts eight;
// 1.21.5 lacks three), so the renderer writes canonical ids and
// remapClientboundIDs rewrites them per client version.
var attributeIDs = map[string]int32{
	"minecraft:armor": 0, "minecraft:armor_toughness": 1, "minecraft:attack_damage": 2,
	"minecraft:attack_knockback": 3, "minecraft:attack_speed": 4, "minecraft:block_break_speed": 5,
	"minecraft:block_interaction_range": 6, "minecraft:burning_time": 7, "minecraft:camera_distance": 8,
	"minecraft:explosion_knockback_resistance": 9, "minecraft:entity_interaction_range": 10,
	"minecraft:fall_damage_multiplier": 11, "minecraft:flying_speed": 12, "minecraft:follow_range": 13,
	"minecraft:gravity": 14, "minecraft:jump_strength": 15, "minecraft:knockback_resistance": 16,
	"minecraft:luck": 17, "minecraft:max_absorption": 18, "minecraft:max_health": 19,
	"minecraft:mining_efficiency": 20, "minecraft:movement_efficiency": 21, "minecraft:movement_speed": 22,
	"minecraft:oxygen_bonus": 23, "minecraft:safe_fall_distance": 24, "minecraft:scale": 25,
	"minecraft:sneaking_speed": 26, "minecraft:spawn_reinforcements": 27, "minecraft:step_height": 28,
	"minecraft:submerged_mining_speed": 29, "minecraft:sweeping_damage_ratio": 30, "minecraft:tempt_range": 31,
	"minecraft:water_movement_efficiency": 32, "minecraft:waypoint_transmit_range": 33,
	"minecraft:waypoint_receive_range": 34,
}

// AttributeID is the canonical registry id of an attribute name.
func AttributeID(name string) (int32, bool) {
	id, ok := attributeIDs[name]
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
