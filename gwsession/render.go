package gwsession

// render.go turns domain attach data into 770 wire format. Ported from the
// engine monolith's play.go (pre-rename) (the proven encoders) but consuming attach.ChunkBody
// instead of world types.

import (
	"sync/atomic"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

const (
	sections      = attach.Sections
	lightSections = sections + 2 // one below + one above the world
	minY          = attach.MinY

	heightmapMotionBlocking = 4
)

var skyAbove = func() [2048]byte {
	var a [2048]byte
	for i := range a {
		a[i] = 0xFF
	}
	return a
}()

var fullDark [2048]byte

// joinPacket builds Login/"Join Game". gamemode is the player's real mode
// (0 survival, 1 creative, 2 adventure, 3 spectator) so the client renders
// the correct HUD from the first frame instead of flashing survival.
func joinPacket(eid int32, gamemode int32, view int32) []byte {
	b := protocol.AppendI32(nil, eid)
	b = protocol.AppendBool(b, false) // hardcore
	b = protocol.AppendVarInt(b, 3)
	b = protocol.AppendString(b, "minecraft:overworld")
	b = protocol.AppendString(b, "minecraft:the_end")
	b = protocol.AppendString(b, "minecraft:the_nether")
	b = protocol.AppendVarInt(b, 100)  // max players
	b = protocol.AppendVarInt(b, view) // view distance
	b = protocol.AppendVarInt(b, view) // simulation distance
	b = protocol.AppendBool(b, false)  // reduced debug
	b = protocol.AppendBool(b, true)   // respawn screen
	b = protocol.AppendBool(b, false)  // limited crafting
	// SpawnInfo
	b = protocol.AppendVarInt(b, protocol.DimensionOverworldID)
	b = protocol.AppendString(b, "minecraft:overworld")
	b = protocol.AppendI64(b, 0)             // hashed seed
	b = protocol.AppendI8(b, int8(gamemode)) // game mode
	b = protocol.AppendU8(b, 0xFF)           // previous gamemode: none
	b = protocol.AppendBool(b, false)        // debug
	b = protocol.AppendBool(b, false)        // flat
	b = protocol.AppendBool(b, false)        // death location
	b = protocol.AppendVarInt(b, 0)          // portal cooldown
	b = protocol.AppendVarInt(b, 63)         // sea level
	b = protocol.AppendBool(b, false)        // enforces secure chat
	return b
}

var teleportID atomic.Int32

// syncPositionBody: Synchronize Player Position, all absolute.
func syncPositionBody(p attach.Pos) []byte {
	b := protocol.AppendVarInt(nil, teleportID.Add(1))
	b = protocol.AppendF64(b, p.X)
	b = protocol.AppendF64(b, p.Y)
	b = protocol.AppendF64(b, p.Z)
	b = protocol.AppendF64(b, 0)
	b = protocol.AppendF64(b, 0)
	b = protocol.AppendF64(b, 0)
	b = protocol.AppendF32(b, p.Yaw)
	b = protocol.AppendF32(b, p.Pitch)
	b = protocol.AppendI32(b, 0)
	return b
}

// syncPositionKeepView is syncPositionBody for a shard crossing: it sets the
// player's ABSOLUTE x/y/z (so the client lands at the real crossing position and
// not a stale/undefined height — the "tunnel at the wrong height" after the
// Respawn), but marks yaw+pitch AND velocity RELATIVE with zero deltas so the
// camera doesn't snap and MOMENTUM is kept (a sprint-jump carries across the
// seam instead of stopping dead). 1.21.5 teleport flags: 0x08 rel yaw, 0x10 rel
// pitch, 0x20/0x40/0x80 rel (delta) velocity x/y/z.
func syncPositionKeepView(p attach.Pos) []byte {
	b := protocol.AppendVarInt(nil, teleportID.Add(1))
	b = protocol.AppendF64(b, p.X)
	b = protocol.AppendF64(b, p.Y)
	b = protocol.AppendF64(b, p.Z)
	b = protocol.AppendF64(b, 0) // velocity deltas (relative → keep momentum)
	b = protocol.AppendF64(b, 0)
	b = protocol.AppendF64(b, 0)
	b = protocol.AppendF32(b, 0) // yaw delta (relative → unchanged)
	b = protocol.AppendF32(b, 0) // pitch delta (relative → unchanged)
	b = protocol.AppendI32(b, 0x08|0x10|0x20|0x40|0x80)
	return b
}

// packNibbles compresses 4096 light levels into 2048 bytes (even index low).
func packNibbles(levels []uint8) [2048]byte {
	var a [2048]byte
	for i := 0; i < 2048; i++ {
		a[i] = levels[2*i] | levels[2*i+1]<<4
	}
	return a
}

// appendHeightmap: one MOTION_BLOCKING entry, 9 bits/value, 7 per long.
func appendHeightmap(b []byte, hm []int16) []byte {
	const bits = 9
	const perLong = 64 / bits
	const nLongs = (256 + perLong - 1) / perLong
	var longs [nLongs]int64
	for i := 0; i < 256; i++ {
		v := int64(hm[i]) - int64(minY) + 1
		if v < 0 {
			v = 0
		}
		longs[i/perLong] |= (v & 0x1FF) << uint((i%perLong)*bits)
	}
	b = protocol.AppendVarInt(b, 1)
	b = protocol.AppendVarInt(b, heightmapMotionBlocking)
	b = protocol.AppendVarInt(b, nLongs)
	for _, l := range longs {
		b = protocol.AppendI64(b, l)
	}
	return b
}

// chunkPacket renders one domain chunk into Chunk Data and Update Light.
func chunkPacket(h attach.ChunkHeader, body *attach.ChunkBody) []byte {
	var col []byte
	for sec := 0; sec < sections; sec++ {
		biome := "minecraft:plains"
		if sec < len(h.Biomes) && h.Biomes[sec] != "" {
			biome = h.Biomes[sec]
		}
		col = protocol.AppendSection(col, body.BlockStates[sec*4096:(sec+1)*4096], protocol.BiomeID(biome))
	}

	b := protocol.AppendI32(nil, h.CX)
	b = protocol.AppendI32(b, h.CZ)
	b = appendHeightmap(b, body.Heightmap)
	b = protocol.AppendVarInt(b, int32(len(col)))
	b = append(b, col...)
	if len(h.BEs) > 0 {
		b = append(b, h.BEs...) // canonical wire form from the world
	} else {
		b = protocol.AppendVarInt(b, 0)
	}

	b = protocol.AppendVarInt(b, 1)
	b = protocol.AppendI64(b, int64((1<<lightSections)-1))
	b = protocol.AppendVarInt(b, 1)
	b = protocol.AppendI64(b, int64((1<<lightSections)-1))
	b = protocol.AppendVarInt(b, 0)
	b = protocol.AppendVarInt(b, 0)

	b = protocol.AppendVarInt(b, lightSections)
	for i := 0; i < lightSections; i++ {
		b = protocol.AppendVarInt(b, 2048)
		switch {
		case i == 0:
			b = append(b, fullDark[:]...)
		case i == lightSections-1:
			b = append(b, skyAbove[:]...)
		default:
			s := packNibbles(body.SkyLight[(i-1)*4096 : i*4096])
			b = append(b, s[:]...)
		}
	}
	b = protocol.AppendVarInt(b, lightSections)
	for i := 0; i < lightSections; i++ {
		b = protocol.AppendVarInt(b, 2048)
		if i == 0 || i == lightSections-1 {
			b = append(b, fullDark[:]...)
		} else {
			s := packNibbles(body.BlockLight[(i-1)*4096 : i*4096])
			b = append(b, s[:]...)
		}
	}
	return b
}
