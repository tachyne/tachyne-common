package gwsession

import "testing"

// 26.3's brewing stand has four data slots, 26.2's two: the extra two never
// reach a 26.2 client, and other menus are untouched.
func TestWindowDataFits(t *testing.T) {
	if windowDataFits(776, menuBrewingStand, 2) || windowDataFits(776, menuBrewingStand, 3) {
		t.Error("a 26.2 client was sent the 26.3 brewing totals")
	}
	if !windowDataFits(776, menuBrewingStand, 1) || !windowDataFits(777, menuBrewingStand, 3) {
		t.Error("slots a client has were dropped")
	}
	if !windowDataFits(776, 14, 3) { // a furnace's four slots are old
		t.Error("a furnace's data slot was dropped")
	}
}
