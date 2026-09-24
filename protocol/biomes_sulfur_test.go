package protocol

import "testing"

// sulfur_caves is appended for 26.x: its id is its index in what that client
// is sent, never plains.
func TestSulfurCavesBiomeID(t *testing.T) {
	for _, v := range []int32{776, 777} {
		want := int32(-1)
		for i, n := range biomeEntriesFor(v, sharedBiomes()) {
			if n == "minecraft:sulfur_caves" {
				want = int32(i)
			}
		}
		if want < 0 {
			t.Fatalf("v%d is not sent sulfur_caves", v)
		}
		if got := BiomeIDFor(v, "minecraft:sulfur_caves"); got != want || got == BiomePlainsID {
			t.Errorf("v%d sulfur_caves id %d, want %d", v, got, want)
		}
	}
}
