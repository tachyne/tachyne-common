package render770

import (
	"bytes"
	"testing"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// TestUpdateAttributesBytes: eid, count, then per attribute the canonical
// registry id, base, and modifiers as identifier + amount + op; unknown
// names are skipped and sources become valid identifiers.
func TestUpdateAttributesBytes(t *testing.T) {
	p := UpdateAttributes(attach.EntityAttributes{EID: 7, Attrs: []attach.AttributeSnapshot{
		{Name: "minecraft:max_health", Base: 20, Modifiers: []attach.AttributeModifier{{ID: "effect:Health Boost", Amount: 4, Op: 0}}},
		{Name: "minecraft:no_such_thing", Base: 1},
		{Name: "minecraft:movement_speed", Base: 0.1},
	}})
	if p.ID != IDUpdateAttributes {
		t.Fatalf("packet id %#x", p.ID)
	}
	want := protocol.AppendVarInt(nil, 7)
	want = protocol.AppendVarInt(want, 2)
	want = protocol.AppendVarInt(want, 19) // max_health
	want = protocol.AppendF64(want, 20)
	want = protocol.AppendVarInt(want, 1)
	want = protocol.AppendString(want, "effect:health_boost")
	want = protocol.AppendF64(want, 4)
	want = protocol.AppendVarInt(want, 0)
	want = protocol.AppendVarInt(want, 22) // movement_speed
	want = protocol.AppendF64(want, 0.1)
	want = protocol.AppendVarInt(want, 0)
	if !bytes.Equal(p.Body, want) {
		t.Fatalf("body\n got %x\nwant %x", p.Body, want)
	}
	if got := modifierIdentifier("minecraft:generic.armor"); got != "minecraft:generic.armor" {
		t.Fatalf("a valid identifier passes through: %s", got)
	}
}
