package protocol

import (
	"bytes"
	"testing"
)

// decodeTags parses an Update Tags payload into registry -> tag -> ids.
func decodeTags(t *testing.T, body []byte) map[string]map[string][]int32 {
	t.Helper()
	br := bytes.NewReader(body)
	rv := func() int32 {
		v, err := ReadVarInt(br)
		if err != nil {
			t.Fatalf("varint: %v", err)
		}
		return v
	}
	rs := func() string {
		s, err := ReadString(br)
		if err != nil {
			t.Fatalf("string: %v", err)
		}
		return s
	}
	out := map[string]map[string][]int32{}
	nReg := rv()
	for i := int32(0); i < nReg; i++ {
		reg := rs()
		m := map[string][]int32{}
		for j, n := int32(0), rv(); j < n; j++ {
			tag := rs()
			var ids []int32
			for k, c := int32(0), rv(); k < c; k++ {
				ids = append(ids, rv())
			}
			m[tag] = ids
		}
		out[reg] = m
	}
	if br.Len() != 0 {
		t.Fatalf("%d leftover bytes", br.Len())
	}
	return out
}

func TestTags776HaveRealContents(t *testing.T) {
	tags := decodeTags(t, tags26x(776))

	// mineable/pickaxe drives client-side mining speed — it must contain stone
	// (in the 26.2 BLOCK registry id space).
	pick := tags["minecraft:block"]["minecraft:mineable/pickaxe"]
	if len(pick) < 100 {
		t.Fatalf("mineable/pickaxe should list hundreds of blocks, got %d", len(pick))
	}
	stone := block26xID["minecraft:stone"]
	found := false
	for _, id := range pick {
		if id == stone {
			found = true
		}
	}
	if !found {
		t.Fatalf("mineable/pickaxe must contain stone (id %d)", stone)
	}

	// damage_type is_fire resolves against OUR declared order and must be
	// in-range for the declared list (1.21.5 entries + 26.x extras).
	fire := tags["minecraft:damage_type"]["minecraft:is_fire"]
	if len(fire) == 0 {
		t.Fatal("is_fire should have entries")
	}
	var declared int32
	for _, reg := range SyncedRegistries {
		if reg.ID == "minecraft:damage_type" {
			declared = int32(len(reg.Entries) + len(extra26xEntries[reg.ID]))
		}
	}
	for _, id := range fire {
		if id < 0 || id >= declared {
			t.Fatalf("is_fire id %d out of declared range %d", id, declared)
		}
	}

	// fluid water tag must reference the static fluid ids.
	water := tags["minecraft:fluid"]["minecraft:water"]
	if len(water) != 2 { // water + flowing_water
		t.Fatalf("fluid water tag should have 2 entries, got %v", water)
	}

	// Undeclared/datapack-only registries: skip-set absent, enchantment empty.
	if _, ok := tags["minecraft:worldgen/world_preset"]; ok {
		t.Fatal("skip-set registry must not be sent")
	}
	for tag, ids := range tags["minecraft:enchantment"] {
		if len(ids) != 0 {
			t.Fatalf("enchantment tag %s must stay empty (registry never declared)", tag)
		}
	}
}

func TestTags775StayEmpty(t *testing.T) {
	tags := decodeTags(t, tags26x(775))
	for reg, m := range tags {
		for tag, ids := range m {
			// The one deliberate exception: fluid water/lava tags carry their
			// static ids everywhere — the client's swim physics need them.
			if reg == "minecraft:fluid" && (tag == "minecraft:water" || tag == "minecraft:lava") {
				if len(ids) != 2 {
					t.Fatalf("775 fluid %s must carry its 2 static ids, has %d", tag, len(ids))
				}
				continue
			}
			if len(ids) != 0 {
				t.Fatalf("775 must get empty tags, %s/%s has %d", reg, tag, len(ids))
			}
		}
	}
}

// TestFluidTagsCarrySwimPhysicsEverywhere: #minecraft:water must have content
// on EVERY version — the client's swim physics are tag-driven, and an empty
// water tag makes players sink like stones (live bug report).
func TestFluidTagsCarrySwimPhysicsEverywhere(t *testing.T) {
	for name, body := range map[string][]byte{"legacy": tagsLegacy(), "775": tags26x(775), "776": tags26x(776)} {
		tags := decodeTags(t, body)
		water := tags["minecraft:fluid"]["minecraft:water"]
		if len(water) != 2 {
			t.Fatalf("%s: #minecraft:water must list water+flowing_water, got %v (fluid tags: %v)", name, water, tags["minecraft:fluid"])
		}
	}
}
