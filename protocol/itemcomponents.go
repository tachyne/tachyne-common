package protocol

import "bytes"

// Item components added for the ominous banner, a loaded crossbow and a mob
// bucket, in CANONICAL (770) numbering, and their ids on the later clients.
// Every id is its position in DataComponents' registration order (counted
// per version; the per-version registry reports agree):
//
//	                         770  774  775  776  777
//	item_name                  6    9    9    9    9
//	rarity                     9   12   12   12   12
//	tooltip_display           15   18   18   18   18
//	charged_projectiles       40   47   49   49   51
//	bucket_entity_data        50   57   59   59   61
//	salmon/size               77   84   86   87   93
//	tropical_fish/pattern     79   86   88   89   95
//	tropical_fish/base_color  80   87   89   90   96
//	tropical_fish/pattern_col 81   88   90   91   97
//	axolotl/variant           91   99  104  105  111
const (
	componentItemName         = 6  // item_name: a text component (network NBT)
	componentRarity           = 9  // rarity: the Rarity enum, one varint
	componentTooltipDisplay   = 15 // tooltip_display: hide flag + a set of component ids
	componentChargedProj      = 40 // charged_projectiles: a crossbow's loaded stacks
	componentBucketEntityData = 50 // bucket_entity_data: a compound tag
	componentSalmonSize       = 77 // salmon/size: Salmon.Variant id
	componentFishPattern      = 79 // tropical_fish/pattern: Pattern packed id
	componentFishBaseColor    = 80 // tropical_fish/base_color: DyeColor id
	componentFishPatternColor = 81 // tropical_fish/pattern_color: DyeColor id
	componentAxolotlVariant   = 91 // axolotl/variant: Axolotl.Variant id
)

// laterComponentIDs: each canonical id above at 774, 775, 776 and 777.
var laterComponentIDs = map[int32][4]int32{
	componentItemName:         {9, 9, 9, 9},
	componentRarity:           {12, 12, 12, 12},
	componentTooltipDisplay:   {18, 18, 18, 18},
	componentChargedProj:      {47, 49, 49, 51},
	componentBucketEntityData: {57, 59, 59, 61},
	componentSalmonSize:       {84, 86, 87, 93},
	componentFishPattern:      {86, 88, 89, 95},
	componentFishBaseColor:    {87, 89, 90, 96},
	componentFishPatternColor: {88, 90, 91, 97},
	componentAxolotlVariant:   {99, 104, 105, 111},
}

func laterCompID(canon, version int32) int32 {
	row, ok := laterComponentIDs[canon]
	switch {
	case !ok:
		return -1
	case version >= 777:
		return row[3]
	case version >= 776:
		return row[2]
	case version >= 775:
		return row[1]
	case version >= 774:
		return row[0]
	}
	return canon
}

// knownComponents is every canonical component id the slot copier walks.
var knownComponents = []int32{
	componentMaxDamage, componentDamage, componentEnchantments, componentCustomName,
	componentLore, componentStoredEnch, componentMapID, componentDyedColor, componentTrim,
	componentPotionContents, componentStewEffects, componentRepairCost, componentContainer,
	componentOminousBottle, componentFireworks, componentFireworkStar, componentPotDecorations,
	componentInstrument, componentBannerPatterns, componentWritableBook, componentWrittenBook,
	componentBundleContents, componentLodestone, componentBaseColor,
	componentItemName, componentRarity, componentTooltipDisplay, componentChargedProj,
	componentBucketEntityData, componentSalmonSize, componentFishPattern,
	componentFishBaseColor, componentFishPatternColor, componentAxolotlVariant,
}

// componentIDAt is a known canonical component's id at a client version —
// what a tooltip_display's hidden set names. ok=false for one we don't know.
func componentIDAt(canon, version int32) (int32, bool) {
	switch canon {
	case componentMaxDamage, componentDamage:
		return canon, true
	case componentEnchantments:
		return enchCompID(version), true
	case componentCustomName:
		return customNameCompID(version), true
	case componentLore:
		return loreCompID(version), true
	case componentStoredEnch:
		return storedEnchCompID(version), true
	case componentMapID:
		return mapIDCompID(version), true
	case componentDyedColor:
		return dyedColorCompID(version), true
	case componentTrim:
		return trimCompID(version), true
	case componentPotionContents:
		return potionContentsCompID(version), true
	case componentStewEffects:
		return stewEffectsCompID(version), true
	case componentRepairCost:
		return repairCostCompID(version), true
	case componentContainer:
		return containerCompID(version), true
	case componentOminousBottle:
		return ominousBottleCompID(version), true
	case componentFireworks:
		return fireworksCompID(version), true
	case componentFireworkStar:
		return fireworkStarCompID(version), true
	case componentPotDecorations:
		return potDecorationsCompID(version), true
	case componentInstrument:
		return instrumentCompID(version), true
	case componentBannerPatterns:
		return bannerPatternsCompID(version), true
	case componentWritableBook:
		return writableBookCompID(version), true
	case componentWrittenBook:
		return writtenBookCompID(version), true
	case componentBundleContents:
		return bundleContentsCompID(version), true
	case componentLodestone:
		return lodestoneCompID(version), true
	case componentBaseColor:
		return baseColorCompID(version), true
	}
	if id := laterCompID(canon, version); id >= 0 {
		return id, true
	}
	return 0, false
}

// componentIDFrom is componentIDAt's inverse: a client's component id back
// to canonical.
func componentIDFrom(wire, version int32) (int32, bool) {
	for _, c := range knownComponents {
		if id, _ := componentIDAt(c, version); id == wire {
			return c, true
		}
	}
	return 0, false
}

// laterComponent reports whether cid (canonical clientbound, the client's
// own id serverbound) is one of the components above, and the canonical id
// and the id to write.
func laterComponent(cid, version int32, serverbound bool) (canon, outID int32, ok bool) {
	if !serverbound {
		if _, ok := laterComponentIDs[cid]; !ok {
			return 0, 0, false
		}
		return cid, laterCompID(cid, version), true
	}
	for c := range laterComponentIDs {
		if laterCompID(c, version) == cid {
			return c, c, true
		}
	}
	return 0, 0, false
}

// copyLaterComponent copies the payload of one of the components above.
func copyLaterComponent(r *bytes.Reader, out *[]byte, canon int32, remap func(int32) int32, version int32, serverbound bool, depth int) bool {
	switch canon {
	case componentItemName, componentBucketEntityData:
		// A text component and a compound tag: network NBT, the same on
		// every version.
		return copyNBTValue(r, out)
	case componentRarity, componentSalmonSize, componentFishPattern,
		componentFishBaseColor, componentFishPatternColor, componentAxolotlVariant:
		// One enum id (ByteBufCodecs.idMapper), stable across versions.
		v, err := ReadVarInt(r)
		if err != nil {
			return false
		}
		*out = AppendVarInt(*out, v)
		return true
	case componentTooltipDisplay:
		// TooltipDisplay: hideTooltip, then the hidden components — a set
		// of component-type ids, which renumber like the components do.
		hide, err := r.ReadByte()
		if err != nil || hide > 1 {
			return false
		}
		n, err := ReadVarInt(r)
		if err != nil || n < 0 || n > 64 {
			return false
		}
		*out = append(*out, hide)
		*out = AppendVarInt(*out, n)
		for i := int32(0); i < n; i++ {
			id, err := ReadVarInt(r)
			if err != nil {
				return false
			}
			var ok bool
			if serverbound {
				id, ok = componentIDFrom(id, version)
			} else {
				id, ok = componentIDAt(id, version)
			}
			if !ok {
				return false
			}
			*out = AppendVarInt(*out, id)
		}
		return true
	case componentChargedProj:
		// charged_projectiles: the stacks a crossbow holds loaded. They
		// recurse like a bundle's, so the ids inside are remapped too.
		if depth >= maxBundleNesting {
			return false
		}
		n, err := ReadVarInt(r)
		if err != nil || n < 0 || n > 64 {
			return false
		}
		return copyNestedStacks(r, out, n, remap, version, serverbound, depth+1, false)
	}
	return false
}

// templateStacks reports whether a version nests stacks inside components
// (bundle_contents, container, charged_projectiles) as ItemStackTemplates —
// item, count, components — rather than as Slots (count, item, components).
// 26.1 made the change; a template cannot be empty, so a container marks
// each of its positions present or absent with a flag instead.
func templateStacks(version int32) bool { return version >= 775 }

// copyNestedStacks copies the n stacks a component carries. Canonically
// they are Slots (a container's empties as count 0); on a 26.x client they
// are templates, a container's wrapped in an Optional. A stack the client
// lacks is left out of a list, or goes out as an empty position in a
// container, whose list is positional.
func copyNestedStacks(r *bytes.Reader, out *[]byte, n int32, remap func(int32) int32, version int32, serverbound bool, depth int, positional bool) bool {
	tmpl := templateStacks(version)
	var items []byte
	kept := int32(0)
	for j := int32(0); j < n; j++ {
		// Read one stack in the incoming form.
		var count, item int32
		var err error
		if serverbound && tmpl {
			present := true
			if positional {
				flag, err := r.ReadByte()
				if err != nil || flag > 1 {
					return false
				}
				present = flag == 1
			}
			if present {
				if item, err = ReadVarInt(r); err != nil {
					return false
				}
				if count, err = ReadVarInt(r); err != nil || count <= 0 {
					return false
				}
			}
		} else {
			if count, err = ReadVarInt(r); err != nil {
				return false
			}
			if count > 0 {
				if item, err = ReadVarInt(r); err != nil {
					return false
				}
			}
		}
		empty := count <= 0
		var patch []byte
		if !empty {
			if !copyComponentPatch(r, &patch, remap, version, serverbound, depth) {
				return false
			}
			item = remap(item)
			if !serverbound && item == 0 {
				empty = true // a stack the client lacks
			}
		}
		// Write it in the outgoing form.
		switch {
		case empty && !positional:
			continue
		case !serverbound && tmpl:
			if positional {
				if empty {
					items = append(items, 0)
					kept++
					continue
				}
				items = append(items, 1)
			}
			items = AppendVarInt(items, item)
			items = AppendVarInt(items, count)
		case empty:
			items = AppendVarInt(items, 0)
			kept++
			continue
		default:
			items = AppendVarInt(items, count)
			items = AppendVarInt(items, item)
		}
		items = append(items, patch...)
		kept++
	}
	*out = AppendVarInt(*out, kept)
	*out = append(*out, items...)
	return true
}
