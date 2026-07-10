package render770

import (
	"testing"

	attach "github.com/tachyne/tachyne-common/attach"
)

// Ported from the gomc server's crafting_test.go when the TCP parse moved
// here (stage 6c): a hand-encoded container_click round-trips, including a
// hashed changed-slot.
func TestParseWindowClickRoundTrip(t *testing.T) {
	// window 0, state 7, slot 1... changed slot 36 -> 2 oak logs, cursor empty.
	b := []byte{0}                       // windowId
	b = append(b, 7)                     // stateId
	b = append(b, 0, 36)                 // slot i16
	b = append(b, 0)                     // button
	b = append(b, 0)                     // mode
	b = append(b, 1)                     // changedSlots count
	b = append(b, 0, 36)                 // location i16
	b = append(b, 1)                     // Option: present
	b = append(b, 134&0x7f|0x80, 134>>7) // itemId 134 varint
	b = append(b, 2)                     // count
	b = append(b, 0, 0)                  // components add/remove counts
	b = append(b, 0)                     // cursor: absent
	e, ok := ParseWindowClick(b)
	if !ok {
		t.Fatal("well-formed click failed to parse")
	}
	if e.ID != 0 || e.Slot != 36 || len(e.Changed) != 1 {
		t.Fatalf("parsed click wrong: %+v", e)
	}
	if e.Changed[0].Item.ID != 134 || e.Changed[0].Item.Count != 2 {
		t.Fatalf("changed slot wrong: %+v", e.Changed[0].Item)
	}
}

func TestParseSmallActions(t *testing.T) {
	if e, ok := ParseUseEntity([]byte{5, 1}); !ok || e.Target != 5 || !e.Attack {
		t.Fatalf("attack parse: %+v %v", e, ok)
	}
	if e, ok := ParseUseEntity([]byte{5, 0}); !ok || e.Attack {
		t.Fatalf("interact parse: %+v %v", e, ok)
	}
	if _, ok := ParseUseEntity([]byte{5, 2}); ok {
		t.Fatal("interact_at should not parse to an action")
	}
	if e, ok := ParseCreativeSlot(append([]byte{0, 44}, 2, 99)); !ok || e.Slot != 44 || e.Item.ID != 99 || e.Item.Count != 2 {
		t.Fatalf("creative slot: %+v %v", e, ok)
	}
	if e, ok := ParseRespawnReq([]byte{0}); !ok || e != (attach.RespawnReq{}) {
		t.Fatal("respawn req")
	}
	if _, ok := ParseRespawnReq([]byte{1}); ok {
		t.Fatal("client-stats action must not respawn")
	}
}
