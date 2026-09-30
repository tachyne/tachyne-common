package protocol

import "bytes"

// 26.3 made the explorer maps items of their own (buried_treasure_map,
// ocean_monument_map, …) and added map decorations for the new ones. A 26.2
// client has neither: the item shows as a filled map (absentItemStandIns)
// and the decoration as the nearest 26.2 icon.

// explorerMapNames: for each explorer map item, the name a 26.2 client
// shows it by — the key vanilla 26.2 named that map with where it had one
// (filled_map.*), else 26.3's own key with its English name as the
// fallback text.
var explorerMapNames = map[string][2]string{
	"buried_treasure_map":       {"filled_map.buried_treasure", ""},
	"jungle_pyramid_map":        {"filled_map.explorer_jungle", ""},
	"swamp_hut_map":             {"filled_map.explorer_swamp", ""},
	"woodland_mansion_map":      {"filled_map.mansion", ""},
	"ocean_monument_map":        {"filled_map.monument", ""},
	"buried_trial_chambers_map": {"filled_map.trial_chambers", ""},
	"desert_village_map":        {"filled_map.village_desert", ""},
	"plains_village_map":        {"filled_map.village_plains", ""},
	"savanna_village_map":       {"filled_map.village_savanna", ""},
	"snowy_village_map":         {"filled_map.village_snowy", ""},
	"taiga_village_map":         {"filled_map.village_taiga", ""},
	"abandoned_camp_map":        {"item.minecraft.abandoned_camp_map", "Abandoned Camp Map"},
	"buried_ancient_city_map":   {"item.minecraft.buried_ancient_city_map", "Buried Ancient City Map"},
	"buried_mineshaft_map":      {"item.minecraft.buried_mineshaft_map", "Buried Mineshaft Map"},
	"desert_pyramid_map":        {"item.minecraft.desert_pyramid_map", "Desert Pyramid Map"},
	"warm_ocean_ruins_map":      {"item.minecraft.warm_ocean_ruins_map", "Warm Ocean Ruins Map"},
}

// explorerMapIDs is explorerMapNames by canonical item id.
var explorerMapIDs = func() map[int32][2]string {
	m := map[int32][2]string{}
	for name, key := range explorerMapNames {
		if id, ok := canonicalItemIDs[name]; ok {
			m[id] = key
		}
	}
	return m
}()

// explorerMapItemName is the item_name component entry (the version's
// component id, then the text component as network NBT) for a canonical
// explorer map a client version lacks; nil for anything else.
func explorerMapItemName(version, item int32) []byte {
	key, ok := explorerMapIDs[item]
	if !ok || IDPresent(RegItem, version, item) {
		return nil
	}
	id, ok := componentIDAt(componentItemName, version)
	if !ok {
		return nil
	}
	b := AppendVarInt(nil, id)
	b = append(b, NBTRoot()...)
	b = NBTString(b, "translate", key[0])
	if key[1] != "" {
		b = NBTString(b, "fallback", key[1])
	}
	return NBTEnd(b)
}

// prependComponent adds one component entry at the front of a copied patch
// (add count, remove count, entries, removed ids). A component the patch
// already sets comes after it and wins, as the client's decode keeps the
// last value it reads for a type.
func prependComponent(patch, entry []byte) ([]byte, bool) {
	r := bytes.NewReader(patch)
	addC, e1 := ReadVarInt(r)
	remC, e2 := ReadVarInt(r)
	if e1 != nil || e2 != nil {
		return nil, false
	}
	out := AppendVarInt(nil, addC+1)
	out = AppendVarInt(out, remC)
	out = append(out, entry...)
	return append(out, patch[len(patch)-r.Len():]...), true
}

// Map decorations 26.3 added (MapDecorationTypes): the explorer maps' new
// targets. A 26.2 client, whose registry ends at trial_chambers (34), is
// shown the nearest icon it has: the desert pyramid a temple (jungle_temple),
// the rest the treasure map's red X.
const (
	mapDecorRedX         = 26
	mapDecorJungleTemple = 32
	mapDecorFirst263     = 35 // abandoned_camp
	mapDecorDesertPyr    = 37 // desert_pyramid
	mapDecorLast263      = 39 // ocean_ruin_warm
)

// MapDecorationFor is a map decoration type's id on a client version.
func MapDecorationFor(version, typ int32) int32 {
	if version >= 777 || typ < mapDecorFirst263 || typ > mapDecorLast263 {
		return typ
	}
	if typ == mapDecorDesertPyr {
		return mapDecorJungleTemple
	}
	return mapDecorRedX
}
