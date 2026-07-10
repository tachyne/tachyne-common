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
