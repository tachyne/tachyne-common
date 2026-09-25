package protocol

import (
	"bytes"
	"testing"
)

// The 26.3 layout, field by field, for a large blast with a knockback.
func TestExplode26xLayout(t *testing.T) {
	k := [3]float64{0.5, 0.25, -1}
	got := Explode26x(777, 1, 2, 3, 4, 12, &k, true, true)
	w := AppendF64(AppendF64(AppendF64(nil, 1), 2), 3)
	w = AppendF32(w, 4)
	w = AppendI32(w, 12)
	w = AppendBool(w, true)
	w = AppendF64(AppendF64(AppendF64(w, 0.5), 0.25), -1)
	w = AppendVarInt(w, 29) // explosion_emitter on 26.3
	w = AppendVarInt(w, 0)
	w = AppendString(w, "minecraft:entity.generic.explode")
	w = AppendBool(w, false)
	w = AppendVarInt(w, 2)
	w = AppendF32(AppendF32(AppendVarInt(w, 69), 0.5), 1) // poof
	w = AppendVarInt(w, 1)
	w = AppendF32(AppendF32(AppendVarInt(w, 72), 1), 1) // smoke
	w = AppendVarInt(w, 1)
	w = AppendBool(w, true)
	if !bytes.Equal(got, w) {
		t.Fatalf("26.3 explode body\n got %x\nwant %x", got, w)
	}
	// 26.2 has no play-sound flag.
	if g := Explode26x(776, 1, 2, 3, 4, 12, &k, true, true); len(g) != len(w)-1 {
		t.Fatalf("26.2 body is %d bytes, want %d", len(g), len(w)-1)
	}
}
