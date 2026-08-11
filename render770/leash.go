package render770

// leash.go renders the leash link between two entities.

import (
	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// IDSetEntityLink is canonical-770 set_entity_link. It sits immediately after
// set_entity_data (0x5c) in the clientbound play protocol.
const IDSetEntityLink = 0x5d

// EntityLink renders set_entity_link.
//
// Both fields are plain big-endian int32s — this is one of the few packets
// vanilla still writes with writeInt rather than a VarInt, so the usual
// AppendVarInt would silently produce a shorter packet the client misparses.
//
// The direction matters and is easy to get backwards: the SOURCE is the
// leashed entity and the DEST is whatever holds the lead. A dest of 0 is not
// entity 0, it is vanilla's sentinel for "no holder" — the packet that cuts a
// leash (ClientboundSetEntityLinkPacket passes a null destEntity, which the
// constructor turns into 0).
func EntityLink(e attach.EntityLink) Packet {
	b := protocol.AppendI32(nil, e.Leashed)
	return Packet{IDSetEntityLink, protocol.AppendI32(b, e.Holder)}
}
