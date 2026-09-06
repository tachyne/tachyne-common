package protocol

import (
	"bytes"
	"testing"
)

// Every holder-variant species' entry (its own index, its own serializer)
// reaches a 26.2 client one index up under the renumbered serializer with
// the registry id intact; a canonical client gets the body untouched. The
// 770 ids follow the 1.21.5 serializer registration order (CAT 22, COW 23,
// WOLF 24, FROG 26, PIG 27, CHICKEN 28); the 26.2 ids the 26.2 order (CAT
// 21, COW 23, WOLF 25, FROG 27, PIG 28, CHICKEN 30).
func TestVariantHolderMetaRenumbersFor26x(t *testing.T) {
	cases := []struct {
		name         string
		idx          byte
		typ770, want int32
	}{
		{"cat", 19, CatVariantSerializer770, 21},
		{"cow", 17, CowVariantSerializer770, 23},
		{"wolf", 22, WolfVariantSerializer770, 25},
		{"frog", 17, FrogVariantSerializer770, 27},
		{"pig", 18, PigVariantSerializer770, 28},
		{"chicken", 17, ChickenVariantSerializer770, 30},
	}
	for _, c := range cases {
		body := AppendVarInt(nil, 42) // eid
		body = append(body, c.idx)
		body = AppendVarInt(body, c.typ770)
		body = AppendVarInt(body, 5) // a registry id, carried verbatim
		body = append(body, 0xff)

		if got := FixVariantMeta(770, body); !bytes.Equal(got, body) {
			t.Errorf("%s: 770 must be untouched", c.name)
		}
		if got := remapEntityMeta(770, body); !bytes.Equal(got, body) {
			t.Errorf("%s: the 770 translator must pass the body through", c.name)
		}
		got := FixVariantMeta(776, body)
		want := AppendVarInt(nil, 42)
		want = append(want, c.idx+1)
		want = AppendVarInt(want, c.want)
		want = AppendVarInt(want, 5)
		want = append(want, 0xff)
		if !bytes.Equal(got, want) {
			t.Errorf("%s: 776 body %x, want %x", c.name, got, want)
		}
	}
	if CatVariantSerializer770 != 22 || CowVariantSerializer770 != 23 || WolfVariantSerializer770 != 24 ||
		FrogVariantSerializer770 != 26 || PigVariantSerializer770 != 27 || ChickenVariantSerializer770 != 28 {
		t.Error("canonical holder serializer ids must follow the 1.21.5 registration order")
	}
}

// A llama's two INT entries (strength 19, variant 20) and a horse's INT
// variant (18) only move index on 26.2; the values and INT type stay.
func TestIntVariantMetaShiftsFor26x(t *testing.T) {
	body := AppendVarInt(nil, 9)
	body = append(body, 19)
	body = AppendVarInt(body, metaTypeVarInt)
	body = AppendVarInt(body, 3) // strength
	body = append(body, 20)
	body = AppendVarInt(body, metaTypeVarInt)
	body = AppendVarInt(body, 2) // brown
	body = append(body, 0xff)
	got := ShiftAgeableMobMeta(776, body)
	want := AppendVarInt(nil, 9)
	want = append(want, 20)
	want = AppendVarInt(want, metaTypeVarInt)
	want = AppendVarInt(want, 3)
	want = append(want, 21)
	want = AppendVarInt(want, metaTypeVarInt)
	want = AppendVarInt(want, 2)
	want = append(want, 0xff)
	if !bytes.Equal(got, want) {
		t.Fatalf("llama 776 body %x, want %x", got, want)
	}
	horse := AppendVarInt(nil, 10)
	horse = append(horse, 18)
	horse = AppendVarInt(horse, metaTypeVarInt)
	horse = AppendVarInt(horse, 4|(3<<8)) // black with white dots
	horse = append(horse, 0xff)
	if got := ShiftAgeableMobMeta(776, horse); got[len(AppendVarInt(nil, 10))] != 19 || !bytes.Equal(got[2:], horse[2:]) {
		t.Fatalf("horse variant should move to index 19 keeping the packed value: %x", got)
	}
	if got := ShiftAgeableMobMeta(770, horse); !bytes.Equal(got, horse) {
		t.Fatal("770 must be untouched")
	}
}
