package protocol

import (
	"bytes"
	"testing"
)

// 26.1 restructured dimension_type: `effects` was replaced by an `attributes`
// map carrying every visual and lighting property, and `skybox` /
// `cardinal_light` were added. We inline dimension_type for EVERY client
// version (the bounds are load-bearing), so a 1.21.5-shaped entry OVERRODE the
// client's correct 26.x one and left the nether with the overworld's skybox,
// face shading and ambient tint.
func TestNetherCarriesIts26xLighting(t *testing.T) {
	// The values are vanilla the_nether.json, identical in 26.1, 26.2 and 26.3.
	for _, v := range []int32{775, 776, 777} {
		nbt, ok := RegistryEntryDataFor("minecraft:dimension_type", "minecraft:the_nether", v)
		if !ok {
			t.Fatalf("v%d: the nether should carry inline data", v)
		}
		for _, want := range []string{
			"skybox", "none",
			"cardinal_light", "nether",
			"attributes",
			"minecraft:visual/ambient_light_color", "#302821",
			"minecraft:visual/sky_light_color", "#7a7aff",
			"minecraft:visual/sky_light_factor",
			"minecraft:visual/fog_start_distance",
			"minecraft:visual/fog_end_distance",
		} {
			if !bytes.Contains(nbt, []byte(want)) {
				t.Errorf("v%d: the nether entry is missing %q", v, want)
			}
		}
	}
	// A 1.21.5-era client must NOT be sent them: it has no attributes system,
	// and the fields did not exist.
	nbt, _ := RegistryEntryDataFor("minecraft:dimension_type", "minecraft:the_nether", 770)
	for _, absent := range []string{"attributes", "cardinal_light", "skybox"} {
		if bytes.Contains(nbt, []byte(absent)) {
			t.Errorf("v770 should not carry %q", absent)
		}
	}
	if !bytes.Contains(nbt, []byte("effects")) {
		t.Error("v770 still wants the old effects field")
	}
}

// The End's visuals moved the same way, and it is the dimension whose ambient
// light vanilla actually sets (0.25).
func TestEndCarriesIts26xLighting(t *testing.T) {
	nbt, ok := RegistryEntryDataFor("minecraft:dimension_type", "minecraft:the_end", 776)
	if !ok {
		t.Fatal("the End should carry inline data")
	}
	for _, want := range []string{"skybox", "end", "minecraft:visual/ambient_light_color", "#3f473f", "#ac60cd"} {
		if !bytes.Contains(nbt, []byte(want)) {
			t.Errorf("the End entry is missing %q", want)
		}
	}
}

// The attributes compound must be well-formed NBT: our writer opens it by
// name and closes it, and the whole entry still parses.
func TestDimensionEntriesStayWellFormed(t *testing.T) {
	for _, name := range []string{"minecraft:overworld", "minecraft:the_nether", "minecraft:the_end"} {
		for _, v := range []int32{770, 772, 775, 776, 777} {
			nbt, ok := RegistryEntryDataFor("minecraft:dimension_type", name, v)
			if !ok || len(nbt) == 0 {
				t.Fatalf("%s v%d: no data", name, v)
			}
			if got := nbt[len(nbt)-1]; got != 0x00 {
				t.Errorf("%s v%d: entry does not end with TAG_End, got %#x", name, v, got)
			}
			if opens, closes := bytes.Count(nbt, []byte{0x0a}), bytes.Count(nbt, []byte{0x00}); closes < 1 || opens < 1 {
				t.Errorf("%s v%d: %d compound opens, %d ends", name, v, opens, closes)
			}
		}
	}
}
