package render770

// border.go renders the world border.

import (
	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// IDInitBorder is canonical-770 initialize_world_border.
const IDInitBorder = 0x25

// borderAbsoluteMax is the packet's newAbsoluteMaxSize field — a protocol
// constant (vanilla's MAX_CENTER_COORDINATE), not world state, so it is not
// carried on the attach frame.
const borderAbsoluteMax = 29999984

// WorldBorder renders initialize_world_border.
//
// The single packet covers a stationary border and a moving one alike: a
// stationary border is simply one whose target equals its current size with a
// zero lerp time, which is why the engine never needs the five set_border_*
// packets.
func WorldBorder(e attach.WorldBorder) Packet {
	target := e.Target
	if target <= 0 {
		target = e.Size
	}
	b := protocol.AppendF64(nil, e.CenterX)
	b = protocol.AppendF64(b, e.CenterZ)
	b = protocol.AppendF64(b, e.Size) // oldSize: where it is now
	b = protocol.AppendF64(b, target) // newSize: where it is heading
	b = protocol.AppendVarLong(b, e.LerpMs)
	b = protocol.AppendVarInt(b, borderAbsoluteMax)
	b = protocol.AppendVarInt(b, e.WarnBlocks)
	return Packet{IDInitBorder, protocol.AppendVarInt(b, e.WarnTime)}
}
