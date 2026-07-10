package gwsession

import "testing"

// TestEffectiveView pins the honored render-distance policy: the client's
// slider is honored up to the deployment cap; 0 (no Client Information) falls
// back to the default radius; the floor is 2. Regression guard for the
// dedup-era slowdown, where the shared pipeline shipped with the earth-mode
// cap (32) and a maxed slider meant a 65×65 chunk window for every client.
func TestEffectiveView(t *testing.T) {
	cases := []struct {
		client, cap, want int32
	}{
		{0, defaultViewCap, viewRadius}, // client never said → default radius
		{1, defaultViewCap, 2},          // floor
		{8, defaultViewCap, 8},          // honored as-is under the cap
		{32, defaultViewCap, 12},        // maxed slider clamps to the cap
		{32, 32, 32},                    // earth-mode deployment honors it
		{16, 10, 10},                    // custom low cap wins
	}
	for _, c := range cases {
		if got := effectiveView(c.client, c.cap); got != c.want {
			t.Errorf("effectiveView(%d, cap %d) = %d, want %d", c.client, c.cap, got, c.want)
		}
	}
}

// TestConfigViewCap pins the cap resolution: zero → default, huge → clamped
// to the engine's hard attach radius.
func TestConfigViewCap(t *testing.T) {
	if got := (Config{}).viewCap(); got != defaultViewCap {
		t.Errorf("zero ViewCap = %d, want default %d", got, defaultViewCap)
	}
	if got := (Config{ViewCap: 32}).viewCap(); got != 32 {
		t.Errorf("ViewCap 32 = %d, want 32", got)
	}
	if got := (Config{ViewCap: 64}).viewCap(); got != attachMaxRadius {
		t.Errorf("ViewCap 64 = %d, want the attach ceiling %d", got, attachMaxRadius)
	}
}

// TestMetaPolicyEntityIDs pins the canonical (1.21.11 / proto 774) entity-type
// ids the metadata policies key on. These are duplicated from the engine's
// generated registry (tachyne-world internal/server/entityids_gen.go) because
// this module cannot import the engine; if a canonical retarget shifts the
// registry, THIS TEST is the tripwire. Regression: after the 770→774 retarget
// they still held 1.21.5-era values — 111 had become a sheep (silently
// mis-shifted meta on 26.2) and real magma cubes went unshifted, which
// type-mismatch-disconnected every 26.2 client the moment one spawned.
func TestMetaPolicyEntityIDs(t *testing.T) {
	if typeSlime != 117 || typeMagmaCube != 80 || typeCopperGolem != 28 {
		t.Fatalf("meta-policy ids drifted from canonical 1.21.11: slime=%d magma=%d golem=%d (want 117/80/28)",
			typeSlime, typeMagmaCube, typeCopperGolem)
	}
}
