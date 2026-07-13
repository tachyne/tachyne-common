package render770

import (
	"bytes"
	"testing"

	"github.com/tachyne/tachyne-common/protocol"
)

// Re-parse the whole update_recipes body at 770 and 776: empty item sets,
// then one (Ingredient, SlotDisplay) row per generated recipe.
func TestUpdateRecipesReparse(t *testing.T) {
	for _, version := range []int32{770, 776} {
		pkt := UpdateRecipes(version)
		if pkt.ID != IDUpdateRecipes {
			t.Fatalf("id 0x%x", pkt.ID)
		}
		br := bytes.NewReader(pkt.Body)
		if n, _ := protocol.ReadVarInt(br); n != 0 {
			t.Fatalf("v%d: itemSets %d, want 0", version, n)
		}
		n, _ := protocol.ReadVarInt(br)
		if int(n) != len(protocol.StonecuttingRecipes) {
			t.Fatalf("v%d: %d rows, want %d", version, n, len(protocol.StonecuttingRecipes))
		}
		sdItem, sdStack := int32(2), int32(3)
		if version >= 775 {
			sdItem, sdStack = 4, 5
		}
		for i, r := range protocol.StonecuttingRecipes {
			if hs, _ := protocol.ReadVarInt(br); hs != 2 {
				t.Fatalf("v%d row %d: holder-set header %d", version, i, hs)
			}
			in, _ := protocol.ReadVarInt(br)
			if got := protocol.UnmapID(protocol.RegItem, version, in); got != r.In {
				t.Fatalf("v%d row %d: input %d unmaps to %d, want %d", version, i, in, got, r.In)
			}
			typ, _ := protocol.ReadVarInt(br)
			switch {
			case r.Count == 1:
				if typ != sdItem {
					t.Fatalf("v%d row %d: display type %d, want %d", version, i, typ, sdItem)
				}
				out, _ := protocol.ReadVarInt(br)
				if got := protocol.UnmapID(protocol.RegItem, version, out); got != r.Out {
					t.Fatalf("v%d row %d: out %d, want %d", version, i, got, r.Out)
				}
			default:
				if typ != sdStack {
					t.Fatalf("v%d row %d: display type %d, want %d", version, i, typ, sdStack)
				}
				a, _ := protocol.ReadVarInt(br) // ≤774: count first; 775+: id first
				bv, _ := protocol.ReadVarInt(br)
				id, cnt := bv, a
				if version >= 775 {
					id, cnt = a, bv
					if x, _ := protocol.ReadVarInt(br); x != 0 {
						t.Fatalf("v%d row %d: add-components %d", version, i, x)
					}
					if x, _ := protocol.ReadVarInt(br); x != 0 {
						t.Fatalf("v%d row %d: del-components %d", version, i, x)
					}
				} else {
					// ≤774 slot form ends with a 0 components varint pair.
					if x, _ := protocol.ReadVarInt(br); x != 0 {
						t.Fatalf("v%d row %d: slot add-components %d", version, i, x)
					}
					if x, _ := protocol.ReadVarInt(br); x != 0 {
						t.Fatalf("v%d row %d: slot del-components %d", version, i, x)
					}
				}
				if got := protocol.UnmapID(protocol.RegItem, version, id); got != r.Out || cnt != int32(r.Count) {
					t.Fatalf("v%d row %d: out %d x%d, want %d x%d", version, i, got, cnt, r.Out, r.Count)
				}
			}
		}
		if br.Len() != 0 {
			t.Fatalf("v%d: %d trailing bytes", version, br.Len())
		}
	}
}
