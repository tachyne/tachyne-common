package protocol

import (
	"bytes"
	"testing"
)

// 26.3 reads the light masks as byte arrays: each long becomes its eight
// little-endian bytes, in both the light update and the chunk packet.
func TestLightBitSets777(t *testing.T) {
	light := append(AppendVarInt(nil, 1), 0, 0, 0, 0, 0, 0, 0x01, 0x02) // sky mask: one long 0x0102
	light = append(light, 0)                                            // block mask: empty
	light = append(light, 0)                                            // empty sky
	light = append(light, 0)                                            // empty block
	light = append(light, 1, 2, 0xaa, 0xbb)                             // one sky layer of two bytes (test-sized)
	light = append(light, 0)                                            // no block layers
	body := append([]byte{0, 0, 0, 5, 0, 0, 0, 7}, light...)
	got := rewriteLightUpdateBitSets777(StatePlay, body)
	want := append([]byte{0, 0, 0, 5, 0, 0, 0, 7}, 8, 0x02, 0x01, 0, 0, 0, 0, 0, 0)
	want = append(want, 0, 0, 0, 1, 2, 0xaa, 0xbb, 0)
	if !bytes.Equal(got, want) {
		t.Fatalf("light update:\n got %x\nwant %x", got, want)
	}
	chunk := []byte{0, 0, 0, 5, 0, 0, 0, 7}
	chunk = append(chunk, 1, 4, 1, 9, 9, 9, 9, 9, 9, 9, 9) // one heightmap: type 4, one long
	chunk = append(chunk, 3, 0xde, 0xad, 0xbe)             // section buffer of three bytes
	chunk = append(chunk, 1, 0x12, 0, 64, 7, 0)            // one block entity: xz, y=64, type 7, no tag
	chunk = append(chunk, light...)
	got = rewriteChunkLightBitSets777(StatePlay, chunk)
	want = append(append([]byte(nil), chunk[:len(chunk)-len(light)]...), want[8:]...)
	if !bytes.Equal(got, want) {
		t.Fatalf("chunk light:\n got %x\nwant %x", got, want)
	}
	tr := TranslatorFor(777)
	id, _, drop := tr.Clientbound(StatePlay, canonChunkData, chunk)
	if drop || id != 46 {
		t.Fatalf("chunk packet id at 26.3 = %d (want 46)", id)
	}
}
