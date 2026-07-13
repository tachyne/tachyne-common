package protocol

// loom.go — the loom's pattern data, derived from the SAME generated
// sources the clients receive (the banner_pattern registry declaration and
// its tags), so the engine's row indices and every client's list agree by
// construction: the client builds its selectable list from the
// no_item_required tag (pattern slot empty) or the pattern item's tag, in
// tag-entry order, resolved against the synced registry.

import "sync"

var (
	loomOnce   sync.Once
	loomBase   []int32
	loomByItem map[string][]int32
)

// LoomPatterns returns the loom's selectable banner-pattern ids: base is the
// no_item_required list (client order), byTag maps a pattern-item tag suffix
// ("flower", "field_masoned", …) to its pattern ids. Ids index the declared
// banner_pattern registry order — identical on every served version.
func LoomPatterns() (base []int32, byTag map[string][]int32) {
	loomOnce.Do(func() {
		idx := dynamic26xIndex()["minecraft:banner_pattern"]
		loomByItem = map[string][]int32{}
		for _, reg := range tags1215Data {
			if reg.registry != "minecraft:banner_pattern" {
				continue
			}
			for _, t := range reg.tags {
				var ids []int32
				for _, e := range t.entries {
					if id, ok := idx[e]; ok {
						ids = append(ids, id)
					}
				}
				switch {
				case t.name == "minecraft:no_item_required":
					loomBase = ids
				case len(ids) > 0:
					suffix := t.name[len("minecraft:pattern_item/"):]
					loomByItem[suffix] = ids
				}
			}
		}
	})
	return loomBase, loomByItem
}

// BannerPatternName reverse-resolves a pattern id to its registry name
// (block-entity NBT carries names, not ids).
func BannerPatternName(id int32) string {
	for _, reg := range SyncedRegistries {
		if reg.ID == "minecraft:banner_pattern" {
			if int(id) >= 0 && int(id) < len(reg.Entries) {
				return reg.Entries[id]
			}
		}
	}
	return ""
}
