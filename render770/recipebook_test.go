package render770

import (
	"testing"

	attach "github.com/tachyne/tachyne-common/attach"
)

// TestRecipeBookShape checks the recipe_book_add renders with the right packet
// id and a plausible body, and that the 26.1+ (>=775) SlotDisplay type shift
// changes the encoding (a stack count of 2 uses item_stack type 3 at 770, 5 at
// 775+). It is a structural smoke test, not a full byte oracle.
func TestRecipeBookShape(t *testing.T) {
	rb := attach.RecipeBook{
		Shaped: []attach.ShapedRecipe{
			{W: 1, H: 2, Cells: []int32{5, 5}, Result: 280, Count: 4}, // planks -> sticks
		},
		Shapeless: []attach.ShapelessRecipe{
			{Ingredients: []int32{5}, Result: 5, Count: 1},
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
	rb := attach.RecipeBook{
		Replace: false,
		Shaped: []attach.ShapedRecipe{
			{ID: 7, W: 1, H: 1, Cells: []int32{5}, Result: 280, Count: 1,
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
