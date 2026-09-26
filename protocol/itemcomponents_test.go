package protocol

import (
	"bytes"
	"testing"
)

// Expected bytes in these tests are written out by hand from each target's
// stream codecs, with component ids counted from DataComponents'
// registration order — never produced by the code under test.

// nbtTranslatable is a network-NBT {translate: key} compound.
func nbtTranslatable(key string) []byte {
	b := []byte{0x0a, 0x08, 0x00, 9}
	b = append(b, "translate"...)
	b = append(b, byte(len(key)>>8), byte(len(key)))
	b = append(b, key...)
	return append(b, 0x00)
}

// ominousLayers is Raid.getBannerComponentPatch's layers as (holder, dye)
// pairs; the holder values only have to ride through unchanged.
var ominousLayers = [][2]int32{{31, 9}, {29, 8}, {26, 7}, {3, 8}, {28, 15}, {17, 8}, {5, 8}, {3, 15}}

func ominousBannerCanonical() []byte {
	b := AppendVarInt(nil, 1)
	b = AppendVarInt(b, CanonicalItem("white_banner"))
	b = AppendVarInt(b, 4)
	b = AppendVarInt(b, 0)
	b = AppendVarInt(b, componentBannerPatterns)
	b = AppendVarInt(b, int32(len(ominousLayers)))
	for _, l := range ominousLayers {
		b = AppendVarInt(b, l[0])
		b = AppendVarInt(b, l[1])
	}
	b = AppendVarInt(b, componentTooltipDisplay)
	b = append(b, 0) // not hidden whole
	b = AppendVarInt(b, 1)
	b = AppendVarInt(b, componentBannerPatterns)
	b = AppendVarInt(b, componentItemName)
	b = append(b, nbtTranslatable("block.minecraft.ominous_banner")...)
	b = AppendVarInt(b, componentRarity)
	return AppendVarInt(b, 1) // UNCOMMON
}

func TestOminousBannerComponentsPerVersion(t *testing.T) {
	for _, tc := range []struct {
		version                       int32
		banner, tooltip, name, rarity int32
	}{
		{776, 72, 18, 9, 12},
		{777, 74, 18, 9, 12},
	} {
		id := func(i int32) int32 { return RemapID(RegItem, tc.version, i) }
		want := AppendVarInt(nil, 1)
		want = AppendVarInt(want, id(CanonicalItem("white_banner")))
		want = append(want, 4, 0)
		want = AppendVarInt(want, tc.banner)
		want = append(want, 8)
		for _, l := range ominousLayers {
			want = append(want, byte(l[0]), byte(l[1]))
		}
		want = AppendVarInt(want, tc.tooltip)
		want = append(want, 0, 1)
		want = AppendVarInt(want, tc.banner) // the hidden set names the client's id
		want = AppendVarInt(want, tc.name)
		want = append(want, nbtTranslatable("block.minecraft.ominous_banner")...)
		want = AppendVarInt(want, tc.rarity)
		want = append(want, 1)

		body := ominousBannerCanonical()
		var out []byte
		if !copyFullSlot(bytes.NewReader(body), &out, id, tc.version, false) {
			t.Fatalf("v%d: the ominous banner's components were refused", tc.version)
		}
		if !bytes.Equal(out, want) {
			t.Errorf("v%d:\n got %x\nwant %x", tc.version, out, want)
		}
		var back []byte
		if !copyFullSlot(bytes.NewReader(out), &back, func(i int32) int32 { return UnmapID(RegItem, tc.version, i) }, tc.version, true) {
			t.Fatalf("v%d: the client's banner could not be lifted back", tc.version)
		}
		if !bytes.Equal(back, body) {
			t.Errorf("v%d round trip:\n got %x\nwant %x", tc.version, back, body)
		}
	}
}

// Through set_equipment, which is how a captain's head reaches the client.
func TestOminousBannerOnACaptainsHead(t *testing.T) {
	body := AppendVarInt(nil, 42) // eid
	body = append(body, 5)        // head, last entry
	body = append(body, ominousBannerCanonical()...)
	out := remapEquipment(777, body)
	if bytes.Equal(out, body) {
		t.Fatal("set_equipment came back untouched: the banner's components were refused")
	}
	r := bytes.NewReader(out)
	ReadVarInt(r)
	r.ReadByte()
	ReadVarInt(r) // count
	ReadVarInt(r) // item
	if n, _ := ReadVarInt(r); n != 4 {
		t.Fatalf("%d components, want 4", n)
	}
}

// A hidden component the copier cannot renumber is refused, not guessed.
func TestTooltipDisplayRefusesAnUnknownHiddenID(t *testing.T) {
	b := AppendVarInt(nil, 1)
	b = AppendVarInt(b, 1)
	b = append(b, 1, 0)
	b = AppendVarInt(b, componentTooltipDisplay)
	b = append(b, 0, 1)
	b = AppendVarInt(b, 1) // max_stack_size: not one the copier knows
	var out []byte
	if copyFullSlot(bytes.NewReader(b), &out, func(i int32) int32 { return i }, 777, false) {
		t.Error("a tooltip_display hiding an unknown component was copied")
	}
}

func TestComponentIDsInvert(t *testing.T) {
	for _, v := range []int32{770, 776, 777} { // the canonical side and the served clients
		seen := map[int32]int32{}
		for _, c := range knownComponents {
			id, ok := componentIDAt(c, v)
			if !ok {
				t.Fatalf("v%d: no id for canonical %d", v, c)
			}
			if prev, dup := seen[id]; dup {
				t.Errorf("v%d: canonical %d and %d both map to %d", v, prev, c, id)
			}
			seen[id] = c
			if back, ok := componentIDFrom(id, v); !ok || back != c {
				t.Errorf("v%d: %d → %d → %d", v, c, id, back)
			}
		}
	}
}

// A loaded crossbow: charged_projectiles holds the arrows, canonically as
// Slots (count, item, components), on 26.x as ItemStackTemplates (item,
// count, components). The tipped arrow's own potion_contents renumbers too.
func TestChargedProjectilesPerVersion(t *testing.T) {
	arrow, tipped, xbow := CanonicalItem("arrow"), CanonicalItem("tipped_arrow"), CanonicalItem("crossbow")
	potion := []byte{1, 7, 0, 0, 0} // holder, no colour, no effects, no name
	body := AppendVarInt(nil, 1)
	body = AppendVarInt(body, xbow)
	body = append(body, 1, 0)
	body = AppendVarInt(body, componentChargedProj)
	body = append(body, 2)
	body = append(body, 1)
	body = AppendVarInt(body, arrow)
	body = append(body, 0, 0)
	body = append(body, 1)
	body = AppendVarInt(body, tipped)
	body = append(body, 1, 0)
	body = AppendVarInt(body, componentPotionContents)
	body = append(body, potion...)

	for _, tc := range []struct{ version, charged, potion int32 }{{776, 49, 51}, {777, 51, 53}} {
		id := func(i int32) int32 { return RemapID(RegItem, tc.version, i) }
		want := AppendVarInt(nil, 1)
		want = AppendVarInt(want, id(xbow))
		want = append(want, 1, 0)
		want = AppendVarInt(want, tc.charged)
		want = append(want, 2)
		want = AppendVarInt(want, id(arrow))
		want = append(want, 1, 0, 0)
		want = AppendVarInt(want, id(tipped))
		want = append(want, 1, 1, 0)
		want = AppendVarInt(want, tc.potion)
		want = append(want, potion...)

		var out []byte
		if !copyFullSlot(bytes.NewReader(body), &out, id, tc.version, false) {
			t.Fatalf("v%d: charged_projectiles was refused", tc.version)
		}
		if !bytes.Equal(out, want) {
			t.Errorf("v%d:\n got %x\nwant %x", tc.version, out, want)
		}
		var back []byte
		if !copyFullSlot(bytes.NewReader(out), &back, func(i int32) int32 { return UnmapID(RegItem, tc.version, i) }, tc.version, true) {
			t.Fatalf("v%d: the client's crossbow could not be lifted back", tc.version)
		}
		if !bytes.Equal(back, body) {
			t.Errorf("v%d round trip:\n got %x\nwant %x", tc.version, back, body)
		}
	}
}

// A tropical fish bucket: the three variant components the tooltip reads,
// and bucket_entity_data, a compound carried verbatim.
func TestMobBucketComponentsPerVersion(t *testing.T) {
	health := []byte{0x0a, 0x05, 0x00, 6, 'H', 'e', 'a', 'l', 't', 'h', 0x40, 0x40, 0x00, 0x00, 0x00}
	bucket := CanonicalItem("tropical_fish_bucket")
	body := AppendVarInt(nil, 1)
	body = AppendVarInt(body, bucket)
	body = append(body, 4, 0)
	body = AppendVarInt(body, componentFishPattern)
	body = AppendVarInt(body, 256) // SUNSTREAK: small base, index 1
	body = AppendVarInt(body, componentFishBaseColor)
	body = append(body, 11) // blue
	body = AppendVarInt(body, componentFishPatternColor)
	body = append(body, 7) // gray
	body = AppendVarInt(body, componentBucketEntityData)
	body = append(body, health...)

	for _, tc := range []struct{ version, pat, base, patCol, data int32 }{{776, 89, 90, 91, 59}, {777, 95, 96, 97, 61}} {
		id := func(i int32) int32 { return RemapID(RegItem, tc.version, i) }
		want := AppendVarInt(nil, 1)
		want = AppendVarInt(want, id(bucket))
		want = append(want, 4, 0)
		want = AppendVarInt(want, tc.pat)
		want = append(want, 0x80, 0x02)
		want = AppendVarInt(want, tc.base)
		want = append(want, 11)
		want = AppendVarInt(want, tc.patCol)
		want = append(want, 7)
		want = AppendVarInt(want, tc.data)
		want = append(want, health...)

		var out []byte
		if !copyFullSlot(bytes.NewReader(body), &out, id, tc.version, false) {
			t.Fatalf("v%d: the bucket's components were refused", tc.version)
		}
		if !bytes.Equal(out, want) {
			t.Errorf("v%d:\n got %x\nwant %x", tc.version, out, want)
		}
		var back []byte
		if !copyFullSlot(bytes.NewReader(out), &back, func(i int32) int32 { return UnmapID(RegItem, tc.version, i) }, tc.version, true) {
			t.Fatalf("v%d: the client's bucket could not be lifted back", tc.version)
		}
		if !bytes.Equal(back, body) {
			t.Errorf("v%d round trip:\n got %x\nwant %x", tc.version, back, body)
		}
	}
}

func TestSalmonAndAxolotlBucketsPerVersion(t *testing.T) {
	for _, tc := range []struct {
		item             string
		canon            int32
		want776, want777 int32
	}{
		{"salmon_bucket", componentSalmonSize, 87, 93},
		{"axolotl_bucket", componentAxolotlVariant, 105, 111},
	} {
		body := AppendVarInt(nil, 1)
		body = AppendVarInt(body, CanonicalItem(tc.item))
		body = append(body, 1, 0)
		body = AppendVarInt(body, tc.canon)
		body = append(body, 2)
		for v, wantID := range map[int32]int32{776: tc.want776, 777: tc.want777} {
			var out []byte
			if !copyFullSlot(bytes.NewReader(body), &out, func(i int32) int32 { return i }, v, false) {
				t.Fatalf("%s v%d: refused", tc.item, v)
			}
			r := bytes.NewReader(out)
			for i := 0; i < 4; i++ {
				ReadVarInt(r)
			}
			if cid, _ := ReadVarInt(r); cid != wantID {
				t.Errorf("%s v%d: component id %d, want %d", tc.item, v, cid, wantID)
			}
			if val, _ := ReadVarInt(r); val != 2 {
				t.Errorf("%s v%d: value %d, want 2", tc.item, v, val)
			}
		}
	}
}

// A bundle on 26.x holds templates: item, then count.
func TestBundleContentsAreTemplatesOn26x(t *testing.T) {
	diamond := CanonicalItem("diamond")
	body := AppendVarInt(nil, 1)
	body = AppendVarInt(body, CanonicalItem("bundle"))
	body = append(body, 1, 0)
	body = AppendVarInt(body, componentBundleContents)
	body = append(body, 1, 5) // one stack: five diamonds
	body = AppendVarInt(body, diamond)
	body = append(body, 0, 0)
	for _, v := range []int32{776, 777} {
		id := func(i int32) int32 { return RemapID(RegItem, v, i) }
		want := AppendVarInt(nil, 1)
		want = AppendVarInt(want, id(CanonicalItem("bundle")))
		want = append(want, 1, 0)
		want = AppendVarInt(want, bundleContentsCompID(v))
		want = append(want, 1)
		want = AppendVarInt(want, id(diamond))
		want = append(want, 5, 0, 0)
		var out []byte
		if !copyFullSlot(bytes.NewReader(body), &out, id, v, false) {
			t.Fatalf("v%d: refused", v)
		}
		if !bytes.Equal(out, want) {
			t.Errorf("v%d:\n got %x\nwant %x", v, out, want)
		}
	}
}
