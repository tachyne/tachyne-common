package render770

import (
	"testing"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// Oracles: the gomc hub's item builders at deletion time (stage 4).

func oracleSlot(b []byte, item int32, count int32, comps []byte) []byte {
	b = protocol.AppendVarInt(b, count)
	if count == 0 {
		return b
	}
	b = protocol.AppendVarInt(b, item)
	if comps == nil {
		b = protocol.AppendVarInt(b, 0)
		return protocol.AppendVarInt(b, 0)
	}
	return append(b, comps...)
}

func TestItemStackEncoding(t *testing.T) {
	if got := AppendItemStack(nil, attach.ItemStack{}); len(got) != 1 || got[0] != 0 {
		t.Fatalf("empty stack: %x", got)
	}
	// Plain stack, no components.
	got := AppendItemStack(nil, attach.ItemStack{ID: 5, Count: 64})
	if want := oracleSlot(nil, 5, 64, nil); string(got) != string(want) {
		t.Fatalf("plain: got %x want %x", got, want)
	}
	// Component bytes pass through verbatim (damage component 3 = 17).
	comps := protocol.AppendVarInt(nil, 1)
	comps = protocol.AppendVarInt(comps, 0)
	comps = protocol.AppendVarInt(comps, 3)
	comps = protocol.AppendVarInt(comps, 17)
	got = AppendItemStack(nil, attach.ItemStack{ID: 800, Count: 1, Components: comps})
	if want := oracleSlot(nil, 800, 1, comps); string(got) != string(want) {
		t.Fatalf("components: got %x want %x", got, want)
	}
}

func TestEquipmentMatchesOracle(t *testing.T) {
	// Oracle: eid, then 6 entries (slot | 0x80 continuation), each a Slot.
	main := attach.ItemStack{ID: 900, Count: 1}
	head := attach.ItemStack{ID: 901, Count: 1}
	var e attach.Equipment
	e.EID = 12
	e.Slots[attach.EquipMainHand] = main
	e.Slots[attach.EquipHead] = head
	want := protocol.AppendVarInt(nil, 12)
	for slot := 0; slot < 7; slot++ {
		marker := byte(slot)
		if slot < 6 {
			marker |= 0x80
		}
		want = append(want, marker)
		switch slot {
		case attach.EquipMainHand:
			want = oracleSlot(want, 900, 1, nil)
		case attach.EquipHead:
			want = oracleSlot(want, 901, 1, nil)
		default:
			want = protocol.AppendVarInt(want, 0)
		}
	}
	eq(t, "equipment", Equipment(e), IDSetEquipment, want)
}

func TestWindowFamilyMatchesOracle(t *testing.T) {
	eq(t, "open", WindowOpen(attach.WindowOpen{ID: 3, Menu: 2, Title: "Chest"}),
		IDOpenWindow, append(protocol.AppendVarInt(protocol.AppendVarInt(nil, 3), 2), oracleChatNBT("Chest")...))

	wi := attach.WindowItems{ID: 3, StateID: 9,
		Slots:  []attach.ItemStack{{ID: 1, Count: 2}, {}},
		Cursor: attach.ItemStack{ID: 4, Count: 1}}
	want := protocol.AppendVarInt(nil, 3)
	want = protocol.AppendVarInt(want, 9)
	want = protocol.AppendVarInt(want, 2)
	want = oracleSlot(want, 1, 2, nil)
	want = oracleSlot(want, 0, 0, nil)
	want = oracleSlot(want, 4, 1, nil)
	eq(t, "items", WindowItems(wi), IDWindowItems, want)

	ws := attach.WindowSlot{ID: 3, StateID: 10, Slot: 40, Item: attach.ItemStack{ID: 7, Count: 3}}
	want = protocol.AppendVarInt(nil, 3)
	want = protocol.AppendVarInt(want, 10)
	want = protocol.AppendI16(want, 40)
	want = oracleSlot(want, 7, 3, nil)
	eq(t, "slot", WindowSlot(ws), IDSetSlot, want)

	want = protocol.AppendVarInt(nil, 3)
	want = protocol.AppendI16(want, 2)
	want = protocol.AppendI16(want, 150)
	eq(t, "data", WindowData(attach.WindowData{ID: 3, Prop: 2, Value: 150}), IDContainerData, want)

	eq(t, "held", HeldSync(attach.HeldSync{Slot: 4}), IDHeldSlot, protocol.AppendVarInt(nil, 4))

	want = protocol.AppendVarInt(nil, 20)
	want = protocol.AppendVarInt(want, 5)
	want = protocol.AppendVarInt(want, 2)
	eq(t, "collect", Collect(attach.Collect{Collected: 20, Collector: 5, Count: 2}), IDCollect, want)
}

func TestEntityMetaEnvelope(t *testing.T) {
	meta := []byte{8, 7, 1, 2, 0xff}
	want := append(protocol.AppendVarInt(nil, 44), meta...)
	eq(t, "meta", EntityMeta(attach.EntityMeta{EID: 44, Meta: meta}), IDEntityMeta, want)
}
