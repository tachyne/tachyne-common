package render770

// blockbreak.go renders block_destruction (the crack overlay).

import (
	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// IDBlockDestruction is canonical-770 block_destruction.
const IDBlockDestruction = 0x05

// BlockDestruction renders one breaker's progress on one block: entity id,
// position, then the stage byte.
func BlockDestruction(e attach.BlockBreakProgress) Packet {
	b := protocol.AppendVarInt(nil, e.EID)
	b = protocol.AppendPosition(b, int(e.X), int(e.Y), int(e.Z))
	return Packet{IDBlockDestruction, protocol.AppendU8(b, byte(e.Progress))}
}
