package protocol

import (
	"bufio"
	"bytes"
	"testing"
)

func TestCompressRoundTrip(t *testing.T) {
	cases := []struct {
		id   int32
		data []byte
	}{
		{0x27, bytes.Repeat([]byte{0xFF}, 5000)}, // large+repetitive -> compressed
		{0x26, []byte{1, 2, 3}},                  // tiny -> below threshold, raw
		{0x00, nil},                              // empty
	}
	for _, c := range cases {
		var buf bytes.Buffer
		if err := WriteCompressed(&buf, c.id, c.data, 256); err != nil {
			t.Fatal(err)
		}
		pk, err := ReadCompressed(bufio.NewReader(&buf))
		if err != nil {
			t.Fatalf("id 0x%x: %v", c.id, err)
		}
		if pk.ID != c.id || !bytes.Equal(pk.Data, c.data) {
			t.Fatalf("id 0x%x: round-trip mismatch (got id 0x%x, %d bytes)", c.id, pk.ID, len(pk.Data))
		}
	}
	// confirm the large packet actually shrank on the wire
	var buf bytes.Buffer
	WriteCompressed(&buf, 0x27, bytes.Repeat([]byte{0xFF}, 5000), 256)
	if buf.Len() >= 5000 {
		t.Errorf("compressed frame %d bytes, expected << 5000", buf.Len())
	}
	t.Logf("5000-byte packet -> %d-byte compressed frame", buf.Len())
}
