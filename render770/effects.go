package render770

// effects.go renders the world-effect event family: sounds, particles,
// world events, and block updates.

import (
	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// Canonical-770 clientbound play packet IDs for this family.
const (
	IDBlockUpdate = 0x08
	IDWorldEvent  = 0x28
	IDParticles   = 0x29
	IDSoundEffect = 0x6e
)

// Sound renders sound_effect with an inline (by-name) sound event.
func Sound(e attach.Sound) Packet {
	b := protocol.AppendVarInt(nil, 0) // 0 = inline sound event follows
	b = protocol.AppendString(b, e.Name)
	b = protocol.AppendBool(b, false) // no fixed range
	b = protocol.AppendVarInt(b, e.Category)
	b = protocol.AppendI32(b, int32(e.X*8)) // fixed-point ×8
	b = protocol.AppendI32(b, int32(e.Y*8))
	b = protocol.AppendI32(b, int32(e.Z*8))
	b = protocol.AppendF32(b, e.Volume)
	b = protocol.AppendF32(b, e.Pitch)
	return Packet{IDSoundEffect, protocol.AppendI64(b, 0)} // seed 0: client picks variants
}

// Particles renders level_particles for a payload-free particle type.
func Particles(e attach.Particles) Packet {
	b := protocol.AppendBool(nil, true) // long distance
	b = protocol.AppendBool(b, false)   // always show
	b = protocol.AppendF64(b, e.X)
	b = protocol.AppendF64(b, e.Y)
	b = protocol.AppendF64(b, e.Z)
	b = protocol.AppendF32(b, e.Spread) // offset x/y/z
	b = protocol.AppendF32(b, e.Spread)
	b = protocol.AppendF32(b, e.Spread)
	b = protocol.AppendF32(b, e.Speed)
	b = protocol.AppendI32(b, e.Count)
	return Packet{IDParticles, protocol.AppendVarInt(b, e.PID)}
}

// WorldFX renders world_event.
func WorldFX(e attach.WorldFX) Packet {
	b := protocol.AppendI32(nil, e.Event)
	b = protocol.AppendPosition(b, e.X, e.Y, e.Z)
	b = protocol.AppendI32(b, e.Data)
	return Packet{IDWorldEvent, protocol.AppendBool(b, false)} // not global
}

// BlockSet renders block_update.
func BlockSet(e attach.BlockSet) Packet {
	b := protocol.AppendPosition(nil, e.X, e.Y, e.Z)
	return Packet{IDBlockUpdate, protocol.AppendVarInt(b, int32(e.State))}
}
