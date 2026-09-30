package protocol

import (
	"bytes"
	"testing"
)

// Expected bytes here are written out field by field from the vanilla
// stream codecs — ResolvableProfile (1.21.5's partial; 1.21.9+'s
// either(GAME_PROFILE, Partial) + PlayerSkin.Patch), Identifier, Unit and
// AdventureModePredicate — with component ids from the per-version
// data_component_type registry reports: unbreakable 4 everywhere,
// can_place_on 11/14, can_break 12/15, profile 61/68/70/70/72 and
// note_block_sound 62/69/71/71/73 at 770/774/775/776/777.

var testUUID = [16]byte{0x06, 0x9a, 0x79, 0xf4, 0x44, 0xe9, 0x40, 0x26, 0xbf, 0xa4, 0x10, 0x10, 0x10, 0x10, 0x10, 0x01}

// headSlot is a canonical player_head Slot whose patch is the given
// components (id + payload, already encoded).
func headSlot(n int32, comps []byte) []byte {
	b := AppendVarInt(nil, 1)
	b = AppendVarInt(b, CanonicalItem("player_head"))
	b = AppendVarInt(b, n)
	b = AppendVarInt(b, 0)
	return append(b, comps...)
}

// texturesProps is GAME_PROFILE_PROPERTIES holding one signed property.
func texturesProps() []byte {
	b := AppendVarInt(nil, 1)
	b = AppendString(b, "textures")
	b = AppendString(b, "e30=")
	b = append(b, 1)
	return AppendString(b, "c2ln")
}

// canonicalProfile is 1.21.5's ResolvableProfile.STREAM_CODEC.
func canonicalProfile(name string, id bool, props []byte) []byte {
	b := []byte{}
	if name != "" {
		b = AppendString(append(b, 1), name)
	} else {
		b = append(b, 0)
	}
	if id {
		b = append(append(b, 1), testUUID[:]...)
	} else {
		b = append(b, 0)
	}
	return append(b, props...)
}

func TestProfileComponentPerVersion(t *testing.T) {
	full := canonicalProfile("Notch", true, texturesProps())
	nameOnly := canonicalProfile("Notch", false, AppendVarInt(nil, 0))
	for _, tc := range []struct {
		version, comp int32
	}{{774, 68}, {775, 70}, {776, 70}, {777, 72}} {
		id := func(i int32) int32 { return RemapID(RegItem, tc.version, i) }
		back := func(i int32) int32 { return UnmapID(RegItem, tc.version, i) }

		// Name and id: the left of the either, a whole GAME_PROFILE (UUID,
		// name, properties), then an empty skin patch.
		body := headSlot(1, append(AppendVarInt(nil, componentProfile), full...))
		want := AppendVarInt(nil, 1)
		want = AppendVarInt(want, id(CanonicalItem("player_head")))
		want = append(want, 1, 0)
		want = AppendVarInt(want, tc.comp)
		want = append(want, 1)
		want = append(want, testUUID[:]...)
		want = AppendString(want, "Notch")
		want = append(want, texturesProps()...)
		want = append(want, 0, 0, 0, 0)
		var out []byte
		if !copyFullSlot(bytes.NewReader(body), &out, id, tc.version, false) {
			t.Fatalf("v%d: the profile was refused", tc.version)
		}
		if !bytes.Equal(out, want) {
			t.Errorf("v%d full:\n got %x\nwant %x", tc.version, out, want)
		}
		var rt []byte
		if !copyFullSlot(bytes.NewReader(out), &rt, back, tc.version, true) || !bytes.Equal(rt, body) {
			t.Errorf("v%d full round trip:\n got %x\nwant %x", tc.version, rt, body)
		}

		// A name alone: the right of the either, the partial as it is.
		body = headSlot(1, append(AppendVarInt(nil, componentProfile), nameOnly...))
		want = AppendVarInt(nil, 1)
		want = AppendVarInt(want, id(CanonicalItem("player_head")))
		want = append(want, 1, 0)
		want = AppendVarInt(want, tc.comp)
		want = append(want, 0, 1)
		want = AppendString(want, "Notch")
		want = append(want, 0, 0) // no id, no properties
		want = append(want, 0, 0, 0, 0)
		out = nil
		if !copyFullSlot(bytes.NewReader(body), &out, id, tc.version, false) {
			t.Fatalf("v%d: the name-only profile was refused", tc.version)
		}
		if !bytes.Equal(out, want) {
			t.Errorf("v%d name only:\n got %x\nwant %x", tc.version, out, want)
		}
		rt = nil
		if !copyFullSlot(bytes.NewReader(out), &rt, back, tc.version, true) || !bytes.Equal(rt, body) {
			t.Errorf("v%d name-only round trip:\n got %x\nwant %x", tc.version, rt, body)
		}
	}
	// 1.21.5 is the canonical form itself.
	body := headSlot(1, append(AppendVarInt(nil, componentProfile), full...))
	var out []byte
	same := func(i int32) int32 { return i }
	if !copyFullSlot(bytes.NewReader(body), &out, same, 770, false) || !bytes.Equal(out, body) {
		t.Errorf("v770:\n got %x\nwant %x", out, body)
	}
}

// A 26.3 client's profile with a skin patch: the patch has no canonical
// field and is read past; the profile itself comes back.
func TestProfileSkinPatchServerbound(t *testing.T) {
	wire := AppendVarInt(nil, 1)
	wire = AppendVarInt(wire, RemapID(RegItem, 777, CanonicalItem("player_head")))
	wire = append(wire, 1, 0)
	wire = AppendVarInt(wire, 72)
	wire = append(wire, 0, 1) // a partial, with a name
	wire = AppendString(wire, "Steve")
	wire = append(wire, 0, 0)                                                  // no id, no properties
	wire = AppendString(append(wire, 1), "minecraft:entity/player/wide/steve") // body texture
	wire = append(wire, 0, 0)                                                  // no cape, no elytra
	wire = append(wire, 1, 1)                                                  // model: slim

	want := headSlot(1, append(AppendVarInt(nil, componentProfile), canonicalProfile("Steve", false, AppendVarInt(nil, 0))...))
	var out []byte
	if !copyFullSlot(bytes.NewReader(wire), &out, func(i int32) int32 { return UnmapID(RegItem, 777, i) }, 777, true) {
		t.Fatal("the client's profile was refused")
	}
	if !bytes.Equal(out, want) {
		t.Errorf("\n got %x\nwant %x", out, want)
	}
	// A truncated patch is refused.
	var bad []byte
	if copyFullSlot(bytes.NewReader(wire[:len(wire)-1]), &bad, func(i int32) int32 { return i }, 777, true) {
		t.Error("a truncated skin patch was copied")
	}
}

func TestUnbreakableAndNoteBlockSoundPerVersion(t *testing.T) {
	const sound = "minecraft:block.note_block.harp"
	comps := AppendVarInt(nil, componentUnbreakable) // a Unit: no payload
	comps = AppendVarInt(comps, componentNoteBlockSound)
	comps = AppendString(comps, sound)
	body := headSlot(2, comps)
	for _, tc := range []struct{ version, unb, note int32 }{
		{770, 4, 62}, {774, 4, 69}, {775, 4, 71}, {776, 4, 71}, {777, 4, 73},
	} {
		id := func(i int32) int32 { return RemapID(RegItem, tc.version, i) }
		want := AppendVarInt(nil, 1)
		want = AppendVarInt(want, id(CanonicalItem("player_head")))
		want = append(want, 2, 0)
		want = AppendVarInt(want, tc.unb)
		want = AppendVarInt(want, tc.note)
		want = AppendString(want, sound)
		var out []byte
		if !copyFullSlot(bytes.NewReader(body), &out, id, tc.version, false) {
			t.Fatalf("v%d: refused", tc.version)
		}
		if !bytes.Equal(out, want) {
			t.Errorf("v%d:\n got %x\nwant %x", tc.version, out, want)
		}
		var rt []byte
		if !copyFullSlot(bytes.NewReader(out), &rt, func(i int32) int32 { return UnmapID(RegItem, tc.version, i) }, tc.version, true) || !bytes.Equal(rt, body) {
			t.Errorf("v%d round trip:\n got %x\nwant %x", tc.version, rt, body)
		}
	}
}

// advPredicate builds an AdventureModePredicate of two BlockPredicates: a
// block list (ids) with nothing else, and a tag with an exact and a ranged
// state property.
func advPredicate(ids []int32) []byte {
	b := AppendVarInt(nil, 2)
	// 1: blocks = the listed ids; no properties, no nbt; empty matchers.
	b = append(b, 1)
	b = AppendVarInt(b, int32(len(ids))+1)
	for _, id := range ids {
		b = AppendVarInt(b, id)
	}
	b = append(b, 0, 0, 0, 0)
	// 2: blocks = #minecraft:logs; properties axis=y, age in [1,); no nbt.
	b = append(b, 1)
	b = AppendVarInt(b, 0)
	b = AppendString(b, "minecraft:logs")
	b = append(b, 1)
	b = AppendVarInt(b, 2)
	b = AppendString(b, "axis")
	b = append(b, 1) // exact
	b = AppendString(b, "y")
	b = AppendString(b, "age")
	b = append(b, 0)                    // ranged
	b = AppendString(append(b, 1), "1") // min
	b = append(b, 0)                    // no max
	return append(b, 0, 0, 0)           // no nbt; empty exact, empty partial
}

func TestAdventurePredicatesPerVersion(t *testing.T) {
	const stone, poplarPlanks, shifted = 1, 23, 30
	canon := advPredicate([]int32{stone, poplarPlanks, shifted})
	comps := AppendVarInt(nil, componentCanBreak)
	comps = append(comps, canon...)
	comps = AppendVarInt(comps, componentCanPlaceOn)
	comps = append(comps, canon...)
	body := headSlot(2, comps)

	// 26.3 has every block: ids and layout ride through; only the
	// component ids renumber.
	id777 := func(i int32) int32 { return RemapID(RegItem, 777, i) }
	want := AppendVarInt(nil, 1)
	want = AppendVarInt(want, id777(CanonicalItem("player_head")))
	want = append(want, 2, 0)
	want = AppendVarInt(want, 15)
	want = append(want, canon...)
	want = AppendVarInt(want, 14)
	want = append(want, canon...)
	var out []byte
	if !copyFullSlot(bytes.NewReader(body), &out, id777, 777, false) {
		t.Fatal("v777: refused")
	}
	if !bytes.Equal(out, want) {
		t.Errorf("v777:\n got %x\nwant %x", out, want)
	}

	// 26.2 has no poplar planks: it leaves the list, and the other ids
	// shift to 26.2's block registry.
	if IDPresent(RegBlock, 776, poplarPlanks) {
		t.Fatal("poplar planks present on 26.2: pick another absent block")
	}
	id776 := func(i int32) int32 { return RemapID(RegItem, 776, i) }
	c776 := advPredicate([]int32{RemapID(RegBlock, 776, stone), RemapID(RegBlock, 776, shifted)})
	want = AppendVarInt(nil, 1)
	want = AppendVarInt(want, id776(CanonicalItem("player_head")))
	want = append(want, 2, 0)
	want = AppendVarInt(want, 15)
	want = append(want, c776...)
	want = AppendVarInt(want, 14)
	want = append(want, c776...)
	out = nil
	if !copyFullSlot(bytes.NewReader(body), &out, id776, 776, false) {
		t.Fatal("v776: refused")
	}
	if !bytes.Equal(out, want) {
		t.Errorf("v776:\n got %x\nwant %x", out, want)
	}
	// And back: the 26.2 ids return to canonical.
	var rt []byte
	if !copyFullSlot(bytes.NewReader(out), &rt, func(i int32) int32 { return UnmapID(RegItem, 776, i) }, 776, true) {
		t.Fatal("v776: the client's predicates were refused")
	}
	wantBack := AppendVarInt(nil, componentCanBreak)
	wantBack = append(wantBack, advPredicate([]int32{stone, shifted})...)
	wantBack = AppendVarInt(wantBack, componentCanPlaceOn)
	wantBack = append(wantBack, advPredicate([]int32{stone, shifted})...)
	if want := headSlot(2, wantBack); !bytes.Equal(rt, want) {
		t.Errorf("v776 round trip:\n got %x\nwant %x", rt, want)
	}
}

// Component matchers the copier does not walk are refused, not guessed.
func TestAdventurePredicateRefusesComponentMatchers(t *testing.T) {
	p := AppendVarInt(nil, 1)
	p = append(p, 0, 0, 0) // no blocks, no properties, no nbt
	p = AppendVarInt(p, 1) // one exact component: not walked
	p = AppendVarInt(p, componentDamage)
	p = AppendVarInt(p, 3)
	p = AppendVarInt(p, 0)
	body := headSlot(1, append(AppendVarInt(nil, componentCanBreak), p...))
	var out []byte
	if copyFullSlot(bytes.NewReader(body), &out, func(i int32) int32 { return i }, 777, false) {
		t.Error("an exact component matcher was copied")
	}
}

// The creative path: a 26.3 client's head with a profile, unbreakable and
// can_break arrives canonical, and the canonical patch walks.
func TestCreativeSlotProfileAndAdventure(t *testing.T) {
	head := CanonicalItem("player_head")
	prof := []byte{1}
	prof = append(prof, testUUID[:]...)
	prof = AppendString(prof, "Notch")
	prof = append(prof, texturesProps()...)
	prof = append(prof, 0, 0, 0, 0)
	pred := advPredicate([]int32{1})
	body := delimitedCreativeSlot(RemapID(RegItem, 777, head), [][2][]byte{
		{AppendVarInt(nil, 72), prof},
		{AppendVarInt(nil, 4), nil},
		{AppendVarInt(nil, 15), pred},
	})
	got := unmapCreativeSlot(777, body)
	wantProf := canonicalProfile("Notch", true, texturesProps())
	want := AppendI16(nil, 36)
	want = AppendVarInt(want, 1)
	want = AppendVarInt(want, head)
	want = append(want, 3, 0)
	want = AppendVarInt(want, componentProfile)
	want = append(want, wantProf...)
	want = AppendVarInt(want, componentUnbreakable)
	want = AppendVarInt(want, componentCanBreak)
	want = append(want, pred...)
	if !bytes.Equal(got, want) {
		t.Fatalf("\n got %x\nwant %x", got, want)
	}
	var ids []int32
	ok := WalkCanonicalComponents(want[2+1+len(AppendVarInt(nil, head)):], func(id int32, payload []byte) {
		ids = append(ids, id)
		switch id {
		case componentProfile:
			if !bytes.Equal(payload, wantProf) {
				t.Errorf("profile payload %x", payload)
			}
		case componentUnbreakable:
			if len(payload) != 0 {
				t.Errorf("unbreakable payload %x", payload)
			}
		case componentCanBreak:
			if !bytes.Equal(payload, pred) {
				t.Errorf("can_break payload %x", payload)
			}
		}
	})
	if !ok || len(ids) != 3 {
		t.Errorf("walk: ok=%v ids=%v", ok, ids)
	}
}
