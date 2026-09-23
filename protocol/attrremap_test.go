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
		{"armor", attr("armor"), 0, 1},
		{"jump_strength", attr("jump_strength"), 14, 19},
		{"max_health", attr("max_health"), 18, 23},
		{"movement_speed", attr("movement_speed"), 21, 26},
		{"scale", attr("scale"), 24, 30},
	} {
		if got := RemapID(RegAttribute, 770, c.canon); got != c.v770 {
			t.Errorf("%s: canonical %d -> 770 gave %d, want %d", c.name, c.canon, got, c.v770)
		}
		if got := RemapID(RegAttribute, 776, c.canon); got != c.v776 {
			t.Errorf("%s: canonical %d -> 776 gave %d, want %d", c.name, c.canon, got, c.v776)
		}
		// The canonical protocol must pass straight through.
		if got := RemapID(RegAttribute, CanonicalProtocol, c.canon); got != c.canon {
			t.Errorf("%s: canonical %d -> canonical gave %d, want itself", c.name, c.canon, got)
		}
	}
}

// The 1.21.x versions between canonical and 770 share the canonical registry,
// so they must be identity rather than accidentally inheriting 770's shift.
func TestAttributeIDsIdenticalOnNearbyVersions(t *testing.T) {
	for _, v := range []int32{771, 772, 773, 775} {
		if js := attr("jump_strength"); RemapID(RegAttribute, v, js) != js {
			got := RemapID(RegAttribute, v, js)
			t.Errorf("proto %d shifted jump_strength to %d, want it unchanged", v, got)
		}
	}
}

// Serverbound translation has to invert cleanly, or a client's own attribute
// ids would be misread coming back.
func TestAttributeIDsRoundTrip(t *testing.T) {
	for _, v := range []int32{770, 776} {
		for canon := int32(0); canon < int32(len(canonicalAttributeIDs)); canon++ {
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
	lacking := []int32{attr("camera_distance"), attr("waypoint_transmit_range"), attr("waypoint_receive_range")}
	for _, canon := range lacking {
		if IDPresent(RegAttribute, 770, canon) {
			t.Errorf("canonical attribute %d reported present on 770", canon)
		}
	}
	if !IDPresent(RegAttribute, 770, attr("jump_strength")) {
		t.Error("jump_strength should be present on 770")
	}
	// 26.2 dropped none of them.
	for _, canon := range lacking {
		if !IDPresent(RegAttribute, 776, canon) {
			t.Errorf("canonical attribute %d reported absent on 776", canon)
		}
	}
}

// attr is a canonical attribute id by name, for fixtures: the canonical
// registry's order moves with the canonical version.
func attr(name string) int32 {
	id, ok := AttributeID("minecraft:" + name)
	if !ok {
		panic("no attribute " + name)
	}
	return id
}
