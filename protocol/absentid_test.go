package protocol

import "testing"

// registrySize is each client version's registry length, from the vanilla
// datagen reports. An ID at or beyond this cannot be decoded by that client.
var registrySize = map[IDSpace]map[int32]int32{
	RegItem:   {770: 1396, 776: 1537},
	RegEntity: {770: 150, 776: 158},
	RegBlock:  {770: 1104, 776: 1196},
}

// The invariant that matters: nothing RemapID produces may fall outside the
// client's registry. Canonical IDs run higher than an older client's registry
// (items reach 1504 against 1396 entries on 770), so an absent ID passed
// through unshifted lands past the end and the client cannot decode the packet
// at all — a disconnect, not a wrong icon.
func TestRemappedIDsStayInsideTheClientRegistry(t *testing.T) {
	canonMax := map[IDSpace]int32{RegItem: 1504, RegEntity: 156, RegBlock: 1165}
	for reg, sizes := range registrySize {
		for version, size := range sizes {
			for id := int32(0); id <= canonMax[reg]; id++ {
				// Entities take a SEMANTIC substitute rather than air — a happy
				// ghast should read as a ghast, not vanish — so mirror the
				// pipeline the callers use: substitute, then shift.
				in := id
				if reg == RegEntity {
					in = substituteEntityType(version, id)
				}
				if got := RemapID(reg, version, in); got < 0 || got >= size {
					t.Fatalf("registry %d proto %d: canonical %d remapped to %d, outside 0..%d",
						reg, version, id, got, size-1)
				}
			}
		}
	}
}

// Absent entries must become the fallback rather than passing through, and
// present ones must be untouched by the new branch.
func TestAbsentIDsSubstituteRatherThanPassThrough(t *testing.T) {
	// Canonical items that 1.21.5 has no concept of resolve to air.
	absent := 0
	for id := int32(0); id < int32(len(canonicalItemIDs)); id++ {
		if !IDPresent(RegItem, 770, id) {
			absent++
			if got := RemapID(RegItem, 770, id); got != 0 {
				t.Errorf("absent item %d became %d, want air", id, got)
			}
		}
	}
	if absent == 0 {
		t.Fatal("no absent items found — this test would prove nothing")
	}
	// On a 26.x client an item is absent exactly when that client's own
	// registry lacks its name — none while the canonical version was older
	// than 26.2, 26.3's additions once 26.3 is canonical.
	for _, v := range []int32{776, 777} {
		client := ClientItems(v)
		for name, id := range canonicalItemIDs {
			_, has := client["minecraft:"+name]
			if got := IDPresent(RegItem, v, id); got != has {
				t.Errorf("proto %d: %s present=%v, but the client's registry has it=%v", v, name, got, has)
			}
		}
	}
}

// Entity substitution has to COVER every absent id, or the invariant above
// only passes because the test happens to mirror an incomplete table.
func TestEverySubstitutedEntityResolvesInsideTheRegistry(t *testing.T) {
	// Every canonical entity a client lacks, as the generated tables say — not
	// a hand-kept list of them — needs a stand-in that the client does have.
	checked := 0
	for _, version := range ServedVersions() {
		for _, id := range absentIDs[RegEntity][version] {
			sub := substituteEntityType(version, id)
			if sub == id {
				t.Errorf("proto %d: canonical entity %d has no stand-in", version, id)
				continue
			}
			if !IDPresent(RegEntity, version, sub) {
				t.Errorf("proto %d: entity %d stands in as %d, which that client lacks too", version, id, sub)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("no absent entities on any version — this test would prove nothing")
	}
}
