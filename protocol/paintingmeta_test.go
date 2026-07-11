package protocol

import (
	"bytes"
	"testing"
)

func TestPaintingVariantIndex(t *testing.T) {
	if PaintingVariantIndex("kebab") < 0 {
		t.Fatal("kebab missing from the synced registry")
	}
	if PaintingVariantIndex("minecraft:kebab") != PaintingVariantIndex("kebab") {
		t.Fatal("namespaced lookup differs")
	}
	if PaintingVariantIndex("dennis") < 0 {
		t.Fatal("the 26.x-appended dennis must be present (appended, ids stable)")
	}
	if PaintingVariantIndex("nope") != -1 {
		t.Fatal("unknown variant should be -1")
	}
}

func TestFixPaintingMeta(t *testing.T) {
	body := AppendVarInt(nil, 1234)
	body = append(body, 8)
	body = AppendVarInt(body, PaintingVariantSerializer770)
	body = AppendVarInt(body, 7)
	body = append(body, 0xff)

	// 770-772: untouched
	if got := FixPaintingMeta(770, body); !bytes.Equal(got, body) {
		t.Fatal("770 body changed")
	}
	// 776: index 8 -> 9 (26.x HangingEntity gained a synched DIRECTION at 8)
	// and serializer 30 -> 34; the holder value is untouched
	got := FixPaintingMeta(776, body)
	want := AppendVarInt(nil, 1234)
	want = append(want, 9)
	want = AppendVarInt(want, 34)
	want = AppendVarInt(want, 7)
	want = append(want, 0xff)
	if !bytes.Equal(got, want) {
		t.Fatalf("776 rewrite: %v want %v", got, want)
	}
	// malformed input falls back to the original
	if got := FixPaintingMeta(776, body[:3]); !bytes.Equal(got, body[:3]) {
		t.Fatal("truncated body not passed through")
	}
}
