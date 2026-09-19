package protocol

import (
	"bytes"
	"testing"
)

// 26.3 wraps each added advancement in a PositionedAdvancement: the x/y
// floats leave the display info and follow the entry; no display → (0, 0).
func TestAdvancementsPositioned777(t *testing.T) {
	str := func(s string) []byte { return AppendString(nil, s) }
	nbtStr := func(s string) []byte { // network NBT: TAG_String + u16 length + bytes
		b := []byte{8, byte(len(s) >> 8), byte(len(s))}
		return append(b, s...)
	}
	// entry 1: a display with a background and position (1.5, 2.5); built as
	// the part before the position, the position, and the part after it.
	pre := str("minecraft:story/root")
	pre = append(pre, 0)              // no parent
	pre = append(pre, 1)              // display present
	pre = append(pre, nbtStr("t")...) // title
	pre = append(pre, nbtStr("d")...) // description
	pre = append(pre, 5, 1, 0, 0)     // icon template: item 5, count 1, no components
	pre = append(pre, 0)              // frame task
	pre = append(pre, 0, 0, 0, 3)     // flags: background | toast
	pre = append(pre, str("minecraft:bg")...)
	xy := []byte{0x3f, 0xc0, 0, 0, 0x40, 0x20, 0, 0} // 1.5, 2.5
	post := []byte{1, 1}                             // one requirement group of one criterion
	post = append(post, str("c")...)
	post = append(post, 0) // telemetry
	e1 := append(append(append([]byte(nil), pre...), xy...), post...)
	// entry 2: no display
	e2 := str("minecraft:story/hidden")
	e2 = append(e2, 1)
	e2 = append(e2, str("minecraft:story/root")...)
	e2 = append(e2, 0)         // no display
	e2 = append(e2, 0)         // no requirements
	e2 = append(e2, 1)         // telemetry
	trailer := []byte{0, 0, 1} // no removed, no progress, show
	body := append([]byte{1}, AppendVarInt(nil, 2)...)
	body = append(body, e1...)
	body = append(body, e2...)
	body = append(body, trailer...)

	got := rewriteAdvancementsPositioned777(StatePlay, body)
	// e1 without its x/y, then x/y; e2 then zeros; trailer.
	want := append([]byte{1}, AppendVarInt(nil, 2)...)
	want = append(want, pre...)
	want = append(want, post...)
	want = append(want, xy...)
	want = append(want, e2...)
	want = append(want, 0, 0, 0, 0, 0, 0, 0, 0)
	want = append(want, trailer...)
	if !bytes.Equal(got, want) {
		t.Fatalf("positioned advancements:\n got %x\nwant %x", got, want)
	}
	// Through the chain: the packet keeps its 26.3 id and the shape above.
	tr := TranslatorFor(777)
	id, out, drop := tr.Clientbound(StatePlay, canonUpdateAdvancements, body)
	if drop || id != 133 || len(out) != len(want) {
		t.Fatalf("chain: id %d (want 133) len %d (want %d) drop %v", id, len(out), len(want), drop)
	}
}
