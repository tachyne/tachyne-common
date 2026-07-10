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
	minY = attach.MinY

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

// bitsFor is the client's heightmap bits-per-entry rule: ceil(log2(n)) with a
// floor of 1 (vanilla MathHelper.ceillog2 applied to worldHeight+1).
func bitsFor(n int) int {
	bits := 1
	for 1<<bits < n {
		bits++
	}
	return bits
}

// appendHeightmap: one MOTION_BLOCKING entry. Bits-per-value derives from the
// dimension height exactly as the client computes it — 9 bits / 7-per-long
// for a vanilla 384 world, 11 bits for a tall earth world.
func appendHeightmap(b []byte, hm []int16, sections int) []byte {
	bits := bitsFor(sections*16 + 1)
	perLong := 64 / bits
	nLongs := (256 + perLong - 1) / perLong
	longs := make([]int64, nLongs)
	mask := int64(1)<<bits - 1
	for i := 0; i < 256; i++ {
		v := int64(hm[i]) - int64(minY) + 1
		if v < 0 {
			v = 0
		}
		longs[i/perLong] |= (v & mask) << uint((i%perLong)*bits)
	}
	b = protocol.AppendVarInt(b, 1)
	b = protocol.AppendVarInt(b, heightmapMotionBlocking)
	b = protocol.AppendVarInt(b, int32(nLongs))
	for _, l := range longs {
		b = protocol.AppendI64(b, l)
	}
	return b
}

// appendFullMask appends a BitSet with the low n bits set (varint long count,
// then longs). One long for a vanilla-height world; tall worlds spill over.
func appendFullMask(b []byte, n int) []byte {
	nLongs := (n + 63) / 64
	b = protocol.AppendVarInt(b, int32(nLongs))
	for i := 0; i < nLongs; i++ {
		if rem := n - i*64; rem >= 64 {
			b = protocol.AppendI64(b, -1)
		} else {
			b = protocol.AppendI64(b, int64(1)<<uint(rem)-1)
		}
	}
	return b
}

// appendMaskList appends a BitSet with exactly the listed bits set.
func appendMaskList(b []byte, bits []int, total int) []byte {
	nLongs := (total + 63) / 64
	longs := make([]int64, nLongs)
	for _, i := range bits {
		longs[i/64] |= int64(1) << uint(i%64)
	}
	// Trim trailing zero longs (vanilla BitSet.toLongArray does the same).
	for nLongs > 0 && longs[nLongs-1] == 0 {
		nLongs--
	}
	b = protocol.AppendVarInt(b, int32(nLongs))
	for i := 0; i < nLongs; i++ {
		b = protocol.AppendI64(b, longs[i])
	}
	return b
}

// sectionHasLight reports whether any block in the 4096-cell slice emits.
func sectionHasLight(levels []uint8) bool {
	for _, v := range levels {
		if v != 0 {
			return true
		}
	}
	return false
}

// chunkPacket renders one domain chunk into Chunk Data and Update Light. All
// sizes derive from the chunk's own section count (attach ChunkHeader), so a
// tall earth overworld and a vanilla-height nether render from one path.
func chunkPacket(h attach.ChunkHeader, body *attach.ChunkBody) []byte {
	sections := h.SectionCount()
	lightSections := sections + 2 // one below + one above the world
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
	b = appendHeightmap(b, body.Heightmap, sections)
	b = protocol.AppendVarInt(b, int32(len(col)))
	b = append(b, col...)
	if len(h.BEs) > 0 {
		b = append(b, h.BEs...) // canonical wire form from the world
	} else {
		b = protocol.AppendVarInt(b, 0)
	}

	// Light sections are TRIMMED in the overworld: sky arrays stop one section
	// above the highest terrain — the client's SkyLightSectionStorage returns
	// 15 for anything at/above its topmost stored section (decompiled-source
	// fact), so open sky costs nothing on the wire or in client heap. Block
	// light ships only sections that actually contain a lit cell (absent
	// sections default to 0). This is what keeps a tall (108-section) world
	// inside a stock client heap: untrimmed, light alone is ~450 KB/chunk ×
	// 4225 chunks at render distance 32 ≈ 1.9 GB — an instant client OOM.
	// The no-sky dims keep the legacy full-array form (24 sections, cheap).
	skyBits := make([]int, 0, lightSections)
	blkBits := make([]int, 0, lightSections)
	if h.Dim == 0 {
		maxH := minY - 1
		for _, hv := range body.Heightmap {
			if int(hv) > maxH {
				maxH = int(hv)
			}
		}
		topLit := (maxH-minY)/16 + 1 // one block-section above the surface
		if topLit > sections-1 {
			topLit = sections - 1
		}
		skyLit := topLit + 2 // +1: light index 0 is the below-world section
		if skyLit > lightSections {
			skyLit = lightSections
		}
		for i := 0; i < skyLit; i++ {
			skyBits = append(skyBits, i)
		}
		for i := 1; i < lightSections-1; i++ {
			if sectionHasLight(body.BlockLight[(i-1)*4096 : i*4096]) {
				blkBits = append(blkBits, i)
			}
		}
	} else {
		for i := 0; i < lightSections; i++ {
			skyBits = append(skyBits, i)
			blkBits = append(blkBits, i)
		}
	}

	b = appendMaskList(b, skyBits, lightSections) // sky-light mask
	b = appendMaskList(b, blkBits, lightSections) // block-light mask
	b = protocol.AppendVarInt(b, 0)               // empty sky-light mask
	b = protocol.AppendVarInt(b, 0)               // empty block-light mask

	b = protocol.AppendVarInt(b, int32(len(skyBits)))
	for _, i := range skyBits {
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
	b = protocol.AppendVarInt(b, int32(len(blkBits)))
	for _, i := range blkBits {
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
