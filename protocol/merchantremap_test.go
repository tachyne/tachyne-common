package protocol

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

// offersBody builds a canonical merchant_offers body with one offer: 20
// emeralds + 1 book → an enchanted book carrying Sharpness (enchantments
// component 10 in canonical numbering), plus the trailing villager fields.
func offersBody(emerald, book, enchantedBook int32) []byte {
	b := AppendVarInt(nil, 5) // container
	b = AppendVarInt(b, 1)    // one offer
	b = AppendVarInt(b, emerald)
	b = AppendVarInt(b, 20)
	b = AppendVarInt(b, 0) // no component predicates
	// result: enchanted book with one component (enchantments: 1 × (id 32, lvl 3))
	b = AppendVarInt(b, 1)
	b = AppendVarInt(b, enchantedBook)
	b = AppendVarInt(b, 1)
	b = AppendVarInt(b, 0)
	b = AppendVarInt(b, componentStoredEnch)
	b = AppendVarInt(b, 1)
	b = AppendVarInt(b, 32)
	b = AppendVarInt(b, 3)
	b = append(b, 1) // has cost B
	b = AppendVarInt(b, book)
	b = AppendVarInt(b, 1)
	b = AppendVarInt(b, 0)
	b = append(b, 0) // outOfStock
	b = binary.BigEndian.AppendUint32(b, 0)
	b = binary.BigEndian.AppendUint32(b, 12)
	b = binary.BigEndian.AppendUint32(b, 5)
	b = binary.BigEndian.AppendUint32(b, 0)
	b = binary.BigEndian.AppendUint32(b, math.Float32bits(0.05))
	b = binary.BigEndian.AppendUint32(b, 0)
	b = AppendVarInt(b, 2) // villager level
	b = AppendVarInt(b, 30)
	b = append(b, 1, 1)
	return b
}

// For a 26.2 client the offer's item ids take 26.2 numbering, the result's
// stored_enchantments component takes its 26.2 id, and everything else —
// counts, the second cost, the fixed tail, the villager fields — survives.
func TestMerchantOffersTranslateItemsAndComponents(t *testing.T) {
	const emerald, book, ebook = 100, 200, 300 // any ids: the remap is a lookup we can check
	body := offersBody(emerald, book, ebook)
	out := remapMerchantOffers(776, body)
	if bytes.Equal(out, body) {
		t.Fatal("the offers body was returned untouched")
	}
	r := bytes.NewReader(out)
	ReadVarInt(r) // container
	if n, _ := ReadVarInt(r); n != 1 {
		t.Fatalf("offer count %d", n)
	}
	if it, _ := ReadVarInt(r); it != RemapID(RegItem, 776, emerald) {
		t.Errorf("cost A item %d, want %d", it, RemapID(RegItem, 776, emerald))
	}
	if c, _ := ReadVarInt(r); c != 20 {
		t.Errorf("cost A count %d", c)
	}
	ReadVarInt(r) // predicates
	ReadVarInt(r) // result count
	if it, _ := ReadVarInt(r); it != RemapID(RegItem, 776, ebook) {
		t.Errorf("result item %d, want %d", it, RemapID(RegItem, 776, ebook))
	}
	ReadVarInt(r) // add count
	ReadVarInt(r) // remove count
	if cid, _ := ReadVarInt(r); cid != storedEnchCompID(776) {
		t.Errorf("result component id %d, want stored_enchantments at 26.2 = %d", cid, storedEnchCompID(776))
	}
	ReadVarInt(r) // ench count
	ReadVarInt(r) // ench id
	ReadVarInt(r) // ench lvl
	if hb, _ := r.ReadByte(); hb != 1 {
		t.Fatal("second cost flag lost")
	}
	if it, _ := ReadVarInt(r); it != RemapID(RegItem, 776, book) {
		t.Errorf("cost B item %d, want %d", it, RemapID(RegItem, 776, book))
	}
	ReadVarInt(r)
	ReadVarInt(r)
	tail := make([]byte, r.Len())
	r.Read(tail)
	wantTail := body[len(body)-len(tail):]
	if !bytes.Equal(tail, wantTail) {
		t.Errorf("fixed tail changed: %x vs %x", tail, wantTail)
	}
	// A canonical client gets the body untouched through the dispatcher.
	if got := remapClientboundIDs(770, canonMerchantOffers, body); !bytes.Equal(got, body) {
		t.Error("770 must not rewrite merchant offers")
	}
}
