package render770

// misc.go renders the stage-6a straggler families: game events, abilities,
// riding, velocity, trades, cursor item, difficulty, the command tree.

import (
	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// Canonical-770 clientbound play packet IDs for these families.
const (
	IDDifficulty  = 0x0a
	IDCommands    = 0x10
	IDGameEvent   = 0x22
	IDTrades      = 0x2d
	IDVehicleMove = 0x32
	IDAbilities   = 0x39
	IDCursorItem  = 0x59
	IDVelocity    = 0x5e
	IDPassengers  = 0x64
)

// GameEvent renders game_event (reason byte + float value).
func GameEvent(e attach.GameEvent) Packet {
	b := protocol.AppendU8(nil, byte(e.Event))
	return Packet{IDGameEvent, protocol.AppendF32(b, e.Value)}
}

// Abilities renders player_abilities with the vanilla default speeds.
func Abilities(e attach.Abilities) Packet {
	var flags int8
	if e.Invulnerable {
		flags |= 0x01
	}
	if e.Flying {
		flags |= 0x02
	}
	if e.MayFly {
		flags |= 0x04
	}
	if e.Creative {
		flags |= 0x08
	}
	b := protocol.AppendI8(nil, flags)
	b = protocol.AppendF32(b, 0.05)                        // flying speed
	return Packet{IDAbilities, protocol.AppendF32(b, 0.1)} // walking speed (fov)
}

// Passengers renders set_passengers.
func Passengers(e attach.Passengers) Packet {
	b := protocol.AppendVarInt(nil, e.Vehicle)
	b = protocol.AppendVarInt(b, int32(len(e.Riders)))
	for _, r := range e.Riders {
		b = protocol.AppendVarInt(b, r)
	}
	return Packet{IDPassengers, b}
}

// VehicleMove renders move_vehicle (server-authoritative snap-back).
func VehicleMove(e attach.VehicleMove) Packet {
	b := protocol.AppendF64(nil, e.X)
	b = protocol.AppendF64(b, e.Y)
	b = protocol.AppendF64(b, e.Z)
	b = protocol.AppendF32(b, e.Yaw)
	return Packet{IDVehicleMove, protocol.AppendF32(b, 0)}
}

// velocityUnit is the set_entity_velocity unit: 1/8000 block per tick.
const velocityUnit = 8000

// Velocity renders set_entity_velocity from a blocks/tick impulse.
func Velocity(e attach.Velocity) Packet {
	b := protocol.AppendVarInt(nil, e.EID)
	b = protocol.AppendI16(b, int16(e.VX*velocityUnit))
	b = protocol.AppendI16(b, int16(e.VY*velocityUnit))
	return Packet{IDVelocity, protocol.AppendI16(b, int16(e.VZ*velocityUnit))}
}

// Trades renders merchant_offers from the opaque canonical body.
func Trades(e attach.Trades) Packet { return Packet{IDTrades, e.Data} }

// CursorItem renders set_cursor_item.
func CursorItem(e attach.CursorItem) Packet {
	return Packet{IDCursorItem, AppendItemStack(nil, e.Item)}
}

// Difficulty renders change_difficulty.
func Difficulty(e attach.Difficulty) Packet {
	b := protocol.AppendU8(nil, uint8(e.Level))
	return Packet{IDDifficulty, protocol.AppendBool(b, e.Locked)}
}

// CommandTree renders the brigadier tree from the opaque canonical body.
func CommandTree(e attach.CommandTree) Packet { return Packet{IDCommands, e.Data} }

// IDSetDefaultSpawn is the world spawn packet: the compass's north.
const IDSetDefaultSpawn = 0x5a

// defaultSpawnGlobalPos is the first protocol whose set_default_spawn_position
// carries a GlobalPos rather than a bare block position. 1.21.9 rebuilt the
// packet around LevelData.RespawnData — the dimension the spawn is in, the
// block, and a yaw AND pitch — so anything from 773 up decodes a different
// shape. There is no body rewriter for this packet in the chain, which is why
// it has to be composed at the client's real version: a 26.x client handed the
// 1.21.5 body throws "Failed to decode packet clientbound/minecraft:
// set_default_spawn_position" and never finishes joining.
const defaultSpawnGlobalPos = 773

// DefaultSpawnData composes set_default_spawn_position for the given client
// protocol version.
func DefaultSpawnData(e attach.DefaultSpawn, version int32) Packet {
	if version < defaultSpawnGlobalPos {
		b := protocol.AppendPosition(nil, e.X, e.Y, e.Z)
		return Packet{IDSetDefaultSpawn, protocol.AppendF32(b, e.Angle)}
	}
	dim := e.Dim
	if dim == "" {
		dim = "minecraft:overworld" // the world spawn's home, and vanilla's default
	}
	b := protocol.AppendString(nil, dim) // GlobalPos: the dimension key, then the block
	b = protocol.AppendPosition(b, e.X, e.Y, e.Z)
	b = protocol.AppendF32(b, e.Angle)
	return Packet{IDSetDefaultSpawn, protocol.AppendF32(b, e.Pitch)}
}

// IDRespawn is the clientbound respawn packet (dimension switch / death).
const IDRespawn = 0x4b

// Respawn renders the respawn packet for a dimension event, keeping
// attributes + metadata (flag 0x03) — portal travel, not a fresh join.
func Respawn(e attach.Dimension) Packet {
	id, name := int32(protocol.DimensionOverworldID), "minecraft:overworld"
	switch e.Dim {
	case 1:
		id, name = int32(protocol.DimensionNetherID), "minecraft:the_nether"
	case 2:
		id, name = int32(protocol.DimensionEndID), "minecraft:the_end"
	}
	b := protocol.AppendVarInt(nil, id)
	b = protocol.AppendString(b, name)
	b = protocol.AppendI64(b, 0) // hashed seed
	b = protocol.AppendI8(b, int8(e.Gamemode))
	b = protocol.AppendU8(b, 0xFF)    // previous gamemode: none
	b = protocol.AppendBool(b, false) // debug
	b = protocol.AppendBool(b, false) // flat
	b = AppendDeathLocation(b, e.Death)
	b = protocol.AppendVarInt(b, 0)  // portal cooldown
	b = protocol.AppendVarInt(b, 63) // sea level
	return Packet{IDRespawn, protocol.AppendU8(b, 0x03)}
}

// dimensionKey is the dimension's ResourceKey<Level> identifier.
func dimensionKey(dim int32) string {
	switch dim {
	case 1:
		return "minecraft:the_nether"
	case 2:
		return "minecraft:the_end"
	}
	return "minecraft:overworld"
}

// AppendDeathLocation writes CommonPlayerSpawnInfo's Optional<GlobalPos>
// last death location: absent, or the dimension key and the block position.
func AppendDeathLocation(b []byte, d *attach.DeathPos) []byte {
	if d == nil {
		return protocol.AppendBool(b, false)
	}
	b = protocol.AppendBool(b, true)
	b = protocol.AppendString(b, dimensionKey(d.Dim))
	return protocol.AppendPosition(b, int(d.X), int(d.Y), int(d.Z))
}

// Entity animation packet IDs.
const (
	IDSwing        = 0x02
	IDEntityStatus = 0x1e
)

// EntityStatus renders entity_event.
func EntityStatus(e attach.EntityStatus) Packet {
	return Packet{IDEntityStatus, protocol.AppendU8(protocol.AppendI32(nil, e.EID), byte(e.Status))}
}

// Swing renders the main-hand arm-swing animation.
func Swing(e attach.Swing) Packet {
	action := byte(0) // SWING_MAIN_HAND
	if e.Hand == 1 {
		action = 3 // SWING_OFF_HAND
	}
	return Packet{IDSwing, protocol.AppendU8(protocol.AppendVarInt(nil, e.EID), action)}
}
