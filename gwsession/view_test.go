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
