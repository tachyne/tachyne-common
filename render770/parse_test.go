package render770

import (
	"bytes"
	"testing"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
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
	// interact: target, type 0, then the hand and the sneak flag.
	if e, ok := ParseUseEntity([]byte{5, 0, 1, 0}); !ok || e.Hand != 1 || e.Attack {
		t.Fatalf("interact with the offhand: %+v ok=%v", e, ok)
	}
	if e, ok := ParseUseEntity([]byte{5, 0, 0, 1}); !ok || e.Hand != 0 {
		t.Fatalf("interact with the main hand: %+v ok=%v", e, ok)
	}
	if _, ok := ParseUseEntity([]byte{5, 2}); ok {
		t.Fatal("interact_at should not parse to an action")
	}
	if e, ok := ParseCreativeSlot(append([]byte{0, 44}, 2, 99), 770); !ok || e.Slot != 44 || e.Item.ID != 99 || e.Item.Count != 2 {
		t.Fatalf("creative slot: %+v %v", e, ok)
	}
	if e, ok := ParseRespawnReq([]byte{0}); !ok || e != (attach.RespawnReq{}) {
		t.Fatal("respawn req")
	}
	if _, ok := ParseRespawnReq([]byte{1}); ok {
		t.Fatal("client-stats action must not respawn")
	}
}

// TestParseSetBeacon: two Optional<Holder<MobEffect>> — presence bool +
// mob_effect registry id VarInt. The frame carries id+1 (0 = none).
func TestParseSetBeacon(t *testing.T) {
	// primary speed (id 0), secondary regeneration (id 9)
	if e, ok := ParseSetBeacon([]byte{1, 0, 1, 9}); !ok || e.Primary != 1 || e.Secondary != 10 {
		t.Fatalf("both: %+v %v", e, ok)
	}
	// primary haste (id 2), no secondary
	if e, ok := ParseSetBeacon([]byte{1, 2, 0}); !ok || e.Primary != 3 || e.Secondary != 0 {
		t.Fatalf("primary only: %+v %v", e, ok)
	}
	// neither (clearing the selection)
	if e, ok := ParseSetBeacon([]byte{0, 0}); !ok || e != (attach.SetBeacon{}) {
		t.Fatalf("none: %+v %v", e, ok)
	}
	if _, ok := ParseSetBeacon([]byte{1}); ok {
		t.Fatal("truncated body must not parse")
	}
}

// TestParseSetSlotState: container_slot_state_changed = VarInt slotId,
// VarInt containerId, Boolean newState. The container id is dropped.
func TestParseSetSlotState(t *testing.T) {
	// slot 4, container 7, newState true (enable).
	if e, ok := ParseSetSlotState([]byte{4, 7, 1}); !ok || e.Slot != 4 || !e.State {
		t.Fatalf("enable slot 4: %+v %v", e, ok)
	}
	// slot 0, container 3, newState false (disable).
	if e, ok := ParseSetSlotState([]byte{0, 3, 0}); !ok || e.Slot != 0 || e.State {
		t.Fatalf("disable slot 0: %+v %v", e, ok)
	}
	// out-of-range slot (result/preview or inventory) is rejected.
	if _, ok := ParseSetSlotState([]byte{9, 7, 0}); ok {
		t.Fatal("slot 9+ must not parse (only the 9 grid slots toggle)")
	}
	if _, ok := ParseSetSlotState([]byte{4, 7}); ok {
		t.Fatal("truncated body (no newState) must not parse")
	}
}

// TestParseChunkBatchReceived: the body is one big-endian float32 (desired
// chunks per tick). Oracle bytes: 25.0f = 0x41C80000.
func TestParseChunkBatchReceived(t *testing.T) {
	if v, ok := ParseChunkBatchReceived([]byte{0x41, 0xC8, 0x00, 0x00}); !ok || v != 25.0 {
		t.Fatalf("25.0f: got %v ok=%v", v, ok)
	}
	if v, ok := ParseChunkBatchReceived([]byte{0x00, 0x00, 0x00, 0x00}); !ok || v != 0 {
		t.Fatalf("0f: got %v ok=%v", v, ok)
	}
	if _, ok := ParseChunkBatchReceived([]byte{0x41, 0xC8}); ok {
		t.Fatal("short body must not parse")
	}
	if _, ok := ParseChunkBatchReceived([]byte{0x7F, 0xC0, 0x00, 0x00}); ok {
		t.Fatal("NaN must not parse")
	}
	if _, ok := ParseChunkBatchReceived([]byte{0xC1, 0xC8, 0x00, 0x00}); ok {
		t.Fatal("negative rate must not parse")
	}
}

// TestCreativeSlotPaintingVariant: a creative-menu painting preset's
// painting/variant component is extracted (per-version component ids), and
// non-preset slots degrade to "" (random fit).
func TestCreativeSlotPaintingVariant(t *testing.T) {
	kebab := protocol.PaintingVariantIndex("kebab")
	if kebab < 0 {
		t.Fatal("no kebab in the synced registry")
	}
	compose := func(compID int32) []byte {
		b := []byte{0, 36}                   // slot 36
		b = protocol.AppendVarInt(b, 1)      // count
		b = protocol.AppendVarInt(b, 1213)   // item id (painting)
		b = protocol.AppendVarInt(b, 1)      // components added
		b = protocol.AppendVarInt(b, 0)      // components removed
		b = protocol.AppendVarInt(b, compID) // painting/variant component
		holder := protocol.AppendVarInt(nil, kebab+1)
		b = protocol.AppendVarInt(b, int32(len(holder))) // untrusted codec: length-prefixed value
		b = append(b, holder...)
		return b
	}
	e, ok := ParseCreativeSlot(compose(89), 770)
	if !ok || e.PaintingVariant != "kebab" { // SHORT name — the engine table is unprefixed
		t.Fatalf("770 preset: ok=%v variant=%q", ok, e.PaintingVariant)
	}
	e, ok = ParseCreativeSlot(compose(103), 776)
	if !ok || e.PaintingVariant == "" {
		t.Fatalf("776 preset: ok=%v variant=%q", ok, e.PaintingVariant)
	}
	// the wrong component id for the version → no preset, but still a valid slot
	e, ok = ParseCreativeSlot(compose(89), 776)
	if !ok || e.PaintingVariant != "" {
		t.Fatalf("mismatched component id must degrade: ok=%v variant=%q", ok, e.PaintingVariant)
	}
	// a componentless painting → no preset
	plain := []byte{0, 36}
	plain = protocol.AppendVarInt(plain, 1)
	plain = protocol.AppendVarInt(plain, 1213)
	plain = protocol.AppendVarInt(plain, 0)
	plain = protocol.AppendVarInt(plain, 0)
	if e, ok := ParseCreativeSlot(plain, 776); !ok || e.PaintingVariant != "" {
		t.Fatalf("plain painting: ok=%v variant=%q", ok, e.PaintingVariant)
	}
}

// swing: the hand is one VarInt; anything but 0 or 1 is refused.
func TestParseSwing(t *testing.T) {
	for _, tc := range []struct {
		data []byte
		hand int32
		ok   bool
	}{{[]byte{0}, 0, true}, {[]byte{1}, 1, true}, {[]byte{2}, 0, false}, {nil, 0, false}} {
		got, ok := ParseSwing(tc.data)
		if ok != tc.ok || (ok && got.Hand != tc.hand) {
			t.Errorf("ParseSwing(%v) = %+v, %v; want hand %d, %v", tc.data, got, ok, tc.hand, tc.ok)
		}
	}
}

// animate: entity id then the action, 0 for the main hand and 3 for the off hand.
func TestSwingRendersEachHand(t *testing.T) {
	if p := Swing(attach.Swing{EID: 9}); p.ID != IDSwing || !bytes.Equal(p.Body, []byte{9, 0}) {
		t.Errorf("main hand: %#x % x", p.ID, p.Body)
	}
	if p := Swing(attach.Swing{EID: 9, Hand: 1}); !bytes.Equal(p.Body, []byte{9, 3}) {
		t.Errorf("off hand: % x", p.Body)
	}
}

// pick_item_from_block is a packed position then the ctrl flag;
// pick_item_from_entity a VarInt id then the flag.
func TestParsePickItem(t *testing.T) {
	body := append(protocol.AppendPosition(nil, -5, 70, 1234567), 1)
	got, ok := ParsePickFromBlock(body)
	if !ok || got.Entity || got.X != -5 || got.Y != 70 || got.Z != 1234567 || !got.IncludeData {
		t.Fatalf("ParsePickFromBlock = %+v, %v", got, ok)
	}
	if _, ok := ParsePickFromBlock(body[:8]); ok {
		t.Fatal("a pick without its flag parsed")
	}
	got, ok = ParsePickFromEntity(append(protocol.AppendVarInt(nil, 300), 0))
	if !ok || !got.Entity || got.EID != 300 || got.IncludeData {
		t.Fatalf("ParsePickFromEntity = %+v, %v", got, ok)
	}
	if _, ok := ParsePickFromEntity(protocol.AppendVarInt(nil, 300)); ok {
		t.Fatal("an entity pick without its flag parsed")
	}
}

// paddle_boat is two booleans, left then right.
func TestParsePaddleBoat(t *testing.T) {
	if got, ok := ParsePaddleBoat([]byte{0, 1}); !ok || got.Left || !got.Right {
		t.Fatalf("ParsePaddleBoat(0,1) = %+v, %v", got, ok)
	}
	if _, ok := ParsePaddleBoat([]byte{1}); ok {
		t.Fatal("a one-byte paddle_boat parsed")
	}
}

// stop_sound: flags (1 source, 2 sound), the source varint, the name.
func TestStopSoundRenders(t *testing.T) {
	for _, tc := range []struct {
		e    attach.StopSound
		want []byte
	}{
		{attach.StopSound{Category: -1}, []byte{0}},
		{attach.StopSound{Category: 2}, []byte{1, 2}},
		{attach.StopSound{Category: -1, Name: "a:b"}, []byte{2, 3, 'a', ':', 'b'}},
		{attach.StopSound{Category: 8, Name: "a:b"}, []byte{3, 8, 3, 'a', ':', 'b'}},
	} {
		if p := StopSound(tc.e); p.ID != IDStopSound || !bytes.Equal(p.Body, tc.want) {
			t.Errorf("%+v → %x, want %x", tc.e, p.Body, tc.want)
		}
	}
}
