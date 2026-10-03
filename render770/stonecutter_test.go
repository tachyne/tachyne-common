package render770

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/tachyne/tachyne-common/attach"

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
		nsets, _ := protocol.ReadVarInt(br)
		if int(nsets) != len(protocol.RecipePropertySets) {
			t.Fatalf("v%d: itemSets %d, want %d", version, nsets, len(protocol.RecipePropertySets))
		}
		for i := int32(0); i < nsets; i++ {
			key, err := protocol.ReadString(br)
			if err != nil || key != protocol.RecipePropertySets[i].Key {
				t.Fatalf("v%d: set key %q (%v)", version, key, err)
			}
			cnt, _ := protocol.ReadVarInt(br)
			if cnt <= 0 {
				t.Fatalf("v%d: set %q empty", version, key)
			}
			for j := int32(0); j < cnt; j++ {
				if _, err := protocol.ReadVarInt(br); err != nil {
					t.Fatalf("v%d: set %q item %d: %v", version, key, j, err)
				}
			}
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
			// A row naming an item the client lacks stays (the buttons must line
			// up with the engine's list) as its stand-in, or a barrier — never
			// air, which the client refuses.
			if !protocol.IDPresent(protocol.RegItem, version, r.In) {
				if in == 0 {
					t.Fatalf("v%d row %d: absent input sent as air", version, i)
				}
			} else if got := protocol.UnmapID(protocol.RegItem, version, in); got != r.In {
				t.Fatalf("v%d row %d: input %d unmaps to %d, want %d", version, i, in, got, r.In)
			}
			typ, _ := protocol.ReadVarInt(br)
			switch {
			case !protocol.IDPresent(protocol.RegItem, version, r.Out):
				if typ != 0 {
					t.Fatalf("v%d row %d: absent output has display type %d, want 0 (empty)", version, i, typ)
				}
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

// A stonecutter input a served client lacks would go out as air, and vanilla's
// Ingredient refuses air ("Ingredient can't contain air") — the client drops
// the connection decoding update_recipes. Outputs may be absent (they render
// as an empty display); inputs may not.
func TestStonecutterInputsExistOnServedClients(t *testing.T) {
	for _, v := range protocol.ServedVersions() {
		for i, r := range protocol.StonecuttingRecipes {
			if !protocol.IDPresent(protocol.RegItem, v, r.In) {
				t.Errorf("v%d row %d: input %d is absent on the client", v, i, r.In)
			}
		}
	}
}

// staticUpdateRecipes is the packet composed straight from the generated
// tables: vanilla 26.3's property sets by name, then the stonecutter rows.
func staticUpdateRecipes(version int32) []byte {
	rid := func(id int32) int32 { return protocol.RemapID(protocol.RegItem, version, id) }
	sd := slotDisplayIDs{item: 2, itemStack: 3}
	if version >= 775 {
		sd = slotDisplayIDs{item: 4, itemStack: 5, templateForm: true}
	}
	b := protocol.AppendVarInt(nil, int32(len(protocol.RecipePropertySets)))
	for _, s := range protocol.RecipePropertySets {
		b = protocol.AppendString(b, s.Key)
		var ids []int32
		for _, n := range s.Items {
			if id := protocol.CanonicalItem(n); protocol.IDPresent(protocol.RegItem, version, id) {
				ids = append(ids, rid(id))
			}
		}
		b = protocol.AppendVarInt(b, int32(len(ids)))
		for _, id := range ids {
			b = protocol.AppendVarInt(b, id)
		}
	}
	b = protocol.AppendVarInt(b, int32(len(protocol.StonecuttingRecipes)))
	for _, r := range protocol.StonecuttingRecipes {
		b = protocol.AppendVarInt(b, 2)
		b = protocol.AppendVarInt(b, rid(r.In))
		out := rid(r.Out)
		if !protocol.IDPresent(protocol.RegItem, version, r.Out) {
			out = 0
		}
		b = appendSlotDisplay(b, sd, out, int(r.Count))
	}
	return b
}

// Vanilla's nine property sets, keys as RecipePropertySet registers them,
// none empty; the cookers' and the brewing stand's hold what their recipes
// take.
func TestVanillaPropertySets(t *testing.T) {
	want := []string{"smithing_base", "smithing_template", "smithing_addition", "furnace_input",
		"blast_furnace_input", "smoker_input", "campfire_input", "brewing_input", "brewing_reagent"}
	sets := VanillaRecipes().ItemSets
	if len(sets) != len(want) {
		t.Fatalf("%d sets", len(sets))
	}
	has := func(key, item string) bool {
		for _, s := range sets {
			if s.Key == key {
				for _, id := range s.Items {
					if id == protocol.CanonicalItem(item) {
						return true
					}
				}
			}
		}
		return false
	}
	for i, s := range sets {
		if s.Key != "minecraft:"+want[i] || len(s.Items) == 0 {
			t.Errorf("set %d: %s (%d items)", i, s.Key, len(s.Items))
		}
	}
	for _, c := range [][2]string{
		{"furnace_input", "iron_ore"}, {"furnace_input", "raw_iron"}, {"blast_furnace_input", "raw_gold"},
		{"smoker_input", "beef"}, {"campfire_input", "potato"}, {"brewing_input", "splash_potion"},
		{"brewing_reagent", "nether_wart"}, {"brewing_reagent", "blaze_powder"},
		{"smithing_template", "netherite_upgrade_smithing_template"}, {"smithing_base", "diamond_sword"},
		{"smithing_addition", "netherite_ingot"},
	} {
		if !has("minecraft:"+c[0], c[1]) {
			t.Errorf("%s lacks %s", c[0], c[1])
		}
	}
	if has("minecraft:smoker_input", "iron_ore") || has("minecraft:brewing_input", "nether_wart") {
		t.Error("a set holds an item its recipes do not take")
	}
}

// The world's recipes, when they are vanilla's, render byte for byte as the
// packet composed from the generated tables — through the frame's JSON, as
// the gateway receives it.
func TestUpdateRecipesFromVanillaMatchesStatic(t *testing.T) {
	raw, err := json.Marshal(VanillaRecipes())
	if err != nil {
		t.Fatal(err)
	}
	var u attach.UpdateRecipes
	if err := json.Unmarshal(raw, &u); err != nil {
		t.Fatal(err)
	}
	for _, v := range []int32{776, 777} {
		want := staticUpdateRecipes(v)
		if got := UpdateRecipes(v).Body; !bytes.Equal(got, want) {
			t.Fatalf("v%d: static packet changed", v)
		}
		if got := UpdateRecipesFrom(v, &u).Body; !bytes.Equal(got, want) {
			t.Fatalf("v%d: vanilla frame renders differently from the static packet", v)
		}
		// A frame with neither half (JSON nulls) is the generated table too.
		var none attach.UpdateRecipes
		json.Unmarshal([]byte(`{"item_sets":null}`), &none)
		if got := UpdateRecipesFrom(v, &none).Body; !bytes.Equal(got, want) {
			t.Fatalf("v%d: nil halves do not fall back", v)
		}
	}
}

// A data pack's recipes: its own property sets (furnace_input included),
// a tag ingredient, an explicit list, and an empty stonecutter list ([] is
// none, not the fallback). Expected bytes from the STREAM_CODECs:
// map(ResourceKey, list(Item holder)) then list(holderSet, SlotDisplay).
func TestUpdateRecipesFromWorld(t *testing.T) {
	stone := protocol.CanonicalItem("stone")
	slab := protocol.CanonicalItem("stone_slab")
	coal := protocol.CanonicalItem("coal")
	var u attach.UpdateRecipes
	if err := json.Unmarshal([]byte(fmt.Sprintf(`{"item_sets":[{"key":"minecraft:furnace_input","items":[%d,%d]}],
		"stonecutter":[{"input":{"tag":"minecraft:stone_crafting_materials"},"result":%d,"count":2},
		{"input":{"items":[%d,%d]},"result":%d,"count":1}]}`, stone, coal, slab, stone, coal, stone)), &u); err != nil {
		t.Fatal(err)
	}
	v := int32(777)
	r := func(id int32) int32 { return protocol.RemapID(protocol.RegItem, v, id) }
	var w []byte
	w = protocol.AppendVarInt(w, 1)
	w = protocol.AppendString(w, "minecraft:furnace_input")
	w = protocol.AppendVarInt(w, 2)
	w = protocol.AppendVarInt(w, r(stone))
	w = protocol.AppendVarInt(w, r(coal))
	w = protocol.AppendVarInt(w, 2) // two rows
	w = protocol.AppendVarInt(w, 0) // tag form
	w = protocol.AppendString(w, "minecraft:stone_crafting_materials")
	w = protocol.AppendVarInt(w, 5) // item_stack (26.1+ template form)
	w = protocol.AppendVarInt(w, r(slab))
	w = protocol.AppendVarInt(w, 2)
	w = protocol.AppendVarInt(w, 0)
	w = protocol.AppendVarInt(w, 0)
	w = protocol.AppendVarInt(w, 3) // two items + 1
	w = protocol.AppendVarInt(w, r(stone))
	w = protocol.AppendVarInt(w, r(coal))
	w = protocol.AppendVarInt(w, 4) // item
	w = protocol.AppendVarInt(w, r(stone))
	if got := UpdateRecipesFrom(v, &u).Body; !bytes.Equal(got, w) {
		t.Fatalf("got  %x\nwant %x", got, w)
	}

	empty := attach.UpdateRecipes{Stonecutter: []attach.StonecutterRecipe{}}
	raw, _ := json.Marshal(empty)
	var back attach.UpdateRecipes
	json.Unmarshal(raw, &back)
	body := UpdateRecipesFrom(v, &back).Body
	br := bytes.NewReader(body)
	nsets, _ := protocol.ReadVarInt(br)
	for i := int32(0); i < nsets; i++ {
		protocol.ReadString(br)
		n, _ := protocol.ReadVarInt(br)
		for j := int32(0); j < n; j++ {
			protocol.ReadVarInt(br)
		}
	}
	if n, _ := protocol.ReadVarInt(br); n != 0 || br.Len() != 0 {
		t.Fatalf("empty stonecutter list: %d rows, %d trailing", n, br.Len())
	}
}

// An explicit ingredient whose items the client lacks entirely keeps its
// row (button indices) with a barrier, never an empty or air ingredient.
func TestUpdateRecipesIngredientNeverAir(t *testing.T) {
	var absent int32 = -1
	for _, v := range protocol.ServedVersions() {
		for id := int32(1); id < 2000; id++ {
			if protocol.RemapID(protocol.RegItem, v, id) == 0 && !protocol.IDPresent(protocol.RegItem, v, id) {
				absent = id
				break
			}
		}
	}
	in := attach.RecipeIngredient{Items: []int32{absent}}
	if absent < 0 {
		in = attach.RecipeIngredient{Items: []int32{}}
	}
	b := appendIngredient(nil, in, func(id int32) int32 { return protocol.RemapID(protocol.RegItem, 776, id) })
	br := bytes.NewReader(b)
	n, _ := protocol.ReadVarInt(br)
	id, _ := protocol.ReadVarInt(br)
	if n != 2 || id == 0 || id != protocol.RemapID(protocol.RegItem, 776, barrierItem) {
		t.Fatalf("ingredient %x", b)
	}
}
