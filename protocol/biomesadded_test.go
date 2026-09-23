package protocol

import "testing"

// Dappled forest (26.3) is declared to 26.3 clients only — a 26.2 client's
// built-in pack lacks it — and a 26.2 client is shown forest instead.
func TestDappledForestPerVersion(t *testing.T) {
	shared := sharedBiomes()
	e776, e777 := biomeEntriesFor(776, shared), biomeEntriesFor(777, shared)
	if len(e777) != len(e776)+1 || e777[len(e777)-1] != "minecraft:dappled_forest" {
		t.Fatalf("26.3 biome registry must end with dappled_forest: %v", e777[len(e776):])
	}
	for _, e := range e776 {
		if e == "minecraft:dappled_forest" {
			t.Fatal("26.2 must not be declared dappled_forest")
		}
	}
	if got := BiomeIDFor(777, "minecraft:dappled_forest"); got != int32(len(e777)-1) {
		t.Errorf("26.3 dappled_forest id %d, want %d", got, len(e777)-1)
	}
	if got, want := BiomeIDFor(776, "minecraft:dappled_forest"), BiomeID("minecraft:forest"); got != want {
		t.Errorf("26.2 dappled_forest id %d, want forest's %d", got, want)
	}
	if BiomeIDFor(777, "minecraft:plains") != BiomeID("minecraft:plains") {
		t.Error("shared biomes keep their ids")
	}
	// The registry actually sent lists it for 26.3 and its tags can name it.
	if _, ok := dynamic26xIndexFor(777)["minecraft:worldgen/biome"]["minecraft:dappled_forest"]; !ok {
		t.Error("26.3's tags cannot resolve dappled_forest")
	}
	if _, ok := dynamic26xIndexFor(776)["minecraft:worldgen/biome"]["minecraft:dappled_forest"]; ok {
		t.Error("26.2's tags resolve a biome it was never sent")
	}
}
