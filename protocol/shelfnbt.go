package protocol

// shelfnbt.go — the wooden shelf's block-entity update tag (ShelfBlockEntity
// 1.21.9+): Items = list of {Slot, id, count} compounds, empties omitted,
// plus align_items_to_bottom. Shared by the chunk block-entity section and
// block_entity_data, like campfires.

// ShelfItemNBT is one shelf slot: item registry name + count (0 = empty).
type ShelfItemNBT struct {
	ID    string
	Count int32
}

// AppendShelfNBT appends a shelf's update tag as network NBT (nameless root).
func AppendShelfNBT(b []byte, items [3]ShelfItemNBT) []byte {
	b = append(b, NBTRoot()...)
	n := int32(0)
	for _, it := range items {
		if it.ID != "" && it.Count > 0 {
			n++
		}
	}
	b = nbtName(append(b, nbtList), "Items")
	b = append(b, nbtCompound)
	b = AppendI32(b, n)
	for slot, it := range items {
		if it.ID == "" || it.Count <= 0 {
			continue
		}
		b = NBTByte(b, "Slot", int8(slot))
		b = NBTString(b, "id", it.ID)
		b = NBTInt(b, "count", it.Count)
		b = NBTEnd(b)
	}
	b = NBTBool(b, "align_items_to_bottom", false)
	return NBTEnd(b)
}
