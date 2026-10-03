package protocol

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// nbtText is a text component as network NBT: a nameless TAG_String.
func nbtText(s string) []byte {
	b := []byte{nbtString}
	b = binary.BigEndian.AppendUint16(b, uint16(len(s)))
	return append(b, s...)
}

// signTextWire is SignText.STREAM_CODEC: fixedSizeList(4) of texts (no
// count), Optional<fixedSizeList(4)> (a bool, then four), the DyeColor
// id (idMapper, a VarInt) and the glowing flag (BOOL).
func signTextWire(filtered bool, color int32, glow bool) []byte {
	var b []byte
	for _, l := range []string{"Hello", "", "tachyne", ""} {
		b = append(b, nbtText(l)...)
	}
	b = AppendBool(b, filtered)
	if filtered {
		for i := 0; i < 4; i++ {
			b = append(b, nbtText("*")...)
		}
	}
	b = AppendVarInt(b, color)
	return AppendBool(b, glow)
}

func patchOf(comps ...[]byte) []byte {
	b := AppendVarInt(AppendVarInt(nil, int32(len(comps))), 0)
	for _, c := range comps {
		b = append(b, c...)
	}
	return b
}

func comp(id int32, payload []byte) []byte { return append(AppendVarInt(nil, id), payload...) }

func copyPatch(t *testing.T, patch []byte, v int32, serverbound bool) []byte {
	t.Helper()
	var out []byte
	r := bytes.NewReader(patch)
	if !copyComponentPatch(r, &out, func(i int32) int32 { return i }, v, serverbound, 0) {
		t.Fatalf("v%d: the copier refused %x", v, patch)
	}
	if r.Len() != 0 {
		t.Fatalf("v%d: %d bytes left", v, r.Len())
	}
	return out
}

// block_entity_data clientbound: the canonical compound's "id" becomes
// TypedEntityData's leading block_entity_type id (ByteBufCodecs.registry,
// a VarInt) at the client's numbering, then the compound — sign is 7 on
// both, conduit 26 canonical and 25 on 26.x; the component is 60 on 26.2
// and 62 on 26.3.
func TestBlockEntityDataClientbound(t *testing.T) {
	for _, tc := range []struct {
		name         string
		v, comp, typ int32
	}{
		{"minecraft:sign", 776, 60, 7}, {"minecraft:sign", 777, 62, 7},
		{"minecraft:conduit", 776, 60, 25}, {"minecraft:conduit", 777, 62, 25},
		{"minecraft:copper_golem_statue", 777, 62, 47},
	} {
		tag := NBTString(NBTRoot(), "id", tc.name)
		tag = NBTEnd(NBTInt(tag, "x", 3))
		got := copyPatch(t, patchOf(comp(componentBlockEntityData, tag)), tc.v, false)
		want := patchOf(comp(tc.comp, append(AppendVarInt(nil, tc.typ), tag...)))
		if !bytes.Equal(got, want) {
			t.Errorf("%s v%d:\n got %x\nwant %x", tc.name, tc.v, got, want)
		}
	}
	// A type the client lacks (26.x has no bed block entity) is refused.
	bed := NBTEnd(NBTString(NBTRoot(), "id", "minecraft:bed"))
	var out []byte
	if copyComponentPatch(bytes.NewReader(patchOf(comp(componentBlockEntityData, bed))), &out, func(i int32) int32 { return i }, 777, false, 0) {
		t.Error("a bed's block entity reached a 26.3 client")
	}
}

// Serverbound (a creative slot): the type id goes back into the compound
// as "id" and the component to its canonical 51.
func TestBlockEntityDataCreativeSlot(t *testing.T) {
	sign := CanonicalItem("oak_sign")
	for _, tc := range []struct{ v, comp int32 }{{776, 60}, {777, 62}} {
		tag := NBTEnd(NBTString(NBTRoot(), "Lock", "k"))
		body := delimitedCreativeSlot(RemapID(RegItem, tc.v, sign), [][2][]byte{
			{AppendVarInt(nil, tc.comp), append(AppendVarInt(nil, 8), tag...)}, // hanging_sign
		})
		got := unmapCreativeSlot(tc.v, body)
		wantTag := NBTString(NBTRoot(), "id", "minecraft:hanging_sign")
		wantTag = NBTEnd(NBTString(wantTag, "Lock", "k"))
		want := AppendVarInt(AppendVarInt(AppendI16(nil, 36), 1), sign)
		want = append(want, patchOf(comp(componentBlockEntityData, wantTag))...)
		if !bytes.Equal(got, want) {
			t.Errorf("v%d:\n got %x\nwant %x", tc.v, got, want)
		}
	}
}

// sign_text_front / sign_text_back / waxed (26.3, 118/119/120): copied
// through on 26.3 both ways; a 26.2 client, which has none of them, gets
// the rest of the patch and a count that leaves them out, and its
// tooltip_display does not name them.
func TestSignTextComponents(t *testing.T) {
	front, back := signTextWire(false, 14, true), signTextWire(true, 0, false)
	canon := patchOf(
		comp(componentSignTextFront, front),
		comp(componentDamage, AppendVarInt(nil, 2)),
		comp(componentSignTextBack, back),
		comp(componentWaxed, nil),
	)
	if got := copyPatch(t, canon, 777, false); !bytes.Equal(got, canon) {
		t.Errorf("26.3:\n got %x\nwant %x", got, canon)
	}
	if got, want := copyPatch(t, canon, 776, false), patchOf(comp(componentDamage, AppendVarInt(nil, 2))); !bytes.Equal(got, want) {
		t.Errorf("26.2:\n got %x\nwant %x", got, want)
	}
	// Serverbound from a 26.3 creative slot: canonical ids are 26.3's.
	body := delimitedCreativeSlot(RemapID(RegItem, 777, CanonicalItem("oak_sign")), [][2][]byte{
		{AppendVarInt(nil, 118), front}, {AppendVarInt(nil, 120), nil},
	})
	got := unmapCreativeSlot(777, body)
	want := AppendVarInt(AppendVarInt(AppendI16(nil, 36), 1), CanonicalItem("oak_sign"))
	want = append(want, patchOf(comp(componentSignTextFront, front), comp(componentWaxed, nil))...)
	if !bytes.Equal(got, want) {
		t.Errorf("creative:\n got %x\nwant %x", got, want)
	}
	// A truncated SignText is refused, not guessed at.
	var out []byte
	if copyComponentPatch(bytes.NewReader(patchOf(comp(componentSignTextFront, front[:len(front)-1]))), &out, func(i int32) int32 { return i }, 777, false, 0) {
		t.Error("a truncated sign text was copied")
	}
	// tooltip_display hiding the sign text and the damage bar.
	td := patchOf(comp(componentTooltipDisplay, append([]byte{0}, append(AppendVarInt(nil, 2), append(AppendVarInt(nil, componentSignTextFront), AppendVarInt(nil, componentDamage)...)...)...)))
	got = copyPatch(t, td, 776, false)
	want = patchOf(comp(18, append([]byte{0}, append(AppendVarInt(nil, 1), AppendVarInt(nil, componentDamage)...)...)))
	if !bytes.Equal(got, want) {
		t.Errorf("tooltip_display on 26.2:\n got %x\nwant %x", got, want)
	}
}

// The name table agrees with the registry orders blockEntityTypeFor maps:
// every client id names one type and back.
func TestBlockEntityTypeNames(t *testing.T) {
	if len(blockEntityTypeNames) != 49 || blockEntityTypeNames[25] != "bed" || blockEntityTypeNames[40] != "shelf" {
		t.Fatal("the canonical (1.21.11) order moved")
	}
	for _, v := range []int32{776, 777} {
		seen := map[int32]bool{}
		for _, n := range append(append([]string(nil), blockEntityTypeNames...), potentSulfurName) {
			id, ok := BlockEntityTypeID(v, n)
			if n == "bed" {
				if ok {
					t.Errorf("v%d: bed has an id", v)
				}
				continue
			}
			if !ok || seen[id] {
				t.Fatalf("v%d: %s → %d (%v, dup %v)", v, n, id, ok, seen[id])
			}
			seen[id] = true
			if back, ok := blockEntityTypeName(v, id); !ok || back != "minecraft:"+n {
				t.Errorf("v%d: %d → %q", v, id, back)
			}
		}
		if len(seen) != 49 {
			t.Errorf("v%d: %d types", v, len(seen))
		}
	}
}
