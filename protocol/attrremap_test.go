package protocol

import "testing"

// Attribute registry ids SHIFT between served versions, which is why
// update_attributes cannot be sent as canonical bytes: 26.2 inserts eight new
// attributes among the existing ones, so almost every id moves. Sending an
// unmapped id would apply a DIFFERENT attribute on the client — silently, with
// no decode error to point at it.
//
// Anchors span the registry so a table that is merely shifted at one end
// cannot pass.
func TestAttributeIDsRemapPerVersion(t *testing.T) {
	for _, c := range []struct {
		name              string
		canon, v770, v776 int32
	}{
		{"armor", 0, 0, 1},
		{"jump_strength", 15, 14, 19},
		{"max_health", 19, 18, 23},
		{"movement_speed", 22, 21, 26},
		{"scale", 25, 24, 30},
	} {
		if got := RemapID(RegAttribute, 770, c.canon); got != c.v770 {
			t.Errorf("%s: canonical %d -> 770 gave %d, want %d", c.name, c.canon, got, c.v770)
		}
		if got := RemapID(RegAttribute, 776, c.canon); got != c.v776 {
			t.Errorf("%s: canonical %d -> 776 gave %d, want %d", c.name, c.canon, got, c.v776)
		}
		// 774 IS the canonical version, so it must pass straight through.
		if got := RemapID(RegAttribute, 774, c.canon); got != c.canon {
			t.Errorf("%s: canonical %d -> 774 gave %d, want itself", c.name, c.canon, got)
		}
	}
}

// The 1.21.x versions between canonical and 770 share the canonical registry,
// so they must be identity rather than accidentally inheriting 770's shift.
func TestAttributeIDsIdenticalOnNearbyVersions(t *testing.T) {
	for _, v := range []int32{771, 772, 773, 775} {
		if got := RemapID(RegAttribute, v, 15); got != 15 {
			t.Errorf("proto %d shifted jump_strength to %d, want 15", v, got)
		}
	}
}

// Serverbound translation has to invert cleanly, or a client's own attribute
// ids would be misread coming back.
func TestAttributeIDsRoundTrip(t *testing.T) {
	for _, v := range []int32{770, 776} {
		for canon := int32(0); canon <= 34; canon++ {
			if !IDPresent(RegAttribute, v, canon) {
				continue // no counterpart on this client; the sender drops it
			}
			if back := UnmapID(RegAttribute, v, RemapID(RegAttribute, v, canon)); back != canon {
				t.Errorf("proto %d: canonical %d round-tripped to %d", v, canon, back)
			}
		}
	}
}

// Three attributes exist canonically and not on 1.21.5. A shift range cannot
// express that, so without IDPresent they alias onto their neighbours and the
// client applies the wrong attribute with no decode error to show for it.
func TestAttributesAbsentFrom770AreNotAliased(t *testing.T) {
	for _, canon := range []int32{8, 33, 34} { // camera_distance, the two waypoint ranges
		if IDPresent(RegAttribute, 770, canon) {
			t.Errorf("canonical attribute %d reported present on 770", canon)
		}
	}
	if !IDPresent(RegAttribute, 770, 15) {
		t.Error("jump_strength should be present on 770")
	}
	// 26.2 dropped none of them.
	for _, canon := range []int32{8, 33, 34} {
		if !IDPresent(RegAttribute, 776, canon) {
			t.Errorf("canonical attribute %d reported absent on 776", canon)
		}
	}
}
