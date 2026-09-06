package protocol

import (
	"bytes"
	"testing"
)

// A frog's variant entry (index 17, FROG_VARIANT holder) reaches a 26.2
// client at index 18 under serializer 27 with the same holder id; a
// canonical client gets the body untouched; an axolotl's INT variant only
// moves index.
func TestFrogVariantMetaRenumbersFor26x(t *testing.T) {
	body := AppendVarInt(nil, 42) // eid
	body = append(body, 17)
	body = AppendVarInt(body, FrogVariantSerializer770)
	body = AppendVarInt(body, 2) // warm, in the declared frog_variant order
	body = append(body, 0xff)

	if got := FixFrogMeta(770, body); !bytes.Equal(got, body) {
		t.Fatal("770 must be untouched")
	}
	got := FixFrogMeta(776, body)
	want := AppendVarInt(nil, 42)
	want = append(want, 18)
	want = AppendVarInt(want, frogVariantSerializer776)
	want = AppendVarInt(want, 2)
	want = append(want, 0xff)
	if !bytes.Equal(got, want) {
		t.Fatalf("776 body %x, want %x", got, want)
	}

	ax := AppendVarInt(nil, 43)
	ax = append(ax, 17)
	ax = AppendVarInt(ax, metaTypeVarInt)
	ax = AppendVarInt(ax, 4) // blue
	ax = append(ax, 0xff)
	got = ShiftAgeableMobMeta(776, ax)
	if got[len(AppendVarInt(nil, 43))] != 18 || !bytes.Equal(got[len(got)-3:], ax[len(ax)-3:]) {
		t.Fatalf("axolotl variant should move to index 18 keeping INT and value: %x", got)
	}
}
