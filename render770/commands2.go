package render770

import (
	"github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// Canonical 770 ids.
const (
	IDSetCamera    = 0x56
	IDTickingState = 0x78
	IDTickingStep  = 0x79
	IDTransfer     = 0x7a
)

// Transfer renders transfer: host, then port.
func Transfer(e attach.Transfer) Packet {
	return Packet{IDTransfer, protocol.AppendVarInt(protocol.AppendString(nil, e.Host), e.Port)}
}

// Camera renders set_camera.
func Camera(e attach.Camera) Packet {
	return Packet{IDSetCamera, protocol.AppendVarInt(nil, e.EID)}
}

// TickingState renders ticking_state: the rate, then frozen.
func TickingState(e attach.TickingState) Packet {
	return Packet{IDTickingState, protocol.AppendBool(protocol.AppendF32(nil, e.Rate), e.Frozen)}
}

// TickingStep renders ticking_step.
func TickingStep(e attach.TickingStep) Packet {
	return Packet{IDTickingStep, protocol.AppendVarInt(nil, e.Steps)}
}

// IDCommandSuggestions is command_suggestions at canonical 770.
const IDCommandSuggestions = 0x0f

// CommandSuggestions renders command_suggestions: id, the replaced range,
// then each match with no tooltip.
func CommandSuggestions(e attach.Suggestions) Packet {
	b := protocol.AppendVarInt(nil, e.ID)
	b = protocol.AppendVarInt(b, e.Start)
	b = protocol.AppendVarInt(b, e.Length)
	b = protocol.AppendVarInt(b, int32(len(e.Matches)))
	for _, m := range e.Matches {
		b = protocol.AppendString(b, m)
		b = protocol.AppendBool(b, false)
	}
	return Packet{IDCommandSuggestions, b}
}

// Block entity type ids in the canonical (1.21.11) numbering the chain maps
// from.
const (
	beTypeBrushable    = 41
	beTypeTrialSpawner = 44
	beTypeVault        = 45
)

// BlockDisplay renders block_entity_data carrying a vault's, trial spawner's
// or suspicious block's update tag.
func BlockDisplay(e attach.BlockDisplay) (Packet, bool) {
	b := protocol.AppendPosition(nil, int(e.Pos[0]), int(e.Pos[1]), int(e.Pos[2]))
	item := func(b []byte, name string) []byte {
		b = protocol.NBTString(b, "id", e.Name)
		return protocol.NBTEnd(protocol.NBTInt(b, "count", max(e.Count, 1)))
	}
	switch e.Kind {
	case attach.DisplayVault:
		b = protocol.AppendVarInt(b, beTypeVault)
		b = append(b, protocol.NBTRoot()...)
		b = protocol.NBTCompound(b, "shared_data")
		if e.Name != "" {
			b = item(protocol.NBTCompound(b, "display_item"), e.Name)
		}
		b = protocol.NBTEnd(protocol.NBTEnd(b))
	case attach.DisplayTrialSpawner:
		b = protocol.AppendVarInt(b, beTypeTrialSpawner)
		b = append(b, protocol.NBTRoot()...)
		if e.NextAt != 0 {
			b = protocol.NBTLong(b, "next_mob_spawns_at", e.NextAt)
		}
		if e.Name != "" {
			b = protocol.NBTCompound(b, "spawn_data")
			b = protocol.NBTCompound(b, "entity")
			b = protocol.NBTEnd(protocol.NBTString(b, "id", e.Name))
			b = protocol.NBTEnd(b)
		}
		b = protocol.NBTEnd(b)
	case attach.DisplayBrushable:
		b = protocol.AppendVarInt(b, beTypeBrushable)
		b = append(b, protocol.NBTRoot()...)
		if e.HitDir > 0 {
			b = protocol.NBTByte(b, "hit_direction", int8(e.HitDir-1))
		}
		if e.Name != "" {
			b = item(protocol.NBTCompound(b, "item"), e.Name)
		}
		b = protocol.NBTEnd(b)
	default:
		return Packet{}, false
	}
	return Packet{IDBlockEntityData, b}, true
}
