// Package shard holds the identities and region map shared by world pods and
// gateways in a multi-pod (sharded) world. Everything here must be computed
// IDENTICALLY on every component, so it lives in one place with no dependency
// beyond the standard library.
//
// Ownership is an EXPLICIT list of rectangular regions (not a modular formula):
// each pod owns a contiguous rectangle of chunks, chunks in no region are the
// edge of the world (Unowned), and adding a pod appends a region without
// touching any existing one — so growing the world never reshuffles ownership.
//
// See tachyne-world/docs/SHARDING-BUILD.md for how it is used.
package shard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

const (
	// MaxSIDs is the modulus of the entity-ID interleave (see MintEID) — the
	// number of eid "lanes". Never change it on a live world: every persisted
	// eid-derived fact would re-route.
	MaxSIDs = 64

	// PlayerSID is the reserved minting lane for PLAYER entity IDs. Player eids
	// are session-stable across handovers, so their lane must not collide with
	// any real shard's mob lane. SIDs therefore stay <= MaxSIDs-2 (lane 63 =
	// players, lane 62 spare) — see Map.Validate.
	PlayerSID = MaxSIDs - 1
)

// Unowned is returned by ShardOf for a chunk that lies in no region — the edge
// of the world (void). Adding a region makes such chunks exist.
const Unowned = int32(-1)

// MintEID interleaves a per-minter counter with the minting shard's SID so that
// (a) two pods never hand out the same eid and (b) the minting lane is
// recoverable from the eid alone (see Minter).
//
// counter must be a monotonically increasing int64, never reused within a pod
// boot. The result is always a non-negative int32. The counter portion wraps at
// ~33.5M mints per boot (2^24); the low 6 bits (the SID lane) survive the wrap.
// This package stays dependency-free and does not log; callers expecting
// long-lived pods should watch for counter >= 1<<24 and warn.
func MintEID(counter int64, sid int32) int32 {
	return int32((counter*MaxSIDs + int64(sid)) & 0x7fffffff)
}

// Minter recovers the SID lane an eid was minted in. For players (lane ==
// PlayerSID) this is NOT ownership — a player's home pod is resolved out of band
// (the handover snapshot / gateway routing), never from the eid.
func Minter(eid int32) int32 { return eid % MaxSIDs }

// Region is one pod's owned rectangle, in CHUNK coordinates, half-open: it
// covers chunks with cx in [MinCX, MinCX+W) and cz in [MinCZ, MinCZ+H). A pod
// may own more than one region (e.g. after a future hot-split), so ownership is
// keyed by SID, not by position.
type Region struct {
	SID   int32 `json:"sid"`
	MinCX int32 `json:"min_cx"`
	MinCZ int32 `json:"min_cz"`
	W     int32 `json:"w"` // width in chunks, > 0
	H     int32 `json:"h"` // height in chunks, > 0
}

func (r Region) contains(cx, cz int32) bool {
	return cx >= r.MinCX && cx < r.MinCX+r.W && cz >= r.MinCZ && cz < r.MinCZ+r.H
}

// Map is the world's ownership partition: an explicit list of regions. Chunks in
// no region are Unowned. Regions currently apply to every dimension (per-dim
// ownership is a future extension); ShardOf takes dim for API stability.
type Map struct {
	Version int32    `json:"version"`
	Regions []Region `json:"regions"`
}

// ShardOf returns the SID owning a chunk, or Unowned if no region covers it.
// Linear scan over regions — fine for the small region counts we run; a spatial
// index is a later optimisation if N grows large.
func (m Map) ShardOf(dim, cx, cz int32) int32 {
	for _, r := range m.Regions {
		if r.contains(cx, cz) {
			return r.SID
		}
	}
	return Unowned
}

// Owns reports whether sid owns the given chunk.
func (m Map) Owns(sid, dim, cx, cz int32) bool { return m.ShardOf(dim, cx, cz) == sid }

// Neighbours returns the distinct SIDs whose regions are edge-adjacent to any
// region owned by sid — the pods this one shares a border with, and therefore
// the pods it must keep warm peer links to. Derived from rectangle geometry.
// Regions that only touch at a corner are NOT neighbours (a zero-length edge
// cannot be crossed).
func (m Map) Neighbours(sid int32) []int32 {
	set := map[int32]bool{}
	for _, r := range m.Regions {
		if r.SID != sid {
			continue
		}
		for _, q := range m.Regions {
			if q.SID != sid && adjacent(r, q) {
				set[q.SID] = true
			}
		}
	}
	out := make([]int32, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// NeighboursWithin returns the distinct SIDs of regions lying within r chunks
// (Chebyshev/box distance; overlap or edge/corner touch counts as within 0) of
// any region owned by sid — the AWARENESS set: regions a player in sid's
// territory could see into with a view radius of r, and therefore the peer links
// this pod must keep warm for cross-border visibility + shadow-push. Excludes
// sid.
//
// This is a SUPERSET of Neighbours: unlike edge-adjacency it includes DIAGONAL
// (corner-touching) regions, because a square view box near a corner where four
// tiles meet overlaps all three others. Use r = the view radius; pass 0 for the
// pure touching (edge+corner) set.
func (m Map) NeighboursWithin(sid, r int32) []int32 {
	set := map[int32]bool{}
	for _, a := range m.Regions {
		if a.SID != sid {
			continue
		}
		for _, b := range m.Regions {
			if b.SID != sid && withinChunks(a, b, r) {
				set[b.SID] = true
			}
		}
	}
	out := make([]int32, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// TopoHash is the hex sha256 of the canonical (region-sorted) JSON encoding, so
// two components with the same partition agree regardless of the order regions
// were listed in the ConfigMap. A mixed-topology cluster must be caught at
// session start via this hash.
func (m Map) TopoHash() string {
	regions := append([]Region(nil), m.Regions...)
	sort.Slice(regions, func(i, j int) bool { return lessRegion(regions[i], regions[j]) })
	b, err := json.Marshal(Map{Version: m.Version, Regions: regions})
	if err != nil {
		return "invalid" // three ints and a slice of ints — Marshal cannot fail
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Validate reports why a map is unusable, or nil if it is fine. Pods must refuse
// to start on a validation error — a bad partition corrupts the world.
func (m Map) Validate() error {
	if m.Version < 1 {
		return fmt.Errorf("shard: version must be >= 1, got %d", m.Version)
	}
	if len(m.Regions) == 0 {
		return fmt.Errorf("shard: map has no regions")
	}
	for i, r := range m.Regions {
		if r.SID < 0 || r.SID > MaxSIDs-2 {
			return fmt.Errorf("shard: region %d has sid %d, must be 0..%d", i, r.SID, MaxSIDs-2)
		}
		if r.W < 1 || r.H < 1 {
			return fmt.Errorf("shard: region %d (sid %d) has non-positive size %dx%d", i, r.SID, r.W, r.H)
		}
	}
	// No two regions may overlap — a chunk owned by more than one pod would have
	// two writers, which the whole design forbids.
	for i := 0; i < len(m.Regions); i++ {
		for j := i + 1; j < len(m.Regions); j++ {
			if overlaps(m.Regions[i], m.Regions[j]) {
				return fmt.Errorf("shard: regions %d and %d overlap", i, j)
			}
		}
	}
	return nil
}

// adjacent reports whether two rectangles share a positive-length edge (touch
// along a vertical or horizontal border with overlapping extent on the other
// axis). Corner-only contact does not count.
func adjacent(a, b Region) bool {
	ax0, ax1, az0, az1 := a.MinCX, a.MinCX+a.W, a.MinCZ, a.MinCZ+a.H
	bx0, bx1, bz0, bz1 := b.MinCX, b.MinCX+b.W, b.MinCZ, b.MinCZ+b.H
	if (ax1 == bx0 || bx1 == ax0) && span(az0, az1, bz0, bz1) { // vertical seam
		return true
	}
	if (az1 == bz0 || bz1 == az0) && span(ax0, ax1, bx0, bx1) { // horizontal seam
		return true
	}
	return false
}

// overlaps reports whether two rectangles share interior area (both axes overlap
// with positive length).
func overlaps(a, b Region) bool {
	return span(a.MinCX, a.MinCX+a.W, b.MinCX, b.MinCX+b.W) &&
		span(a.MinCZ, a.MinCZ+a.H, b.MinCZ, b.MinCZ+b.H)
}

// withinChunks reports whether rectangles a and b are within r chunks of each
// other on both axes (Chebyshev). Overlapping or touching rectangles have gap 0.
func withinChunks(a, b Region, r int32) bool {
	return axisGap(a.MinCX, a.MinCX+a.W, b.MinCX, b.MinCX+b.W) <= r &&
		axisGap(a.MinCZ, a.MinCZ+a.H, b.MinCZ, b.MinCZ+b.H) <= r
}

// axisGap returns the non-negative gap between two half-open intervals, or 0 if
// they overlap or touch.
func axisGap(lo1, hi1, lo2, hi2 int32) int32 {
	if hi1 <= lo2 {
		return lo2 - hi1
	}
	if hi2 <= lo1 {
		return lo1 - hi2
	}
	return 0
}

// span reports whether two half-open intervals overlap with positive length.
func span(lo1, hi1, lo2, hi2 int32) bool { return max(lo1, lo2) < min(hi1, hi2) }

func lessRegion(a, b Region) bool {
	switch {
	case a.SID != b.SID:
		return a.SID < b.SID
	case a.MinCX != b.MinCX:
		return a.MinCX < b.MinCX
	case a.MinCZ != b.MinCZ:
		return a.MinCZ < b.MinCZ
	case a.W != b.W:
		return a.W < b.W
	default:
		return a.H < b.H
	}
}
