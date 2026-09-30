package render770

// effects.go renders the world-effect event family: sounds, particles,
// world events, and block updates.

import (
	"math"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// Canonical-770 clientbound play packet IDs for this family.
const (
	IDBlockUpdate = 0x08
	IDWorldEvent  = 0x28
	IDBlockEvent  = 0x07 // block_event: position, action u8, param u8, block varint
	IDParticles   = 0x29
	IDSoundEffect = 0x6e
	IDStopSound   = 0x70
)

// StopSound renders stop_sound: a flags byte (1 = a source follows, 2 = a
// sound name follows), the source enum, then the sound's identifier.
func StopSound(e attach.StopSound) Packet {
	var flags byte
	if e.Category >= 0 {
		flags |= 1
	}
	if e.Name != "" {
		flags |= 2
	}
	b := []byte{flags}
	if e.Category >= 0 {
		b = protocol.AppendVarInt(b, e.Category)
	}
	if e.Name != "" {
		b = protocol.AppendString(b, e.Name)
	}
	return Packet{IDStopSound, b}
}

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

// IDSoundEntity is sound_entity at canonical 770.
const IDSoundEntity = 0x6d

// SoundEntity renders sound_entity: the sound inline, its source, the entity
// it follows, volume, pitch and seed.
func SoundEntity(e attach.Sound) Packet {
	b := protocol.AppendVarInt(nil, 0)
	b = protocol.AppendString(b, e.Name)
	b = protocol.AppendBool(b, false)
	b = protocol.AppendVarInt(b, e.Category)
	b = protocol.AppendVarInt(b, e.EID)
	b = protocol.AppendF32(b, e.Volume)
	b = protocol.AppendF32(b, e.Pitch)
	return Packet{IDSoundEntity, protocol.AppendI64(b, 0)}
}

// Particles renders level_particles (ClientboundLevelParticlesPacket):
// overrideLimiter, alwaysShow, position, offsets, speed, count, particle.
// The particle's options follow its id in 770 form; the four types that
// took options in 26.x (dragon_breath, effect, instant_effect, flash) carry
// theirs after it when the frame has Options, which the translation chain
// reads (and leaves off for a 770 client).
func Particles(e attach.Particles) Packet {
	dx, dy, dz := e.DX, e.DY, e.DZ
	if dx == 0 && dy == 0 && dz == 0 {
		dx, dy, dz = e.Spread, e.Spread, e.Spread // an engine that sends one spread
	}
	b := protocol.AppendBool(nil, e.Force)
	b = protocol.AppendBool(b, e.AlwaysShow)
	b = protocol.AppendF64(b, e.X)
	b = protocol.AppendF64(b, e.Y)
	b = protocol.AppendF64(b, e.Z)
	b = protocol.AppendF32(b, dx)
	b = protocol.AppendF32(b, dy)
	b = protocol.AppendF32(b, dz)
	b = protocol.AppendF32(b, e.Speed)
	b = protocol.AppendI32(b, e.Count)
	b = protocol.AppendVarInt(b, e.PID)
	return Packet{IDParticles, AppendParticleOptions(b, e.PID, e.Options)}
}

// AppendParticleOptions appends a canonical particle's options (see
// Particles): nothing for a type without them; a type with them and no
// Options gets their zero value (an item particle an empty stack, which the
// chain drops).
func AppendParticleOptions(b []byte, pid int32, o *attach.ParticleOptions) []byte {
	var z attach.ParticleOptions
	if o == nil {
		o = &z
	}
	switch pid {
	case protocol.ParticleBlock770, protocol.ParticleBlockMarker770, protocol.ParticleFallingDust770,
		protocol.ParticleDustPillar770, protocol.ParticleBlockCrumble770:
		return protocol.AppendVarInt(b, o.State)
	case protocol.ParticleDust770:
		b = protocol.AppendI32(b, o.Color)
		return protocol.AppendF32(b, o.Scale)
	case protocol.ParticleDustColorTransition770:
		b = protocol.AppendI32(b, o.Color)
		b = protocol.AppendI32(b, o.ToColor)
		return protocol.AppendF32(b, o.Scale)
	case protocol.ParticleEntityEffect770, protocol.ParticleTintedLeaves770:
		return protocol.AppendI32(b, o.Color)
	case protocol.ParticleSculkCharge770:
		return protocol.AppendF32(b, o.Roll)
	case protocol.ParticleShriek770:
		return protocol.AppendVarInt(b, o.Delay)
	case protocol.ParticleTrail770:
		b = protocol.AppendF64(b, o.TX)
		b = protocol.AppendF64(b, o.TY)
		b = protocol.AppendF64(b, o.TZ)
		b = protocol.AppendI32(b, o.Color)
		return protocol.AppendVarInt(b, o.Ticks)
	case protocol.ParticleVibration770:
		if o.TargetEID != 0 {
			b = protocol.AppendVarInt(b, 1) // minecraft:entity
			b = protocol.AppendVarInt(b, o.TargetEID)
			b = protocol.AppendF32(b, o.TargetYOffset)
		} else {
			b = protocol.AppendVarInt(b, 0) // minecraft:block
			b = protocol.AppendPosition(b, int(math.Floor(o.TX)), int(math.Floor(o.TY)), int(math.Floor(o.TZ)))
		}
		return protocol.AppendVarInt(b, o.Ticks)
	case protocol.ParticleItem770:
		var st attach.ItemStack
		if o.Item != nil {
			st = *o.Item
		}
		return AppendItemStack(b, st)
	case protocol.ParticleDragonBreath770:
		if o != &z {
			b = protocol.AppendF32(b, o.Power)
		}
	case protocol.ParticleFlash770:
		if o != &z {
			b = protocol.AppendI32(b, o.Color)
		}
	case protocol.ParticleEffect770, protocol.ParticleInstantEffect770:
		if o != &z {
			b = protocol.AppendI32(b, o.Color)
			b = protocol.AppendF32(b, o.Power)
		}
	}
	return b
}

// WorldFX renders world_event (level_event): event, position, data, and
// whether it is a global event.
func WorldFX(e attach.WorldFX) Packet {
	b := protocol.AppendI32(nil, e.Event)
	b = protocol.AppendPosition(b, e.X, e.Y, e.Z)
	b = protocol.AppendI32(b, e.Data)
	return Packet{IDWorldEvent, protocol.AppendBool(b, e.Global)}
}

// BlockEvent renders block_event.
func BlockEvent(e attach.BlockEvent) Packet {
	b := protocol.AppendPosition(nil, int(e.X), int(e.Y), int(e.Z))
	b = append(b, e.Action, e.Param)
	return Packet{IDBlockEvent, protocol.AppendVarInt(b, e.Block)}
}

// BlockSet renders block_update.
func BlockSet(e attach.BlockSet) Packet {
	b := protocol.AppendPosition(nil, e.X, e.Y, e.Z)
	return Packet{IDBlockUpdate, protocol.AppendVarInt(b, int32(e.State))}
}
