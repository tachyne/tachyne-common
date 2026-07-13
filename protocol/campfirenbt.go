package protocol

// campfirenbt.go — the campfire block entity's update tag. Vanilla syncs
// ONLY the Items list (cook progress stays server-side); the client renders
// the food models on the fire from it. Shared by both delivery paths, like
// signs: the chunk packet's block-entity section and block_entity_data.

// AppendCampfireNBT appends a campfire's update tag as network NBT (nameless
// root compound): Items = list of {Slot, id, count} compounds, empties
// omitted. Item ids are registry names ("minecraft:cod").
func AppendCampfireNBT(b []byte, items [4]string) []byte {
	b = append(b, NBTRoot()...)
	n := 0
	for _, it := range items {
		if it != "" {
			n++
		}
	}
	b = nbtName(append(b, nbtList), "Items")
	b = append(b, nbtCompound)
	b = AppendI32(b, int32(n))
	for slot, it := range items {
		if it == "" {
			continue
		}
		b = NBTByte(b, "Slot", int8(slot))
		b = NBTString(b, "id", it)
		b = NBTInt(b, "count", 1)
		b = NBTEnd(b)
	}
	return NBTEnd(b)
}
