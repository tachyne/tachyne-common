package render770

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// readVarInt decodes a VarInt in place, returning it and its byte width — the
// re-parser below walks a body by offset, so a ByteReader will not do.
func readVarInt(b []byte) (int32, int) {
	var v, shift int32
	for i := 0; i < len(b) && i < 5; i++ {
		v |= int32(b[i]&0x7f) << shift
		if b[i]&0x80 == 0 {
			return v, i + 1
		}
		shift += 7
	}
	return 0, 0
}

// TestRecipeBookShape checks the recipe_book_add renders with the right packet
// id and a plausible body, and that the 26.1+ (>=775) SlotDisplay type shift
// changes the encoding (a stack count of 2 uses item_stack type 3 at 770, 5 at
// 775+). It is a structural smoke test, not a full byte oracle.
func TestRecipeBookShape(t *testing.T) {
	planks, stick := protocol.CanonicalItem("oak_planks"), protocol.CanonicalItem("stick")
	rb := attach.RecipeBook{
		Shaped: []attach.ShapedRecipe{
			{W: 1, H: 2, Cells: []int32{planks, planks}, Result: stick, Count: 4}, // planks -> sticks
		},
		Shapeless: []attach.ShapelessRecipe{
			{Ingredients: []int32{planks}, Result: planks, Count: 1},
		},
	}
	p770 := RecipeBook(rb, 770)
	if p770.ID != IDRecipeBook {
		t.Fatalf("packet id = %#x, want %#x", p770.ID, IDRecipeBook)
	}
	if len(p770.Body) == 0 {
		t.Fatal("empty body")
	}
	// The count VarInt at the front must be 2 (one shaped + one shapeless).
	if p770.Body[0] != 2 {
		t.Fatalf("entry count = %d, want 2", p770.Body[0])
	}
	// The 775 encoding differs from 770 (SlotDisplay type ids shifted +
	// ItemStackTemplate form), so a result with count 4 encodes differently.
	p775 := RecipeBook(rb, 775)
	if string(p770.Body) == string(p775.Body) {
		t.Fatal("770 and 775 recipe book bodies identical — version shift not applied")
	}
}

// TestRecipeBookFlagsAndReplace pins the progression additions: engine
// display ids on the wire, the entry flags byte (1 notify | 2 highlight —
// vanilla ClientboundRecipeBookAddPacket.Entry), the frame-level replace
// bool, and the settings packet's 4×(open,filter) pairs.
func TestRecipeBookFlagsAndReplace(t *testing.T) {
	planks, stick := protocol.CanonicalItem("oak_planks"), protocol.CanonicalItem("stick")
	rb := attach.RecipeBook{
		Replace: false,
		Shaped: []attach.ShapedRecipe{
			{ID: 7, W: 1, H: 1, Cells: []int32{planks}, Result: stick, Count: 1,
				Notify: true, Highlight: true},
		},
	}
	p := RecipeBook(rb, 770)
	if p.Body[1] != 7 { // first entry's displayId VarInt right after the count
		t.Fatalf("display id on wire = %d, want 7", p.Body[1])
	}
	if last := p.Body[len(p.Body)-1]; last != 0 { // replace=false
		t.Fatalf("replace byte = %d, want 0", last)
	}
	if flags := p.Body[len(p.Body)-2]; flags != 3 { // notify|highlight
		t.Fatalf("entry flags = %d, want 3", flags)
	}

	rb.Replace = true
	rb.Shaped[0].Notify, rb.Shaped[0].Highlight = false, true
	p = RecipeBook(rb, 770)
	if last := p.Body[len(p.Body)-1]; last != 1 {
		t.Fatalf("replace byte = %d, want 1", last)
	}
	if flags := p.Body[len(p.Body)-2]; flags != 2 {
		t.Fatalf("entry flags = %d, want 2", flags)
	}

	s := RecipeBookSettings(attach.RecipeSettings{
		Open: [4]bool{true, false, false, false}, Filter: [4]bool{false, true, false, false}})
	if s.ID != IDRecipeBookSettings || len(s.Body) != 8 {
		t.Fatalf("settings packet: id %#x len %d", s.ID, len(s.Body))
	}
	want := []byte{1, 0, 0, 1, 0, 0, 0, 0} // crafting open; furnace filtering
	if string(s.Body) != string(want) {
		t.Fatalf("settings body %v, want %v", s.Body, want)
	}
}

// TestCookingRecipeDisplay re-parses a furnace book entry field by field. The
// wire shape is vanilla's FurnaceRecipeDisplay (ingredient, fuel, result,
// craftingStation, duration, experience) wrapped in a RecipeDisplayEntry
// (displayId, display, group optvarint, category, craftingRequirements,
// flags). The fuel slot is always SlotDisplay any_fuel — registry index 1 in
// every served version — and the display type id is 2 (recipe_display order:
// crafting_shapeless, crafting_shaped, furnace, stonecutter, smithing).
func TestCookingRecipeDisplay(t *testing.T) {
	const (
		rawIron      = 862 // canonical 1.21.11 item ids
		ironIngot    = 74
		furnaceItem  = 249
		furnaceBlock = 5 // recipe_book_category furnace_blocks
	)
	rb := attach.RecipeBook{Replace: true, Cooking: []attach.CookingRecipe{{
		ID: 900, Ingredient: rawIron, Result: ironIngot, Count: 1,
		Cook: 200, XP: 0.7, Station: furnaceItem, Category: furnaceBlock,
		Notify: true,
	}}}
	p := RecipeBook(rb, 770)
	// The frame carries canonical ids; the renderer remaps them into the
	// client's space, so the expectations must be remapped too.
	rid := func(id int32) int32 { return protocol.RemapID(protocol.RegItem, 770, id) }

	b := p.Body
	var pos int
	varint := func() int32 {
		v, n := readVarInt(b[pos:])
		if n <= 0 {
			t.Fatalf("bad varint at %d", pos)
		}
		pos += n
		return v
	}
	// One SlotDisplay: returns its type id and, for item/item_stack, the item.
	slot := func() (typ, item int32) {
		typ = varint()
		switch typ {
		case 0, slotDisplayAnyFuel:
			return typ, 0
		case 2: // item
			return typ, varint()
		case 3: // item_stack (≤1.21.11 form: count, id, 2 component counts)
			varint()
			item = varint()
			varint()
			varint()
			return typ, item
		}
		t.Fatalf("unexpected slot display type %d", typ)
		return
	}

	if n := varint(); n != 1 {
		t.Fatalf("entry count = %d, want 1", n)
	}
	if id := varint(); id != 900 {
		t.Fatalf("display id = %d, want 900", id)
	}
	if typ := varint(); typ != recipeDisplayFurnace {
		t.Fatalf("display type = %d, want furnace (%d)", typ, recipeDisplayFurnace)
	}
	if typ, item := slot(); typ != 2 || item != rid(rawIron) {
		t.Fatalf("ingredient slot = (%d,%d), want (2,%d)", typ, item, rid(rawIron))
	}
	if typ, _ := slot(); typ != slotDisplayAnyFuel {
		t.Fatalf("fuel slot type = %d, want any_fuel (%d)", typ, slotDisplayAnyFuel)
	}
	if typ, item := slot(); typ != 2 || item != rid(ironIngot) {
		t.Fatalf("result slot = (%d,%d), want (2,%d)", typ, item, rid(ironIngot))
	}
	if typ, item := slot(); typ != 2 || item != rid(furnaceItem) {
		t.Fatalf("station slot = (%d,%d), want (2,%d)", typ, item, rid(furnaceItem))
	}
	if d := varint(); d != 200 {
		t.Fatalf("duration = %d, want 200", d)
	}
	if xp := math.Float32frombits(binary.BigEndian.Uint32(b[pos:])); xp != 0.7 {
		t.Fatalf("experience = %v, want 0.7", xp)
	}
	pos += 4
	if g := varint(); g != ironIngot+1 {
		t.Fatalf("group = %d, want %d", g, ironIngot+1)
	}
	if c := varint(); c != furnaceBlock {
		t.Fatalf("category = %d, want %d (furnace_blocks)", c, furnaceBlock)
	}
	if b[pos] != 1 { // craftingRequirements present
		t.Fatal("craftingRequirements optional not present")
	}
	pos++
	if n := varint(); n != 1 {
		t.Fatalf("requirement count = %d, want 1", n)
	}
	if n := varint(); n != 2 { // IDSet: one direct id
		t.Fatalf("id set header = %d, want 2", n)
	}
	if got := varint(); got != rid(rawIron) {
		t.Fatalf("requirement item = %d, want %d", got, rid(rawIron))
	}
	if b[pos] != 1 { // flags: notify only
		t.Fatalf("entry flags = %d, want 1", b[pos])
	}
	pos++
	if b[pos] != 1 || pos != len(b)-1 {
		t.Fatalf("trailing replace byte = %d at %d (len %d), want 1 at the end", b[pos], pos, len(b))
	}
}

// TestCookingEntriesShiftWithVersion guards the 26.1 SlotDisplay renumbering
// for the cooking path too: a 775 client must see item type 4, not 2.
func TestCookingEntriesShiftWithVersion(t *testing.T) {
	rb := attach.RecipeBook{Cooking: []attach.CookingRecipe{
		{ID: 1, Ingredient: 862, Result: 74, Count: 1, Cook: 200, XP: 0.7, Station: 249, Category: 5},
	}}
	p770, p775 := RecipeBook(rb, 770), RecipeBook(rb, 775)
	if string(p770.Body) == string(p775.Body) {
		t.Fatal("770 and 775 cooking bodies identical — version shift not applied")
	}
	// Body: count(1), displayId(1), type(2), then the ingredient SlotDisplay.
	if p770.Body[3] != 2 {
		t.Fatalf("770 ingredient slot type = %d, want 2", p770.Body[3])
	}
	if p775.Body[3] != 4 {
		t.Fatalf("775 ingredient slot type = %d, want 4", p775.Body[3])
	}
}

// An entry naming an item the client lacks is left out: sent, the item would
// be air, and the client drops the connection on an ingredient of air. Poplar
// is 26.3's: a 26.2 client gets only the oak entry, a 26.3 client both.
func TestRecipeBookSkipsItemsTheClientLacks(t *testing.T) {
	item := protocol.CanonicalItem
	rb := attach.RecipeBook{
		Shaped: []attach.ShapedRecipe{
			{ID: 1, W: 1, H: 2, Cells: []int32{item("oak_planks"), item("oak_planks")}, Result: item("stick"), Count: 4},
			{ID: 2, W: 1, H: 1, Cells: []int32{item("poplar_log")}, Result: item("poplar_planks"), Count: 4},
		},
		Shapeless: []attach.ShapelessRecipe{
			{ID: 3, Ingredients: []int32{item("white_wool")}, Result: item("white_wool_slab"), Count: 6},
		},
		Cooking: []attach.CookingRecipe{
			{ID: 4, Ingredient: item("poplar_log"), Result: item("charcoal"), Count: 1, Station: item("furnace")},
		},
	}
	if n := RecipeBook(rb, 776).Body[0]; n != 1 {
		t.Errorf("26.2 got %d entries, want 1 (oak sticks only)", n)
	}
	if n := RecipeBook(rb, 777).Body[0]; n != 4 {
		t.Errorf("26.3 got %d entries, want 4", n)
	}
}

// place_ghost_recipe: the window, then the shaped display — its size, the
// cells, the result and the crafting table as the station.
func TestGhostRecipeLayout(t *testing.T) {
	p, ok := GhostRecipe(attach.GhostRecipe{Window: 3, Shaped: &attach.ShapedRecipe{W: 1, H: 2, Cells: []int32{5, 5}, Result: 6, Count: 4}}, 770)
	if !ok || p.ID != IDPlaceGhostRecipe {
		t.Fatalf("ok=%v id=%#x", ok, p.ID)
	}
	sd := slotDisplayIDs{item: 2, itemStack: 3}
	w := protocol.AppendVarInt(nil, 3)
	w = protocol.AppendVarInt(w, recipeDisplayShaped)
	w = protocol.AppendVarInt(protocol.AppendVarInt(w, 1), 2)
	w = protocol.AppendVarInt(w, 2)
	rid := func(id int32) int32 { return protocol.RemapID(protocol.RegItem, 770, id) }
	w = appendSlotDisplay(appendSlotDisplay(w, sd, rid(5), 1), sd, rid(5), 1)
	w = appendSlotDisplay(w, sd, rid(6), 4)
	w = appendSlotDisplay(w, sd, rid(itemCraftingTable), 1)
	if !bytes.Equal(p.Body, w) {
		t.Fatalf("body %x\nwant %x", p.Body, w)
	}
}
