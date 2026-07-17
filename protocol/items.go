package protocol

// BlockForItem returns the default block state an item places and whether the
// item corresponds to a placeable block. Buckets are NOT placeable items —
// the engine handles fluid pour/scoop itself. (A literal-id special case for
// them once lived here and outlived an item-id migration, colliding with
// wheat_seeds/wheat — never key item behavior on literal ids.)
func BlockForItem(itemID int32) (uint32, bool) {
	s, ok := itemToBlock[itemID]
	return s, ok
}
