package render770

// stonecutter.go — update_recipes (canonical 0x7e). Since 1.21.5 this packet
// carries only the station item-property sets and the STONECUTTER recipe
// list (Ingredient + result SlotDisplay per row); the client filters it by
// the menu's input, preserving order, and clicks send container_button_click
// with the row index. The list is composed from the shared generated table
// (protocol.StonecuttingRecipes) — the same slice the engine's menu uses, so
// the indices agree. Item ids are raw (no body rewriter): remapped here at
// the source, like recipe_book_add.

import "github.com/tachyne/tachyne-common/protocol"

// IDUpdateRecipes is the canonical-770 clientbound update_recipes id.
const IDUpdateRecipes = 0x7e

// UpdateRecipes composes the packet for the given client protocol version.
// The item-set map is sent empty — the client defaults each set, matching
// its state when the packet is never sent at all.
func UpdateRecipes(version int32) Packet {
	rid := func(id int32) int32 { return protocol.RemapID(protocol.RegItem, version, id) }
	sd := slotDisplayIDs{item: 2, itemStack: 3}
	if version >= 775 {
		sd = slotDisplayIDs{item: 4, itemStack: 5, templateForm: true}
	}
	b := protocol.AppendVarInt(nil, 0) // itemSets: none
	b = protocol.AppendVarInt(b, int32(len(protocol.StonecuttingRecipes)))
	for _, r := range protocol.StonecuttingRecipes {
		// Ingredient: a holder set in explicit-list form (count+1, then ids).
		b = protocol.AppendVarInt(b, 2)
		b = protocol.AppendVarInt(b, rid(r.In))
		b = appendSlotDisplay(b, sd, rid(r.Out), int(r.Count))
	}
	return Packet{IDUpdateRecipes, b}
}
