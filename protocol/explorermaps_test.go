package protocol

import (
	"bytes"
	"testing"
)

// A 26.2 client sees an explorer map as a filled map carrying an item_name
// (component 9 there): 26.2's own key where it had one, else 26.3's key with
// the English name as fallback. 26.3 sees the map as itself, unnamed.
func TestExplorerMapNamedOn262(t *testing.T) {
	for _, tc := range []struct {
		item, key, fallback string
	}{
		{"buried_treasure_map", "filled_map.buried_treasure", ""},
		{"abandoned_camp_map", "item.minecraft.abandoned_camp_map", "Abandoned Camp Map"},
	} {
		item := CanonicalItem(tc.item)
		slot := append(AppendVarInt(AppendVarInt(nil, 1), item), 0, 0)
		var out []byte
		if !copyFullSlot(bytes.NewReader(slot), &out, func(i int32) int32 { return RemapID(RegItem, 776, i) }, 776, false) {
			t.Fatalf("%s: the copier refused it", tc.item)
		}
		text := append(NBTRoot(), 8)
		text = append(AppendU16(text, 9), "translate"...)
		text = append(AppendU16(text, uint16(len(tc.key))), tc.key...)
		if tc.fallback != "" {
			text = append(append(text, 8), AppendU16(nil, 8)...)
			text = append(text, "fallback"...)
			text = append(AppendU16(text, uint16(len(tc.fallback))), tc.fallback...)
		}
		text = append(text, 0)
		want := AppendVarInt(AppendVarInt(nil, 1), RemapID(RegItem, 776, CanonicalItem("filled_map")))
		want = AppendVarInt(AppendVarInt(want, 1), 0) // one added, none removed
		want = append(AppendVarInt(want, 9), text...)
		if !bytes.Equal(out, want) {
			t.Errorf("%s on 26.2:\n got %x\nwant %x", tc.item, out, want)
		}
		out = nil
		copyFullSlot(bytes.NewReader(slot), &out, func(i int32) int32 { return RemapID(RegItem, 777, i) }, 777, false)
		if !bytes.Equal(out, slot) {
			t.Errorf("%s on 26.3 changed: %x", tc.item, out)
		}
	}
}

// 26.3's decorations 35–39 go to a 26.2 client as the icon it has.
func TestMapDecorationFor(t *testing.T) {
	for typ, want := range map[int32]int32{26: 26, 34: 34, 35: 26, 36: 26, 37: 32, 38: 26, 39: 26} {
		if got := MapDecorationFor(776, typ); got != want {
			t.Errorf("26.2 decoration %d -> %d, want %d", typ, got, want)
		}
		if got := MapDecorationFor(777, typ); got != typ {
			t.Errorf("26.3 decoration %d changed to %d", typ, got)
		}
	}
}
