package protocol

// AppendSection appends one chunk section to b: the non-air block count, a
// block-state paletted container, then a single-valued biome container. states
// holds 4096 global block-state IDs indexed (y*16 + z)*16 + x.
func AppendSection(b []byte, states []uint32, biome int32) []byte {
	nonAir := 0
	for _, s := range states {
		if s != 0 {
			nonAir++
		}
	}
	b = AppendI16(b, int16(nonAir))
	b = appendStatePalette(b, states)
	// Biomes: single-valued palette (one biome per section for now).
	b = AppendU8(b, 0)
	b = AppendVarInt(b, biome)
	return b
}

// appendStatePalette writes a block-state paletted container. A uniform section
// uses the single-valued form (0 bits per entry); otherwise an indirect palette
// with a compacted long array. As of 1.21.5 the data array length is not sent —
// the client derives the long count from bits-per-entry.
func appendStatePalette(b []byte, states []uint32) []byte {
	palette := make([]uint32, 0, 16)
	idx := make(map[uint32]int, 16)
	indices := make([]int, len(states))
	for i, s := range states {
		j, ok := idx[s]
		if !ok {
			j = len(palette)
			idx[s] = j
			palette = append(palette, s)
		}
		indices[i] = j
	}

	if len(palette) == 1 {
		b = AppendU8(b, 0)
		return AppendVarInt(b, int32(palette[0]))
	}

	bits := paletteBits(len(palette))
	b = AppendU8(b, byte(bits))
	b = AppendVarInt(b, int32(len(palette)))
	for _, s := range palette {
		b = AppendVarInt(b, int32(s))
	}

	// Compacted long array: floor(64/bits) entries per long, no entry spans two
	// longs (Minecraft's packing, not bit-tight).
	per := 64 / bits
	var cur uint64
	n := 0
	for _, v := range indices {
		cur |= uint64(v) << (uint(n) * uint(bits))
		n++
		if n == per {
			b = AppendI64(b, int64(cur))
			cur, n = 0, 0
		}
	}
	if n > 0 {
		b = AppendI64(b, int64(cur))
	}
	return b
}

// paletteBits returns the bits-per-entry for an indirect block palette: a
// minimum of 4, otherwise the smallest width that covers the palette size.
func paletteBits(size int) int {
	bits := 4
	for (1 << bits) < size {
		bits++
	}
	return bits
}
