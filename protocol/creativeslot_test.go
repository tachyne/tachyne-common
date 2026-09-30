package protocol

import (
	"bytes"
	"testing"
)

// A creative slot as the client sends it: slot 36, count, the client's item
// id, then a DELIMITED patch (each added value length-prefixed).
func delimitedCreativeSlot(item int32, comps [][2][]byte, removed ...int32) []byte {
	b := AppendI16(nil, 36)
	b = AppendVarInt(b, 1)
	b = AppendVarInt(b, item)
	b = AppendVarInt(b, int32(len(comps)))
	b = AppendVarInt(b, int32(len(removed)))
	for _, c := range comps {
		b = append(b, c[0]...) // component id
		b = AppendVarInt(b, int32(len(c[1])))
		b = append(b, c[1]...)
	}
	for _, id := range removed {
		b = AppendVarInt(b, id)
	}
	return b
}

// armorStandTag26x is a 26.x armor stand's entity_data tag as the client
// sends it (TypedEntityData: the id is not in it): {Small:1b}.
func armorStandTag26x() []byte {
	t := NBTRoot()
	t = NBTByte(t, "Small", 1)
	return NBTEnd(t)
}

// The creative slot reaches the pipeline canonical: the item unmapped, the
// patch undelimited, each component's id brought back (painting/variant 103
// on 26.2 and 109 on 26.3 → 89; entity_data 58 / 60 → 49), entity_data's
// type folded back into its tag as "id" (1.21.5's CustomData), components
// the copier does not know and removed components left out.
func TestCreativeSlotComponents(t *testing.T) {
	stand := CanonicalItem("armor_stand")
	for _, tc := range []struct {
		version             int32
		entityComp, paintID int32
	}{{776, 58, 103}, {777, 60, 109}} {
		standType, ok := ClientEntity(tc.version, "armor_stand")
		if !ok {
			t.Fatalf("v%d: no armor_stand", tc.version)
		}
		entity := append(AppendVarInt(nil, standType), armorStandTag26x()...)
		body := delimitedCreativeSlot(RemapID(RegItem, tc.version, stand), [][2][]byte{
			{AppendVarInt(nil, 1000), {1, 2, 3}}, // no component the copier knows
			{AppendVarInt(nil, tc.entityComp), entity},
			{AppendVarInt(nil, tc.paintID), AppendVarInt(nil, 5)},
		}, 7)
		got := unmapCreativeSlot(tc.version, body)

		wantTag := NBTString(NBTRoot(), "id", "minecraft:armor_stand")
		wantTag = NBTByte(wantTag, "Small", 1)
		wantTag = NBTEnd(wantTag)
		want := AppendI16(nil, 36)
		want = AppendVarInt(want, 1)
		want = AppendVarInt(want, stand)
		want = AppendVarInt(want, 2) // two kept
		want = AppendVarInt(want, 0) // removals left out
		want = AppendVarInt(want, ComponentEntityData770)
		want = append(want, wantTag...)
		want = AppendVarInt(want, ComponentPaintingVariant770)
		want = AppendVarInt(want, 5)
		if !bytes.Equal(got, want) {
			t.Errorf("v%d:\n got %x\nwant %x", tc.version, got, want)
		}

		// The canonical patch walks: both components, payloads exact.
		var ids []int32
		ok = WalkCanonicalComponents(want[2+1+len(AppendVarInt(nil, stand)):], func(id int32, payload []byte) {
			ids = append(ids, id)
			if id == ComponentEntityData770 && !bytes.Equal(payload, wantTag) {
				t.Errorf("entity_data payload %x", payload)
			}
		})
		if !ok || len(ids) != 2 || ids[0] != ComponentEntityData770 || ids[1] != ComponentPaintingVariant770 {
			t.Errorf("walk: ok=%v ids=%v", ok, ids)
		}
	}
	// An unreadable patch leaves the bare item.
	bad := append(AppendVarInt(AppendVarInt(AppendI16(nil, 36), 1), RemapID(RegItem, 777, stand)), 1, 0, 60, 50)
	want := append(AppendVarInt(AppendVarInt(AppendI16(nil, 36), 1), stand), 0, 0)
	if got := unmapCreativeSlot(777, bad); !bytes.Equal(got, want) {
		t.Errorf("unreadable patch: %x, want %x", got, want)
	}
}

// Clientbound, entity_data goes the other way: the canonical tag's "id"
// becomes the 26.x type id in front of the tag.
func TestEntityDataClientbound(t *testing.T) {
	tag := NBTString(NBTRoot(), "id", "minecraft:armor_stand")
	tag = NBTEnd(NBTByte(tag, "Small", 1))
	patch := append(AppendVarInt(AppendVarInt(AppendVarInt(nil, 1), 0), ComponentEntityData770), tag...)
	for _, v := range []int32{776, 777} {
		var out []byte
		if !copyComponentPatch(bytes.NewReader(patch), &out, func(i int32) int32 { return i }, v, false, 0) {
			t.Fatalf("v%d: the copier refused entity_data", v)
		}
		typ, _ := ClientEntity(v, "armor_stand")
		want := AppendVarInt(AppendVarInt(AppendVarInt(nil, 1), 0), laterCompID(ComponentEntityData770, v))
		want = append(AppendVarInt(want, typ), tag...)
		if !bytes.Equal(out, want) {
			t.Errorf("v%d:\n got %x\nwant %x", v, out, want)
		}
	}
}
