package render770

// recipebook.go renders the crafting recipe book (recipe_book_add, 0x43) from
// the domain RecipeBook event. Ported from the engine monolith's recipebook.go (pre-rename).
//
// The green book actually lists what can be made instead of sitting empty.
// Clicking a book entry sends craft_recipe_request (SIDCraftRequest 0x25) with
// the display id; the world fills the crafting grid from the player's
// inventory. Display ids are indices: shaped first, then shapeless.
//
// Item ids in this packet are RAW and there is no body rewriter for it in the
// translation chain, so ids are remapped into the client's version here at the
// source, and the version-specific SlotDisplay layout is applied. Build it at
// the client's REAL protocol version, not the gateway's canonical one.

import (
	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// Canonical-770 clientbound id for recipe_book_add.
const IDRecipeBook = 0x43

const (
	itemCraftingTable  = 320 // shown as the crafting station in book entries
	recipeCategoryMisc = 3   // crafting_misc book tab (cosmetic placement)

	// SlotDisplay any_fuel is registry index 1 in every served version — the
	// fuel slot of a furnace display is this, never a concrete item.
	slotDisplayAnyFuel = 1

	// RecipeDisplay type ids (recipe_display registry order, unchanged
	// through 26.2): crafting_shapeless 0, crafting_shaped 1, furnace 2,
	// stonecutter 3, smithing 4.
	recipeDisplayShapeless = 0
	recipeDisplayShaped    = 1
	recipeDisplayFurnace   = 2
)

// RecipeBook renders recipe_book_add for the given client protocol version.
// The result replaces the client's whole book.
func RecipeBook(rb attach.RecipeBook, version int32) Packet {
	rid := func(id int32) int32 { return protocol.RemapID(protocol.RegItem, version, id) }
	// SlotDisplay type ids: 26.1 (protocol 775) inserted with_any_potion(2) and
	// only_with_component(3) before item, shifting item 2→4 and item_stack 3→5
	// (source: ViaVersion mappings slot_displays; sending 1.21.5 ids to a 26.x
	// client makes it decode the wrong structure → a recipe_book_add decode
	// error at join). RecipeDisplay's own enum (shapeless=0, shaped=1) is
	// unchanged through 26.2. item_stack's payload also changed in 26.1: from a
	// full Slot (count, id, components) to an ItemStackTemplate (id, count,
	// components) — id and count swapped.
	sd := slotDisplayIDs{item: 2, itemStack: 3}
	if version >= 775 {
		sd = slotDisplayIDs{item: 4, itemStack: 5, templateForm: true}
	}

	n := len(rb.Shaped) + len(rb.Shapeless) + len(rb.Cooking)
	b := protocol.AppendVarInt(nil, int32(n))
	for i := range rb.Shaped {
		r := &rb.Shaped[i]
		b = protocol.AppendVarInt(b, r.ID) // displayId (engine-assigned, stable)
		b = protocol.AppendVarInt(b, recipeDisplayShaped)
		b = protocol.AppendVarInt(b, r.W)
		b = protocol.AppendVarInt(b, r.H)
		b = protocol.AppendVarInt(b, int32(len(r.Cells)))
		for _, c := range r.Cells {
			b = appendSlotDisplay(b, sd, rid(c), 1)
		}
		b = appendSlotDisplay(b, sd, rid(r.Result), int(r.Count)) // result (with count)
		b = appendSlotDisplay(b, sd, rid(itemCraftingTable), 1)   // crafting station
		b = appendBookEntryTail(b, ingredientInstances(r.Cells), rid, r.Result, recipeCategoryMisc, r.Notify, r.Highlight)
	}
	for i := range rb.Shapeless {
		r := &rb.Shapeless[i]
		b = protocol.AppendVarInt(b, r.ID) // displayId
		b = protocol.AppendVarInt(b, recipeDisplayShapeless)
		b = protocol.AppendVarInt(b, int32(len(r.Ingredients)))
		for _, c := range r.Ingredients {
			b = appendSlotDisplay(b, sd, rid(c), 1)
		}
		b = appendSlotDisplay(b, sd, rid(r.Result), int(r.Count))
		b = appendSlotDisplay(b, sd, rid(itemCraftingTable), 1)
		b = appendBookEntryTail(b, ingredientInstances(r.Ingredients), rid, r.Result, recipeCategoryMisc, r.Notify, r.Highlight)
	}
	// Cooking entries (vanilla FurnaceRecipeDisplay): one ingredient, the
	// implicit any-fuel slot, the result, the cooker's item as the station,
	// then the cook duration and the experience the recipe banks. These are
	// what fills the furnace/blast-furnace/smoker books — without them those
	// three tabs of the green book sit empty on every client.
	for i := range rb.Cooking {
		r := &rb.Cooking[i]
		b = protocol.AppendVarInt(b, r.ID)
		b = protocol.AppendVarInt(b, recipeDisplayFurnace)
		b = appendSlotDisplay(b, sd, rid(r.Ingredient), 1)
		b = protocol.AppendVarInt(b, slotDisplayAnyFuel)
		b = appendSlotDisplay(b, sd, rid(r.Result), int(r.Count))
		b = appendSlotDisplay(b, sd, rid(r.Station), 1)
		b = protocol.AppendVarInt(b, r.Cook)
		b = protocol.AppendF32(b, r.XP)
		b = appendBookEntryTail(b, []int32{r.Ingredient}, rid, r.Result, r.Category, r.Notify, r.Highlight)
	}
	b = protocol.AppendBool(b, rb.Replace)
	return Packet{IDRecipeBook, b}
}

// IDRecipeBookSettings is the canonical-770 clientbound recipe_book_settings
// id. Wire = the four book types' (open, filtering) bool pairs in enum order
// (crafting, furnace, blast furnace, smoker) — the vanilla RecipeBookSettings
// codec, identical on every served version.
const IDRecipeBookSettings = 0x45

func RecipeBookSettings(e attach.RecipeSettings) Packet {
	var b []byte
	for i := 0; i < 4; i++ {
		b = protocol.AppendBool(b, e.Open[i])
		b = protocol.AppendBool(b, e.Filter[i])
	}
	return Packet{IDRecipeBookSettings, b}
}

// slotDisplayIDs carries the version-dependent SlotDisplay encoding: the type
// ids (empty is 0 in every version; item/item_stack shifted in 26.1) and whether
// item_stack uses the 26.1+ ItemStackTemplate form (id before count).
type slotDisplayIDs struct {
	item, itemStack int32
	templateForm    bool
}

// appendSlotDisplay encodes one SlotDisplay: empty for no item, item for a plain
// ingredient, item_stack when a count must show.
func appendSlotDisplay(b []byte, sd slotDisplayIDs, item int32, count int) []byte {
	switch {
	case item == 0 || count <= 0:
		return protocol.AppendVarInt(b, 0) // empty
	case count == 1:
		b = protocol.AppendVarInt(b, sd.item)
		return protocol.AppendVarInt(b, item)
	default:
		b = protocol.AppendVarInt(b, sd.itemStack)
		if sd.templateForm { // 26.1+: id, count, components(0 add, 0 remove)
			b = protocol.AppendVarInt(b, item)
			b = protocol.AppendVarInt(b, int32(count))
			b = protocol.AppendVarInt(b, 0)
			return protocol.AppendVarInt(b, 0)
		}
		// ≤1.21.11: a full Slot (count first, then id + empty components).
		b = protocol.AppendVarInt(b, int32(count))
		b = protocol.AppendVarInt(b, item)
		b = protocol.AppendVarInt(b, 0)
		return protocol.AppendVarInt(b, 0)
	}
}

// appendBookEntryTail writes the entry fields after the display: the group,
// the book category, the ingredient id-sets (lets the book's "craftable" filter
// work), and empty flags. rid translates item ids to the client's id space.
// Entries sharing a group collapse into one book tile (the client cycles the
// variants), so grouping by result folds the per-wood-type recipe variants
// (stick from oak/spruce/… planks) into a single stick entry. The group id is
// opaque to the client; the canonical result item id is a stable, unique
// choice. optvarint: 0 = none, else id+1.
func appendBookEntryTail(b []byte, ingredients []int32, rid func(int32) int32, group, category int32, notify, highlight bool) []byte {
	b = protocol.AppendVarInt(b, group+1)  // group (optvarint, by result)
	b = protocol.AppendVarInt(b, category) // book tab
	b = protocol.AppendBool(b, true)       // craftingRequirements present
	b = protocol.AppendVarInt(b, int32(len(ingredients)))
	for _, id := range ingredients {
		// IDSet holding one direct id: varint(count+1) then the ids.
		b = protocol.AppendVarInt(b, 2)
		b = protocol.AppendVarInt(b, rid(id))
	}
	var flags byte // vanilla entry flags: 1 = notification toast, 2 = highlight
	if notify {
		flags |= 1
	}
	if highlight {
		flags |= 2
	}
	return append(b, flags)
}

// ingredientInstances lists every non-empty cell/ingredient, duplicates
// included — the client's "craftable" filter counts one requirement entry per
// ingredient INSTANCE (a furnace needs 8 cobblestone entries, not 1), so
// deduplicating made recipes look craftable with a single item in stock.
func ingredientInstances(cells []int32) []int32 {
	var out []int32
	for _, c := range cells {
		if c != 0 {
			out = append(out, c)
		}
	}
	return out
}
