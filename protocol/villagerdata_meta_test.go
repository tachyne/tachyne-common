package protocol

import (
	"bytes"
	"testing"
)

// A villager's VILLAGER_DATA entry (index 18, serializer 20: type,
// profession, level) reaches a 26.2 client as serializer 19 at index 19
// (the ageable shift), payload intact; a 770 client gets it untouched.
func TestVillagerDataMetaTranslation(t *testing.T) {
	body := AppendVarInt(nil, 7)
	body = append(body, 18)
	body = AppendVarInt(body, VillagerDataSerializer770)
	body = AppendVarInt(body, 2) // plains
	body = AppendVarInt(body, 9) // librarian
	body = AppendVarInt(body, 3) // tier
	body = append(body, 0xff)

	if got := remapEntityMeta(770, body); !bytes.Equal(got, body) {
		t.Errorf("770 must pass the body through: %x vs %x", got, body)
	}
	want := AppendVarInt(nil, 7)
	want = append(want, 18)
	want = AppendVarInt(want, 19)
	want = AppendVarInt(want, 2)
	want = AppendVarInt(want, 9)
	want = AppendVarInt(want, 3)
	want = append(want, 0xff)
	if got := remapEntityMeta(776, body); !bytes.Equal(got, want) {
		t.Errorf("776 serializer renumber: got %x want %x", got, want)
	}
	shifted := ShiftAgeableMobMeta(776, body)
	if len(shifted) < 2 || shifted[1] != 19 {
		t.Errorf("ageable shift should move index 18 to 19: %x", shifted)
	}
	if got := remapEntityMeta(776, shifted); got[1] != 19 || !bytes.Equal(got[2:], want[2:]) {
		t.Errorf("shift then renumber: got %x", got)
	}
}
