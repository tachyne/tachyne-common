package protocol

// BlockForItem returns the default block state an item places and whether the
// item corresponds to a placeable block.
func BlockForItem(itemID int32) (uint32, bool) {
	// A few items place a block whose name differs from the item's (the
	// generated same-name table misses these). Buckets place fluid sources.
	switch itemID {
	case 951: // water_bucket
		return 86, true // water source (level 0)
	case 952: // lava_bucket
		return 102, true // lava source (level 0)
	}
	s, ok := itemToBlock[itemID]
	return s, ok
}
