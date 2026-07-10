package render770

// items.go renders the item/container event family: equipment, entity
// metadata, container windows, held-slot sync, and pickup animations.

import (
	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// Canonical-770 clientbound play packet IDs for this family.
const (
	IDWindowItems   = 0x12
	IDContainerData = 0x13
	IDSetSlot       = 0x14
	IDOpenWindow    = 0x34
	IDEntityMeta    = 0x5c
	IDSetEquipment  = 0x5f
	IDHeldSlot      = 0x62
	IDCollect       = 0x75
)

// AppendItemStack encodes a Slot: count, then id + component bytes when
// non-empty. Empty Components on a non-empty stack still needs the two
// zero-length component-array counts.
func AppendItemStack(b []byte, st attach.ItemStack) []byte {
	b = protocol.AppendVarInt(b, st.Count)
	if st.Count == 0 {
		return b
	}
	b = protocol.AppendVarInt(b, st.ID)
	if len(st.Components) == 0 {
		b = protocol.AppendVarInt(b, 0) // components to add
		return protocol.AppendVarInt(b, 0)
	}
	return append(b, st.Components...)
}

// Equipment renders set_equipment: all six slots, empty ones included —
// that's what clears a piece the viewer previously saw.
func Equipment(e attach.Equipment) Packet {
	b := protocol.AppendVarInt(nil, e.EID)
	for slot, st := range e.Slots {
		marker := byte(slot)
		if slot < len(e.Slots)-1 {
			marker |= 0x80 // top bit: another entry follows
		}
		b = append(b, marker)
		b = AppendItemStack(b, st)
	}
	return Packet{IDSetEquipment, b}
}

// EntityMeta renders set_entity_metadata from the opaque canonical list.
func EntityMeta(e attach.EntityMeta) Packet {
	b := protocol.AppendVarInt(nil, e.EID)
	return Packet{IDEntityMeta, append(b, e.Meta...)}
}

// WindowOpen renders open_screen.
func WindowOpen(e attach.WindowOpen) Packet {
	b := protocol.AppendVarInt(nil, e.ID)
	b = protocol.AppendVarInt(b, e.Menu)
	return Packet{IDOpenWindow, append(b, chatNBT(e.Title)...)}
}

// WindowItems renders set_container_content (full window + cursor).
func WindowItems(e attach.WindowItems) Packet {
	b := protocol.AppendVarInt(nil, e.ID)
	b = protocol.AppendVarInt(b, e.StateID)
	b = protocol.AppendVarInt(b, int32(len(e.Slots)))
	for _, st := range e.Slots {
		b = AppendItemStack(b, st)
	}
	return Packet{IDWindowItems, AppendItemStack(b, e.Cursor)}
}

// WindowSlot renders set_slot.
func WindowSlot(e attach.WindowSlot) Packet {
	b := protocol.AppendVarInt(nil, e.ID)
	b = protocol.AppendVarInt(b, e.StateID)
	b = protocol.AppendI16(b, int16(e.Slot))
	return Packet{IDSetSlot, AppendItemStack(b, e.Item)}
}

// WindowData renders set container property (progress bars etc.).
func WindowData(e attach.WindowData) Packet {
	b := protocol.AppendVarInt(nil, e.ID)
	b = protocol.AppendI16(b, int16(e.Prop))
	return Packet{IDContainerData, protocol.AppendI16(b, int16(e.Value))}
}

// HeldSync renders the server-set hotbar selection.
func HeldSync(e attach.HeldSync) Packet {
	return Packet{IDHeldSlot, protocol.AppendVarInt(nil, e.Slot)}
}

// Collect renders the pickup fly-to-player animation.
func Collect(e attach.Collect) Packet {
	b := protocol.AppendVarInt(nil, e.Collected)
	b = protocol.AppendVarInt(b, e.Collector)
	return Packet{IDCollect, protocol.AppendVarInt(b, e.Count)}
}
