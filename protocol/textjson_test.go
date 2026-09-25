package protocol

import (
	"bytes"
	"testing"
)

// /tellraw's JSON reaches the client as the network NBT the component codec
// reads: a string root is a TAG_String, an object a compound, an array its
// head with the tail as extra; extra/with children are compounds.
func TestTextComponentNBT(t *testing.T) {
	got, ok := TextComponentNBT([]byte(`"hi"`))
	if !ok || !bytes.Equal(got, []byte{nbtString, 0, 2, 'h', 'i'}) {
		t.Fatalf("string root: %x", got)
	}
	got, ok = TextComponentNBT([]byte(`{"text":"a","bold":true}`))
	want := []byte{nbtCompound,
		nbtByte, 0, 4, 'b', 'o', 'l', 'd', 1,
		nbtString, 0, 4, 't', 'e', 'x', 't', 0, 1, 'a',
		nbtEnd}
	if !ok || !bytes.Equal(got, want) {
		t.Fatalf("object: %x, want %x", got, want)
	}
	got, ok = TextComponentNBT([]byte(`["a","b"]`))
	want = []byte{nbtCompound,
		nbtList, 0, 5, 'e', 'x', 't', 'r', 'a', nbtCompound, 0, 0, 0, 1,
		nbtString, 0, 4, 't', 'e', 'x', 't', 0, 1, 'b', nbtEnd,
		nbtString, 0, 4, 't', 'e', 'x', 't', 0, 1, 'a',
		nbtEnd}
	if !ok || !bytes.Equal(got, want) {
		t.Fatalf("array: %x, want %x", got, want)
	}
	// Every shape re-parses as exactly one NBT value.
	for _, in := range []string{
		`{"text":"x","color":"red","extra":["y",{"text":"z","italic":false},3]}`,
		`{"translate":"chat.type.text","with":["a",{"text":"b"}],"shadow_color":-16777216}`,
		`{"text":"click","click_event":{"action":"open_url","url":"https://example.com"},"hover_event":{"action":"show_text","value":{"text":"t"}}}`,
		`"🙂 and \u0000"`,
	} {
		got, ok := TextComponentNBT([]byte(in))
		r := bytes.NewReader(got)
		if !ok || SkipNetworkNBT(r) != nil || r.Len() != 0 {
			t.Errorf("%s → %x does not re-parse", in, got)
		}
	}
	for _, bad := range []string{``, `nope`, `[]`, `{"text":"a"} x`} {
		if _, ok := TextComponentNBT([]byte(bad)); ok {
			t.Errorf("%q was accepted", bad)
		}
	}
}
