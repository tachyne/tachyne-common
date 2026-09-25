package render770

// survival.go renders the survival-state event family: health/food, XP,
// status effects, the hurt flash, and the death screen.

import (
	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// Canonical-770 clientbound play packet IDs for this family.
const (
	IDHurtAnimation = 0x24
	IDDeathCombat   = 0x3d
	IDRemoveEffect  = 0x47
	IDSetExperience = 0x60
	IDUpdateHealth  = 0x61
	IDEntityEffect  = 0x7d
)

// Health renders set_health.
func Health(e attach.Health) Packet {
	b := protocol.AppendF32(nil, e.Health)
	b = protocol.AppendVarInt(b, e.Food)
	return Packet{IDUpdateHealth, protocol.AppendF32(b, e.Saturation)}
}

// XP renders the experience bar/level.
func XP(e attach.XP) Packet {
	b := protocol.AppendF32(nil, e.Progress)
	b = protocol.AppendVarInt(b, e.Level)
	return Packet{IDSetExperience, protocol.AppendVarInt(b, e.Total)}
}

// Effect renders a status-effect application or removal.
func Effect(e attach.Effect) Packet {
	b := protocol.AppendVarInt(nil, e.EID)
	b = protocol.AppendVarInt(b, e.ID)
	if e.Remove {
		return Packet{IDRemoveEffect, b}
	}
	b = protocol.AppendVarInt(b, e.Amp)
	b = protocol.AppendVarInt(b, e.Ticks)
	// ClientboundUpdateMobEffectPacket's flag byte: 1 ambient, 2 visible
	// (particles), 4 show icon, 8 blend (the darkness screen fade).
	flags := byte(0)
	if e.Ambient {
		flags |= 0x01
	}
	if !e.NoParticles {
		flags |= 0x02
	}
	if !e.NoIcon {
		flags |= 0x04
	}
	if e.Blend {
		flags |= 0x08
	}
	return Packet{IDEntityEffect, protocol.AppendU8(b, flags)}
}

// Hurt renders the hurt animation (red flash + directional camera tilt).
func Hurt(e attach.Hurt) Packet {
	b := protocol.AppendVarInt(nil, e.EID)
	return Packet{IDHurtAnimation, protocol.AppendF32(b, e.Yaw)}
}

// Death renders the death screen (death_combat_event).
func Death(e attach.Death) Packet {
	b := protocol.AppendVarInt(nil, e.EID)
	return Packet{IDDeathCombat, append(b, chatNBT(e.Message)...)}
}
