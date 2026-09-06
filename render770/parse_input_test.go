package render770

import (
	"testing"

	"github.com/tachyne/tachyne-common/attach"
)

// player_input is one flag byte in vanilla's Input bit order; every bit must
// land on its own field (the direction keys drive a server-rolled minecart).
func TestParseInputBits(t *testing.T) {
	cases := []struct {
		flags byte
		want  attach.Input
	}{
		{0x00, attach.Input{}},
		{0x01, attach.Input{Forward: true}},
		{0x02, attach.Input{Backward: true}},
		{0x04, attach.Input{Left: true}},
		{0x08, attach.Input{Right: true}},
		{0x10, attach.Input{Jump: true}},
		{0x20, attach.Input{Sneak: true}},
		{0x40, attach.Input{Sprint: true}},
		{0x61, attach.Input{Forward: true, Sneak: true, Sprint: true}},
	}
	for _, c := range cases {
		got, ok := ParseInput([]byte{c.flags})
		if !ok || got != c.want {
			t.Errorf("flags %#x: got %+v ok=%v, want %+v", c.flags, got, ok, c.want)
		}
	}
	if _, ok := ParseInput(nil); ok {
		t.Error("an empty payload must not parse")
	}
}
