package protocol

import "sort"

// Unified multi-version registry translation (ViaVersion-style MappingData). The
// canonical 770 ID is the internal identifier; per registry and client version we
// translate IDs in either direction. translationTables (generated) holds the
// forward (canonical → client) shift ranges; reverse tables for serverbound
// translation are derived from them at init.
//
// One API serves every registry — blocks, entities, items, biomes — so adding a
// registry is just data, not new code.

// IDSpace selects which ID space to translate. Values are defined in
// translation_gen.go (RegBlockState, RegEntity, RegItem, RegBiome).
type IDSpace int

// idRange shifts a contiguous run of canonical IDs by a constant delta. Field shape
// MUST match the literals emitted by gen_translation.py.
type idRange struct {
	Min, Max uint32
	Delta    int32
}

// reverseTables[registry][version] inverts translationTables for serverbound
// (client → canonical) translation. Built once at init.
var reverseTables = map[IDSpace]map[int32][]idRange{}

func init() {
	for reg, byVer := range translationTables {
		rev := make(map[int32][]idRange, len(byVer))
		for ver, ranges := range byVer {
			inv := make([]idRange, len(ranges))
			for i, r := range ranges {
				// The forward map is monotonic (registry insertions preserve order),
				// so inverting each range and re-sorting yields disjoint ranges.
				inv[i] = idRange{
					Min:   uint32(int32(r.Min) + r.Delta),
					Max:   uint32(int32(r.Max) + r.Delta),
					Delta: -r.Delta,
				}
			}
			sort.Slice(inv, func(a, b int) bool { return inv[a].Min < inv[b].Min })
			rev[ver] = inv
		}
		reverseTables[reg] = rev
	}
}

// shift binary-searches sorted ranges and applies the delta, or returns id unchanged.
func shift(ranges []idRange, id int32) int32 {
	u := uint32(id)
	lo, hi := 0, len(ranges)-1
	for lo <= hi {
		mid := (lo + hi) / 2
		r := ranges[mid]
		switch {
		case u < r.Min:
			hi = mid - 1
		case u > r.Max:
			lo = mid + 1
		default:
			return id + r.Delta
		}
	}
	return id
}

// absentFallback is what a canonical entry becomes when the client version has
// no such entry. Substituting is not a nicety: canonical IDs run HIGHER than an
// older client's registry (items reach 1504 canonically against 1396 entries on
// 770), so an unshifted absent ID can land past the end of the client's
// registry entirely — which is a decode failure and a dropped connection, not
// merely the wrong icon.
//
// Air is the honest stand-in for an item or a block: the client shows nothing
// rather than something it was never meant to see. Entities have a richer
// substitution table of their own (entity_substitute.go) and are left to it.
var absentFallback = map[IDSpace]int32{
	RegItem:       0, // air
	RegBlock:      0, // air
	RegBlockState: 0, // air
}

// RemapID translates a canonical 770 ID to the client version's ID (clientbound).
//
// An ID with no counterpart on that version resolves to the registry's
// fallback rather than passing through unshifted, because passing it through
// means sending an ID that either denotes something else or does not exist at
// all. Callers wanting to handle the gap themselves — dropping the entry, or
// substituting something cleverer — should test IDPresent first.
func RemapID(reg IDSpace, version, id int32) int32 {
	if !IDPresent(reg, version, id) {
		if sub, ok := absentFallback[reg]; ok {
			return sub
		}
	}
	return shift(translationTables[reg][version], id)
}

// UnmapID translates a client version's ID back to canonical 770 (serverbound).
func UnmapID(reg IDSpace, version, id int32) int32 {
	return shift(reverseTables[reg][version], id)
}

// HasRemap reports whether a registry needs translation for a client version.
func HasRemap(reg IDSpace, version int32) bool {
	return len(translationTables[reg][version]) > 0
}

// IDPresent reports whether a canonical ID exists at all on a client version.
//
// The shift tables can only move an ID, never say "this does not exist here".
// A canonical entry with no counterpart gets no delta, so RemapID hands back
// the unshifted ID — which on that client is a DIFFERENT registry entry. There
// is no decode error to catch it; the client simply renders the wrong thing.
// Senders must consult this and DROP such entries.
func IDPresent(reg IDSpace, version, id int32) bool {
	gone, ok := absentIDs[reg][version]
	if !ok {
		return true
	}
	lo, hi := 0, len(gone)-1
	for lo <= hi {
		mid := (lo + hi) / 2
		switch {
		case gone[mid] < id:
			lo = mid + 1
		case gone[mid] > id:
			hi = mid - 1
		default:
			return false
		}
	}
	return true
}
