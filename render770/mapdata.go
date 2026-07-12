package render770

// mapdata.go — the map_item_data packet: color patches + decorations for a
// filled map a player is holding or viewing. The body layout is identical
// 770→776 (map id varint, scale, locked, optional decoration list, optional
// color patch with the width==0 sentinel); only the packet id shifts, which
// the translation chain remaps like any other. Decoration type ids come
// from the map_decoration_type registry (player=0, frame=1 — stable across
// served versions).

import (
	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// IDMapItemData is the canonical-770 clientbound play packet id.
const IDMapItemData = 0x2c

// MapItemData composes map_item_data from one viewer's map update.
func MapItemData(e attach.MapData) Packet {
	b := protocol.AppendVarInt(nil, e.MapID)
	b = append(b, byte(e.Scale))
	if e.Locked {
		b = append(b, 1)
	} else {
		b = append(b, 0)
	}
	if e.HasDecor {
		b = append(b, 1)
		b = protocol.AppendVarInt(b, int32(len(e.Decor)))
		for _, d := range e.Decor {
			b = protocol.AppendVarInt(b, d.Type)
			b = append(b, byte(d.X), byte(d.Z), d.Rot&15)
			if d.Name != "" {
				b = append(b, 1)
				b = append(b, chatNBT(d.Name)...)
			} else {
				b = append(b, 0)
			}
		}
	} else {
		b = append(b, 0)
	}
	// Color patch: a width byte of 0 means "no colors in this update".
	b = append(b, e.Width)
	if e.Width > 0 {
		b = append(b, e.Height, e.X, e.Y)
		b = protocol.AppendVarInt(b, int32(len(e.Colors)))
		b = append(b, e.Colors...)
	}
	return Packet{IDMapItemData, b}
}
