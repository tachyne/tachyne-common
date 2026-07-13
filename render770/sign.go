package render770

// sign.go — sign block entities: the standalone block-entity update (text
// changes pushed to players who already hold the chunk) and the sign edit
// GUI opener. The chunk packet's block-entity section carries the same NBT
// for players (re)loading the chunk; both paths compose it via
// protocol.AppendSignNBT. Layouts are unchanged 770→776 (id remaps ride the
// translation chain; the sign NBT codec and both packet bodies are identical
// through 26.2).

import (
	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// Canonical-770 clientbound play packet IDs.
const (
	IDBlockEntityData = 0x06
	IDOpenSignEditor  = 0x35
)

// Block-entity type registry ids (identical in 1.21.11 and 26.2).
const (
	beTypeSign        = 7
	beTypeHangingSign = 8
	beTypeCampfire    = 33
)

func signSideNBT(s attach.SignSide) protocol.SignSideNBT {
	return protocol.SignSideNBT{Lines: s.Lines, Color: s.Color, Glow: s.Glow}
}

// SignData composes block_entity_data: position, block-entity type, and the
// sign's full update tag.
func SignData(e attach.SignText) Packet {
	b := protocol.AppendPosition(nil, int(e.X), int(e.Y), int(e.Z))
	typ := int32(beTypeSign)
	if e.Hanging {
		typ = beTypeHangingSign
	}
	b = protocol.AppendVarInt(b, typ)
	b = protocol.AppendSignNBT(b, signSideNBT(e.Front), signSideNBT(e.Back), e.Waxed)
	return Packet{IDBlockEntityData, b}
}

// CampfireData composes block_entity_data for a campfire: position, type,
// and the Items update tag (the client renders the food on the fire).
func CampfireData(e attach.CampfireItems) Packet {
	b := protocol.AppendPosition(nil, int(e.X), int(e.Y), int(e.Z))
	b = protocol.AppendVarInt(b, beTypeCampfire)
	b = protocol.AppendCampfireNBT(b, e.Items)
	return Packet{IDBlockEntityData, b}
}

// SignEditor composes open_sign_editor: position + which side to edit.
func SignEditor(e attach.SignEditor) Packet {
	b := protocol.AppendPosition(nil, int(e.X), int(e.Y), int(e.Z))
	if e.Front {
		b = append(b, 1)
	} else {
		b = append(b, 0)
	}
	return Packet{IDOpenSignEditor, b}
}
