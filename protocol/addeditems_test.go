package protocol

import (
	"bytes"
	"strings"
	"testing"
)

// LegionZA, in game on 26.3: "placing poplar planks into my hotbar turns to
// redstone ore. So i cannot place poplar."
//
// Poplar is 26.3 content and the engine's registry is 1.21.11, which has no
// poplar at all. The reverse shift table cannot say so — it only moves ids —
// so 26.3's poplar_planks (72) came back as canonical 72, which is redstone
// ore, and the world stored that. These ids have to be dropped instead.
const (
	poplarPlanks263 = 72  // 26.3 item id (a 26.3 client's own numbering)
	redstoneOre263  = 101 // 26.3 item id for redstone_ore
)

// While the canonical registry lacks poplar, the tests below replay the
// report; once the canonical version has it, no served client carries
// content the engine lacks and they have nothing to test.
func requireCanonicalLacksPoplar(t *testing.T) {
	t.Helper()
	if _, ok := canonicalItemIDs["poplar_planks"]; ok {
		t.Skip("the canonical registry has poplar: no served client carries content the engine lacks")
	}
}

func TestIDAddedMarksContentTheEngineNeverHad(t *testing.T) {
	requireCanonicalLacksPoplar(t)
	canonRedstone := CanonicalItem("redstone_ore")
	if !IDAdded(RegItem, 777, poplarPlanks263) {
		t.Error("26.3 poplar_planks should be flagged as having no canonical counterpart")
	}
	if IDAdded(RegItem, 777, redstoneOre263) {
		t.Error("26.3 redstone_ore exists canonically — it must not be dropped")
	}
	// The canonical registry is itself never "added".
	if IDAdded(RegItem, CanonicalProtocol, poplarPlanks263) {
		t.Error("the canonical version has no added ids")
	}
	// Sanity: the id that caused the report really does collide the way the
	// player saw it, so this test fails loudly if the tables are regenerated
	// into a different shape.
	if got := UnmapID(RegItem, 777, poplarPlanks263); got != canonRedstone {
		t.Errorf("unshifted 26.3 id %d lands on canonical %d, want the %d that was reported",
			poplarPlanks263, got, canonRedstone)
	}
}

// The creative slot is the path the player used: picking poplar out of the
// 26.3 creative menu and dropping it in the hotbar.
func TestUnmapCreativeSlotDropsContentTheEngineNeverHad(t *testing.T) {
	requireCanonicalLacksPoplar(t)
	canonRedstone := CanonicalItem("redstone_ore")
	body := AppendI16(nil, 36) // hotbar slot
	body = append(body, slotBytes(poplarPlanks263)...)

	out := unmapServerboundIDs(777, canonSetCreativeSlot, body)
	r := bytes.NewReader(out)
	if !skip(r, 2) {
		t.Fatal("slot index went missing")
	}
	count, err := ReadVarInt(r)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		item, _ := ReadVarInt(r)
		t.Fatalf("poplar planks came through as item %d (count %d); the slot should be empty", item, count)
	}

	// An item that does exist still translates, rather than the guard eating
	// everything it sees.
	body = AppendI16(nil, 36)
	body = append(body, slotBytes(redstoneOre263)...)
	out = unmapServerboundIDs(777, canonSetCreativeSlot, body)
	r = bytes.NewReader(out)
	skip(r, 2)
	ReadVarInt(r) // count
	if item, _ := ReadVarInt(r); item != canonRedstone {
		t.Errorf("26.3 redstone ore unmapped to %d, want canonical %d", item, canonRedstone)
	}
}

// The same hazard reaches the world through a container click, whose hashed
// slots carry item ids too.
func TestUnmapWindowClickDropsContentTheEngineNeverHad(t *testing.T) {
	requireCanonicalLacksPoplar(t)
	canonRedstone := CanonicalItem("redstone_ore")
	hashed := func(item int32) []byte {
		b := []byte{1}            // has item
		b = AppendVarInt(b, item) // id
		b = AppendVarInt(b, 1)    // count
		b = AppendVarInt(b, 0)    // added components
		return AppendVarInt(b, 0) // removed components
	}
	body := AppendVarInt(nil, 0) // window
	body = AppendVarInt(body, 1) // stateId
	body = append(body, 0, 0, 0) // slot i16 + button i8
	body = AppendVarInt(body, 0) // mode
	body = AppendVarInt(body, 1) // one changed slot
	body = append(body, 0, 0)    // its location i16
	body = append(body, hashed(poplarPlanks263)...)
	body = append(body, hashed(redstoneOre263)...) // the cursor

	out := unmapServerboundIDs(777, canonWindowClick, body)
	r := bytes.NewReader(out)
	for i := 0; i < 4; i++ { // window, stateId, (slot+button), mode
		if i == 2 {
			skip(r, 3)
			continue
		}
		ReadVarInt(r)
	}
	if n, _ := ReadVarInt(r); n != 1 {
		t.Fatalf("changed-slot count = %d, want 1", n)
	}
	skip(r, 2) // location
	has, err := r.ReadByte()
	if err != nil {
		t.Fatal(err)
	}
	if has != 0 {
		item, _ := ReadVarInt(r)
		t.Fatalf("a clicked poplar plank came through as item %d; the slot should be empty", item)
	}
	// The cursor, which the engine does have, survives translated.
	if has, err = r.ReadByte(); err != nil || has != 1 {
		t.Fatalf("cursor has=%d err=%v, want a present item", has, err)
	}
	if item, _ := ReadVarInt(r); item != canonRedstone {
		t.Errorf("cursor unmapped to %d, want canonical %d", item, canonRedstone)
	}
}

// Legion's follow-up asked the right question: "can you check all items that
// are new vs the old ones? I suspect there are a few incorrectly numbered
// items." This is that check, as an invariant over the whole registry rather
// than a spot check.
//
// Two directions have to hold for every id on a served version:
//
//	canonical -> client -> canonical   for every item the client also has
//	client -> canonical -> client      for every client id NOT flagged added
//
// A shift table that is subtly wrong breaks one of them. An item that exists
// on only one side is not a numbering error — it is the case IDPresent (out)
// and IDAdded (in) exist to catch, and it must be flagged rather than shifted.
// canonItemCount is the canonical item registry's size (ids
// 0..canonItemCount-1), which grows only when the canonical version does.
var canonItemCount = len(canonicalItemIDs)

func TestItemIDsRoundTripOnEveryServedVersion(t *testing.T) {
	// The registry has to be walked to its real end and no further: an id
	// past the end is not an item, and the tail shift range would happily
	// move it, which says nothing about correctness.
	for _, version := range []int32{770, 771, 772, 773, 775, 776, 777} {
		if !HasRemap(RegItem, version) {
			continue
		}
		// A served version holds what canonical holds, less what it lacks,
		// plus what it added — so the two bounds follow from the tables.
		canonMax := int32(canonItemCount)
		clientMax := canonMax - int32(len(absentIDs[RegItem][version])) +
			int32(len(addedIDs[RegItem][version]))
		for c := int32(0); c < canonMax; c++ {
			if !IDPresent(RegItem, version, c) {
				continue // the engine drops these rather than sending them
			}
			if back := UnmapID(RegItem, version, RemapID(RegItem, version, c)); back != c {
				t.Errorf("proto %d: canonical item %d renders as %d and comes back as %d",
					version, c, RemapID(RegItem, version, c), back)
			}
		}
		for v := int32(0); v < clientMax; v++ {
			if IDAdded(RegItem, version, v) {
				continue // no canonical counterpart; dropped, never shifted
			}
			if back := RemapID(RegItem, version, UnmapID(RegItem, version, v)); back != v {
				t.Errorf("proto %d: client item %d unmaps to %d and comes back as %d",
					version, v, UnmapID(RegItem, version, v), back)
			}
		}
	}
}

// Whatever the canonical version, a served client's item is flagged "added"
// exactly when the canonical registry lacks its name — so it is dropped
// rather than shifted onto some other item. (26.3's poplar was the case that
// bit: canonical 72 was redstone ore.)
func TestIDAddedIsWhatTheCanonicalRegistryLacks(t *testing.T) {
	for _, v := range []int32{776, 777} {
		flagged := 0
		for name, id := range ClientItems(v) {
			_, canon := canonicalItemIDs[strings.TrimPrefix(name, "minecraft:")]
			if got := IDAdded(RegItem, v, id); got == canon {
				t.Errorf("proto %d: %s (client %d) added=%v, but canonical has it=%v", v, name, id, got, canon)
			}
			if !canon {
				flagged++
			}
		}
		if flagged != len(addedIDs[RegItem][v]) {
			t.Errorf("proto %d: %d client items lack a canonical name, the table flags %d", v, flagged, len(addedIDs[RegItem][v]))
		}
	}
}
