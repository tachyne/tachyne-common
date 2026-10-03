package render770

// stonecutter.go — update_recipes (canonical 0x7e). Since 1.21.5 this packet
// carries only the station item-property sets and the STONECUTTER recipe
// list (Ingredient + result SlotDisplay per row); the client filters it by
// the menu's input, preserving order, and clicks send container_button_click
// with the row index. The list is composed from the shared generated table
// (protocol.StonecuttingRecipes) — the same slice the engine's menu uses, so
// the indices agree. Item ids are raw (no body rewriter): remapped here at
// the source, like recipe_book_add. The world may send its own data
// (attach.UpdateRecipes, a data pack's recipes); the gateway renders that
// the same way.

import (
	"slices"
	"strings"

	"github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// IDUpdateRecipes is the canonical-770 clientbound update_recipes id.
const IDUpdateRecipes = 0x7e

// UpdateRecipes composes the packet from the generated vanilla tables.
func UpdateRecipes(version int32) Packet { return UpdateRecipesFrom(version, nil) }

// VanillaRecipes is the generated tables in the frame's form: the three
// SMITHING property sets (the smithing menu's client-side slot predicates
// come from them, so empty sets would refuse every placement; the cooker
// menus use plain slots and need none) and the stonecutter rows. The map
// tables are listed in id order.
func VanillaRecipes() attach.UpdateRecipes {
	return attach.UpdateRecipes{ItemSets: vanillaItemSets(), Stonecutter: vanillaStonecutter()}
}

func vanillaItemSets() []attach.RecipePropertySet {
	templates := append([]int32{protocol.SmithingUpgradeTemplate}, sortedKeys(protocol.SmithingTrimTemplate)...)
	bases := append(append([]int32(nil), protocol.SmithingTrimmable...), sortedKeys(protocol.SmithingTransform)...)
	return []attach.RecipePropertySet{
		{Key: "minecraft:smithing_template", Items: templates},
		{Key: "minecraft:smithing_base", Items: bases},
		{Key: "minecraft:smithing_addition", Items: sortedKeys(protocol.SmithingTrimMaterial)},
	}
}

func vanillaStonecutter() []attach.StonecutterRecipe {
	rows := make([]attach.StonecutterRecipe, len(protocol.StonecuttingRecipes))
	for i, r := range protocol.StonecuttingRecipes {
		rows[i] = attach.StonecutterRecipe{
			Input:  attach.RecipeIngredient{Items: []int32{r.In}},
			Result: r.Out,
			Count:  int32(r.Count),
		}
	}
	return rows
}

func sortedKeys(m map[int32]int32) []int32 {
	ks := make([]int32, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	slices.Sort(ks)
	return ks
}

// barrierItem stands in for an ingredient whose every item the client
// lacks: the row must stay (the button indices line up with the engine's
// list), and an empty or air ingredient drops the connection.
var barrierItem = protocol.CanonicalItem("barrier")

// UpdateRecipesFrom composes ClientboundUpdateRecipesPacket from the world's
// recipe data; a nil u, or a nil half of it, is the generated vanilla table.
func UpdateRecipesFrom(version int32, u *attach.UpdateRecipes) Packet {
	var sets []attach.RecipePropertySet
	var rows []attach.StonecutterRecipe
	if u != nil {
		sets, rows = u.ItemSets, u.Stonecutter
	}
	if sets == nil {
		sets = vanillaItemSets()
	}
	if rows == nil {
		rows = vanillaStonecutter()
	}
	rid := func(id int32) int32 { return protocol.RemapID(protocol.RegItem, version, id) }
	sd := slotDisplayIDs{item: 2, itemStack: 3}
	if version >= 775 {
		sd = slotDisplayIDs{item: 4, itemStack: 5, templateForm: true}
	}
	// item_sets: a map of ResourceKey → RecipePropertySet (a list of Item
	// holders, registry ids).
	b := protocol.AppendVarInt(nil, int32(len(sets)))
	for _, s := range sets {
		b = protocol.AppendString(b, s.Key)
		b = protocol.AppendVarInt(b, int32(len(s.Items)))
		for _, it := range s.Items {
			b = protocol.AppendVarInt(b, rid(it))
		}
	}
	b = protocol.AppendVarInt(b, int32(len(rows)))
	for _, r := range rows {
		b = appendIngredient(b, r.Input, rid)
		// An output the client lacks shows empty, not as its stand-in: the
		// row would claim to cut white concrete into a quartz slab.
		out := rid(r.Result)
		if !protocol.IDPresent(protocol.RegItem, version, r.Result) {
			out = 0
		}
		b = appendSlotDisplay(b, sd, out, int(r.Count))
	}
	return Packet{IDUpdateRecipes, b}
}

// appendIngredient writes Ingredient.CONTENTS_STREAM_CODEC, a holder set:
// 0 then the tag's identifier, or the item count + 1 then each id. Items the
// client lacks without a stand-in are left out (air is refused).
func appendIngredient(b []byte, in attach.RecipeIngredient, rid func(int32) int32) []byte {
	if in.Tag != "" {
		b = protocol.AppendVarInt(b, 0)
		return protocol.AppendString(b, strings.TrimPrefix(in.Tag, "#"))
	}
	ids := make([]int32, 0, len(in.Items))
	for _, it := range in.Items {
		if c := rid(it); c != 0 {
			ids = append(ids, c)
		}
	}
	if len(ids) == 0 {
		ids = append(ids, rid(barrierItem))
	}
	b = protocol.AppendVarInt(b, int32(len(ids)+1))
	for _, id := range ids {
		b = protocol.AppendVarInt(b, id)
	}
	return b
}
