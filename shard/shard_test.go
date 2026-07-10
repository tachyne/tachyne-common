package shard

import "testing"

// testbed is the first two-pod topology: two edge-adjacent 16x16-chunk
// (256x256 block) tiles, split at chunk x=0 (block x=0). Tiles are >2x the
// ~128-block view radius, so each has an "unaware core" and an approach zone —
// you walk out of view of the neighbour, then into it, then cross. SID 0 is the
// LEFT/west tile, SID 1 the RIGHT/east tile; they share one vertical seam.
// Chunks outside both are Unowned (the world's edge).
func testbed() Map {
	return Map{Version: 1, Regions: []Region{
		{SID: 0, MinCX: -16, MinCZ: -8, W: 16, H: 16}, // west: chunks [-16,-1]x[-8,7], blocks [-256,0)x[-128,128)
		{SID: 1, MinCX: 0, MinCZ: -8, W: 16, H: 16},   // east: chunks [0,15]x[-8,7], blocks [0,256)x[-128,128)
	}}
}

func TestShardOfRegions(t *testing.T) {
	m := testbed()
	if err := m.Validate(); err != nil {
		t.Fatalf("testbed invalid: %v", err)
	}
	for _, tc := range []struct {
		bx, bz int32 // block coords
		want   int32
	}{
		{-200, 0, 0},       // deep in the west core (unaware zone)
		{-1, 0, 0},         // just west of the seam -> chunk -1 -> SID 0
		{0, 0, 1},          // exactly the seam (block 0 -> chunk 0) -> east tile
		{200, 0, 1},        // deep in the east core
		{-256, 0, 0},       // west edge chunk -16
		{255, 127, 1},      // far NE corner of the east tile (block 255 -> chunk 15)
		{-257, 0, Unowned}, // one block west of the world -> chunk -17 -> void
		{256, 0, Unowned},  // one block east of the world -> chunk 16 -> void
		{0, 128, Unowned},  // one block north of the world -> chunk 8 -> void
		{0, -129, Unowned}, // south of the world -> chunk -9 -> void
	} {
		cx, cz := tc.bx>>4, tc.bz>>4 // arithmetic shift = floor for block->chunk
		if got := m.ShardOf(0, cx, cz); got != tc.want {
			t.Errorf("ShardOf block(%d,%d)->chunk(%d,%d)=%d want %d", tc.bx, tc.bz, cx, cz, got, tc.want)
		}
	}
}

func TestNeighbours(t *testing.T) {
	m := testbed()
	if got := m.Neighbours(0); len(got) != 1 || got[0] != 1 {
		t.Errorf("Neighbours(0)=%v want [1]", got)
	}
	if got := m.Neighbours(1); len(got) != 1 || got[0] != 0 {
		t.Errorf("Neighbours(1)=%v want [0]", got)
	}

	// A detached third tile has no neighbours (gap between it and the others).
	detached := Map{Version: 1, Regions: []Region{
		{SID: 0, MinCX: 0, MinCZ: 0, W: 6, H: 6},
		{SID: 1, MinCX: 100, MinCZ: 100, W: 6, H: 6}, // far away
	}}
	if got := detached.Neighbours(0); len(got) != 0 {
		t.Errorf("detached Neighbours(0)=%v want []", got)
	}

	// Corner-only contact is NOT adjacency (a zero-length edge can't be crossed).
	corner := Map{Version: 1, Regions: []Region{
		{SID: 0, MinCX: 0, MinCZ: 0, W: 6, H: 6}, // [0,6)x[0,6)
		{SID: 1, MinCX: 6, MinCZ: 6, W: 6, H: 6}, // touches only at corner (6,6)
	}}
	if got := corner.Neighbours(0); len(got) != 0 {
		t.Errorf("corner-touch Neighbours(0)=%v want [] (not adjacent)", got)
	}

	// A horizontal seam (tiles stacked N/S) is adjacency too.
	stacked := Map{Version: 1, Regions: []Region{
		{SID: 0, MinCX: 0, MinCZ: 0, W: 6, H: 6},
		{SID: 1, MinCX: 0, MinCZ: 6, W: 6, H: 6}, // directly north, shares z=6 edge
	}}
	if got := stacked.Neighbours(0); len(got) != 1 || got[0] != 1 {
		t.Errorf("stacked Neighbours(0)=%v want [1]", got)
	}
}

func TestNeighboursWithinCorner(t *testing.T) {
	// A 2x2 grid of 16x16 tiles meeting at the corner (0,0):
	//   NW | NE          SIDs: NW=0 NE=1 SW=2 SE=3
	//   ---+---
	//   SW | SE
	grid := Map{Version: 1, Regions: []Region{
		{SID: 0, MinCX: -16, MinCZ: 0, W: 16, H: 16},   // NW
		{SID: 1, MinCX: 0, MinCZ: 0, W: 16, H: 16},     // NE
		{SID: 2, MinCX: -16, MinCZ: -16, W: 16, H: 16}, // SW
		{SID: 3, MinCX: 0, MinCZ: -16, W: 16, H: 16},   // SE
	}}
	if err := grid.Validate(); err != nil {
		t.Fatalf("2x2 grid invalid: %v", err)
	}

	// HANDOVER neighbours of NW = edge-adjacent only: NE (shares x=0) and SW
	// (shares z=0). NOT the diagonal SE (corner-only touch).
	if got := grid.Neighbours(0); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("Neighbours(NW)=%v want [1 2] (edges only, no diagonal)", got)
	}

	// AWARENESS neighbours of NW at a corner = all THREE others, because a view
	// box near (0,0) overlaps NE, SW AND the diagonal SE. This is Wesley's
	// "notify the adjoining 3 shards".
	if got := grid.NeighboursWithin(0, 8); len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Errorf("NeighboursWithin(NW,8)=%v want [1 2 3] (incl. diagonal SE)", got)
	}
	// Even r=0 catches the diagonal (corner touch = gap 0 on both axes).
	if got := grid.NeighboursWithin(0, 0); len(got) != 3 {
		t.Errorf("NeighboursWithin(NW,0)=%v want 3 (edge+corner touch)", got)
	}

	// In the 2-tile testbed there is no 4-way corner, so awareness == handover.
	tb := testbed()
	if got := tb.NeighboursWithin(0, 8); len(got) != 1 || got[0] != 1 {
		t.Errorf("testbed NeighboursWithin(0,8)=%v want [1]", got)
	}

	// A tile across a gap wider than r is NOT within awareness.
	gapped := Map{Version: 1, Regions: []Region{
		{SID: 0, MinCX: 0, MinCZ: 0, W: 16, H: 16},
		{SID: 1, MinCX: 26, MinCZ: 0, W: 16, H: 16}, // 10-chunk void gap
	}}
	if got := gapped.NeighboursWithin(0, 8); len(got) != 0 {
		t.Errorf("across a 10-chunk gap, NeighboursWithin(0,8)=%v want [] (gap>r)", got)
	}
	if got := gapped.NeighboursWithin(0, 10); len(got) != 1 {
		t.Errorf("with r=10 the gapped tile is within, got %v", got)
	}
}

func TestValidate(t *testing.T) {
	if err := testbed().Validate(); err != nil {
		t.Fatalf("valid testbed rejected: %v", err)
	}
	for name, bad := range map[string]Map{
		"version<1":  {Version: 0, Regions: []Region{{SID: 0, W: 6, H: 6}}},
		"no regions": {Version: 1, Regions: nil},
		"bad sid":    {Version: 1, Regions: []Region{{SID: PlayerSID, W: 6, H: 6}}},
		"neg size":   {Version: 1, Regions: []Region{{SID: 0, W: 0, H: 6}}},
		"overlap": {Version: 1, Regions: []Region{
			{SID: 0, MinCX: 0, MinCZ: 0, W: 6, H: 6},
			{SID: 1, MinCX: 3, MinCZ: 3, W: 6, H: 6}, // overlaps the first
		}},
	} {
		if err := bad.Validate(); err == nil {
			t.Errorf("expected %q to be rejected", name)
		}
	}

	// Edge-adjacent (touching, not overlapping) tiles are valid.
	if err := (Map{Version: 1, Regions: []Region{
		{SID: 0, MinCX: 0, MinCZ: 0, W: 6, H: 6},
		{SID: 1, MinCX: 6, MinCZ: 0, W: 6, H: 6}, // touches at x=6, no overlap
	}}).Validate(); err != nil {
		t.Errorf("edge-adjacent tiles wrongly rejected: %v", err)
	}
}

func TestTopoHashOrderIndependent(t *testing.T) {
	a := testbed()
	// Same partition, regions listed in the opposite order, must hash equal.
	b := Map{Version: 1, Regions: []Region{a.Regions[1], a.Regions[0]}}
	if a.TopoHash() != b.TopoHash() {
		t.Fatal("region ordering must not change the topology hash")
	}
	if len(a.TopoHash()) != 64 {
		t.Fatalf("sha256 hex must be 64 chars, got %d", len(a.TopoHash()))
	}
	// A different partition must hash differently.
	c := Map{Version: 1, Regions: []Region{
		{SID: 0, MinCX: -16, MinCZ: -8, W: 16, H: 16},
		{SID: 1, MinCX: 0, MinCZ: -8, W: 16, H: 17}, // taller east tile
	}}
	if a.TopoHash() == c.TopoHash() {
		t.Fatal("different partitions must hash differently")
	}
}

func TestMintEIDMinterRoundTrip(t *testing.T) {
	sids := []int32{0, 1, 2, 5, MaxSIDs - 2, PlayerSID}
	counters := []int64{1, 2, 63, 64, 65, 1000, 1 << 20, 1 << 24}
	for _, sid := range sids {
		for _, c := range counters {
			eid := MintEID(c, sid)
			if eid < 0 {
				t.Fatalf("MintEID(%d,%d)=%d is negative", c, sid, eid)
			}
			if got := Minter(eid); got != sid {
				t.Errorf("Minter(MintEID(%d,%d))=%d want %d", c, sid, got, sid)
			}
		}
	}
}

func TestMintEIDUniqueAndLanes(t *testing.T) {
	seen := map[int32]bool{}
	for c := int64(1); c <= 100000; c++ {
		eid := MintEID(c, 1)
		if seen[eid] {
			t.Fatalf("eid collision within lane 1 at counter %d", c)
		}
		seen[eid] = true
	}
	for c := int64(1); c <= 1000; c++ {
		if MintEID(c, PlayerSID) == MintEID(c, 0) {
			t.Fatalf("player and mob lane collided at counter %d", c)
		}
	}
	big := int64(1)<<24 + 5
	if got := Minter(MintEID(big, PlayerSID)); got != PlayerSID {
		t.Errorf("post-wrap Minter=%d want PlayerSID", got)
	}
}
