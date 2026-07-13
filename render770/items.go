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
	last := 6 // players never carry the saddle slot
	if e.SendSaddle {
		last = 7
	}
	for slot := 0; slot <= last; slot++ {
		marker := byte(slot)
		if slot < last {
			marker |= 0x80 // top bit: another entry follows
		}
		b = append(b, marker)
		b = AppendItemStack(b, e.Slots[slot])
	}
	return Packet{IDSetEquipment, b}
}

// IDWaypoint776 is the clientbound tracked_waypoint id at protocol 776
// (26.2). Derived from the 26.2 GameProtocols clientbound registration
// order — validated by reproducing every known 1.21.5 id with the same
// count. New in 1.21.6, so no 770 equivalent exists.
const IDWaypoint776 = 0x8a

// WaypointBody composes the tracked_waypoint packet body (version-independent
// — UUID/identifier/varints/RGB need no id remap). The caller sends it raw at
// the 776 packet id for 26.2 clients and drops it for older ones.
func WaypointBody(e attach.Waypoint) []byte {
	b := protocol.AppendVarInt(nil, int32(e.Op)) // operation ordinal
	b = append(b, 1)                             // Either.left: a UUID identifier
	b = append(b, e.UUID[:]...)
	style := e.Style
	if style == "" {
		style = "minecraft:default"
	}
	b = protocol.AppendString(b, style) // icon style asset id
	if e.HasColor {
		b = append(b, 1, byte(e.Color>>16), byte(e.Color>>8), byte(e.Color)) // optional RGB
	} else {
		b = append(b, 0)
	}
	if e.Op == 1 { // UNTRACK: empty waypoint (type 0, no contents)
		return append(b, 0)
	}
	b = append(b, 1) // type VEC3I
	b = protocol.AppendVarInt(b, e.X)
	b = protocol.AppendVarInt(b, e.Y)
	return protocol.AppendVarInt(b, e.Z)
}

// IDOpenBook is the canonical-770 clientbound open_book id.
const IDOpenBook = 0x33

// OpenBook renders open_book: the hand enum alone.
func OpenBook(e attach.OpenBook) Packet {
	return Packet{IDOpenBook, protocol.AppendVarInt(nil, e.Hand)}
}

// IDOpenHorseWindow is the canonical-770 clientbound open_horse_window id.
const IDOpenHorseWindow = 0x23

// HorseScreen renders open_horse_window: BYTE window id (unlike open_screen's
// varint), varint chest columns, i32 mount entity id.
func HorseScreen(e attach.HorseScreen) Packet {
	b := []byte{byte(e.ID)}
	b = protocol.AppendVarInt(b, e.Columns)
	b = protocol.AppendI32(b, e.EID)
	return Packet{IDOpenHorseWindow, b}
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
