package protocol

import "testing"

// A block a client's version lacks must not keep its canonical id: that id is
// some other block there (a 26.2 client saw a white wool slab as fire and
// poplar planks as bamboo). It is shown as a stand-in of the same shape,
// keeping the properties the two share, or as air.
func TestAbsentBlocksShowAStandIn(t *testing.T) {
	const v = 776
	cs := CanonicalBlockState
	// Offsets from each block's DEFAULT state that stay inside the block: the
	// default of a slab, stairs, door or bed sits mid-range, leaves' is last.
	mid, last, only := []int32{-2, -1, 0, 1, 2}, []int32{-2, -1, 0}, []int32{0}
	for _, c := range []struct {
		newB, stand string
		offs        []int32
	}{
		{"white_wool_slab", "quartz_slab", mid}, {"red_concrete_stairs", "red_nether_brick_stairs", mid},
		{"poplar_planks", "birch_planks", only}, {"poplar_stairs", "birch_stairs", mid},
		{"poplar_door", "birch_door", mid}, {"yellow_poplar_leaves", "birch_leaves", last},
		{"straw_bed", "yellow_bed", mid},
	} {
		newB, stand := c.newB, c.stand
		if IDPresent(RegBlockState, v, cs(newB)) {
			t.Errorf("%s reported present on 26.2", newB)
		}
		// Same schema, same layout: every state lines up with the stand-in's.
		for _, off := range c.offs {
			if got, want := RemapID(RegBlockState, v, cs(newB)+off), RemapID(RegBlockState, v, cs(stand)+off); got != want {
				t.Errorf("%s+%d shows as %d, want %s+%d = %d", newB, off, got, stand, off, want)
			}
		}
	}
	if got := RemapID(RegBlockState, v, cs("shelf_mushroom")); got != 0 {
		t.Errorf("shelf_mushroom has no stand-in and must be air, got %d", got)
	}
	// Blocks 26.2 has are untouched by it.
	if !IDPresent(RegBlockState, v, cs("birch_planks")) || RemapID(RegBlockState, v, cs("stone")) != cs("stone") {
		t.Error("present blocks must translate as before")
	}
	// The canonical client needs nothing.
	if RemapID(RegBlockState, 777, cs("poplar_planks")) != cs("poplar_planks") {
		t.Error("26.3 must see poplar planks as themselves")
	}
}

// An item a client lacks is shown as its stand-in where there is one — an
// explorer map as a filled map (the map id travels with it, so it is the
// same map), a poplar plank as a birch one — and as air otherwise.
func TestAbsentItemsShowAStandIn(t *testing.T) {
	ci := CanonicalItem
	for item, stand := range map[string]string{
		"buried_treasure_map": "filled_map", "ocean_monument_map": "filled_map",
		"poplar_planks": "birch_planks", "white_wool_slab": "quartz_slab", "poplar_boat": "birch_boat",
	} {
		if IDPresent(RegItem, 776, ci(item)) {
			t.Errorf("%s reported present on 26.2", item)
		}
		if got, want := RemapID(RegItem, 776, ci(item)), RemapID(RegItem, 776, ci(stand)); got != want {
			t.Errorf("%s shows as %d, want %s (%d)", item, got, stand, want)
		}
		if RemapID(RegItem, 777, ci(item)) != ci(item) {
			t.Errorf("26.3 must see %s as itself", item)
		}
	}
}
