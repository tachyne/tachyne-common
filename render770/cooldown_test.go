package render770

import (
	"bytes"
	"testing"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

func TestCooldownBytes(t *testing.T) {
	p := Cooldown(attach.ItemCooldown{Group: "minecraft:shield", Ticks: 100})
	want := protocol.AppendString(nil, "minecraft:shield")
	want = protocol.AppendVarInt(want, 100)
	if p.ID != IDCooldown || !bytes.Equal(p.Body, want) {
		t.Fatalf("id %#x body %x want %x", p.ID, p.Body, want)
	}
	if c := Cooldown(attach.ItemCooldown{Group: "minecraft:goat_horn"}); c.Body[len(c.Body)-1] != 0 {
		t.Fatal("ending a cooldown writes 0 ticks")
	}
}
