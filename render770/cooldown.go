package render770

// cooldown.go renders cooldown (the item-use cooldown sweep).

import (
	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// IDCooldown is canonical-770 cooldown.
const IDCooldown = 0x16

// Cooldown renders one cooldown group starting for the viewer: the group
// identifier, then the duration in ticks (0 ends it).
func Cooldown(e attach.ItemCooldown) Packet {
	b := protocol.AppendString(nil, e.Group)
	return Packet{IDCooldown, protocol.AppendVarInt(b, e.Ticks)}
}
