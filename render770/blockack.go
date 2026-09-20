package render770

// blockack.go renders block_changed_ack — the packet that retires a client's
// own block predictions. Without it the client keeps showing what it guessed
// (its own placement orientation, its own idea of a broken block) and quietly
// discards every correction the server sends for those positions.

import (
	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// IDBlockChangedAck is canonical-770 block_changed_ack. The id is 4 on every
// version tachyne serves (770-777), so the chain carries it unchanged.
const IDBlockChangedAck = 0x04

// BlockChangedAck renders the sequence the server has caught up to.
func BlockChangedAck(e attach.BlockAck) Packet {
	return Packet{IDBlockChangedAck, protocol.AppendVarInt(nil, e.Seq)}
}
