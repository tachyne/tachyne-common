package protocol

// bannernbt.go — a placed banner's update tag: the patterns list, each layer
// a compound of the pattern's registry NAME and the dye color name (NBT uses
// strings; only the network item component uses numeric ids). The base color
// is the block's own — never in the tag.

// BannerLayerNBT is one pattern layer for the tag.
type BannerLayerNBT struct {
	Pattern string // registry name, e.g. "minecraft:creeper"
	Color   string // dye name, e.g. "red"
}

// AppendBannerNBT appends a banner block entity's update tag as network NBT.
func AppendBannerNBT(b []byte, layers []BannerLayerNBT) []byte {
	b = append(b, NBTRoot()...)
	b = nbtName(append(b, nbtList), "patterns")
	b = append(b, nbtCompound)
	b = AppendI32(b, int32(len(layers)))
	for _, l := range layers {
		b = NBTString(b, "pattern", l.Pattern)
		b = NBTString(b, "color", l.Color)
		b = NBTEnd(b)
	}
	return NBTEnd(b)
}
