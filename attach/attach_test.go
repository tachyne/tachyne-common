package attach

import (
	"bytes"
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteJSON(&buf, MsgHello, Hello{Token: "t", Name: "wesley", Gateway: "gw-java-770/0"}); err != nil {
		t.Fatal(err)
	}
	typ, payload, err := ReadFrame(&buf)
	if err != nil || typ != MsgHello {
		t.Fatalf("typ=%#x err=%v", typ, err)
	}
	if !bytes.Contains(payload, []byte("wesley")) {
		t.Fatalf("payload: %s", payload)
	}
}

func TestFrameLengthGuard(t *testing.T) {
	// length claims 32MB
	if _, _, err := ReadFrame(bytes.NewReader([]byte{0x02, 0x00, 0x00, 0x00, 0x01})); err == nil {
		t.Fatal("oversized frame must be rejected")
	}
}

func TestChunkRoundTrip(t *testing.T) {
	body := &ChunkBody{
		BlockStates: make([]uint32, BlocksPerCh),
		Heightmap:   make([]int16, 256),
		SkyLight:    make([]uint8, BlocksPerCh),
		BlockLight:  make([]uint8, BlocksPerCh),
	}
	body.BlockStates[0] = 85       // bedrock at the floor corner
	body.BlockStates[4096*5] = 9   // grass somewhere
	body.Heightmap[7] = 71         // absolute Y
	body.Heightmap[8] = -12        // below sea in a carved column
	body.SkyLight[4096*23] = 15    // open sky at the top
	body.BlockLight[4096*2+9] = 14 // a torch in a cave

	biomes := make([]string, Sections)
	for i := range biomes {
		biomes[i] = "minecraft:plains"
	}
	payload, err := EncodeChunk(ChunkHeader{CX: -3, CZ: 12, Biomes: biomes}, body)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) > 64<<10 {
		t.Errorf("near-empty chunk should compress well, got %d bytes", len(payload))
	}

	h, got, err := DecodeChunk(payload)
	if err != nil {
		t.Fatal(err)
	}
	if h.CX != -3 || h.CZ != 12 || len(h.Biomes) != Sections || h.Biomes[0] != "minecraft:plains" {
		t.Fatalf("header: %+v", h)
	}
	if got.BlockStates[0] != 85 || got.BlockStates[4096*5] != 9 {
		t.Error("block states corrupted")
	}
	if got.Heightmap[7] != 71 || got.Heightmap[8] != -12 {
		t.Error("heightmap corrupted (negative values must survive)")
	}
	if got.SkyLight[4096*23] != 15 || got.BlockLight[4096*2+9] != 14 {
		t.Error("light corrupted")
	}
}

func TestChunkDimensionGuard(t *testing.T) {
	if _, err := EncodeChunk(ChunkHeader{}, &ChunkBody{BlockStates: make([]uint32, 7)}); err == nil {
		t.Fatal("bad dimensions must be rejected")
	}
}
