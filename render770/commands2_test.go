package render770

import (
	"bytes"
	"testing"

	"github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

func TestCommandPacketsMatchOracle(t *testing.T) {
	eq(t, "transfer", Transfer(attach.Transfer{Host: "example.org", Port: 25565}), IDTransfer,
		protocol.AppendVarInt(protocol.AppendString(nil, "example.org"), 25565))
	eq(t, "camera", Camera(attach.Camera{EID: 9}), IDSetCamera, protocol.AppendVarInt(nil, 9))
	eq(t, "ticking state", TickingState(attach.TickingState{Rate: 40, Frozen: true}), IDTickingState,
		protocol.AppendBool(protocol.AppendF32(nil, 40), true))
	eq(t, "ticking step", TickingStep(attach.TickingStep{Steps: 5}), IDTickingStep, protocol.AppendVarInt(nil, 5))
}

func TestCommandSuggestionsMatchOracle(t *testing.T) {
	w := protocol.AppendVarInt(nil, 4)
	w = protocol.AppendVarInt(w, 10)
	w = protocol.AppendVarInt(w, 2)
	w = protocol.AppendVarInt(w, 1)
	w = protocol.AppendBool(protocol.AppendString(w, "keep_inventory"), false)
	eq(t, "suggestions", CommandSuggestions(attach.Suggestions{ID: 4, Start: 10, Length: 2, Matches: []string{"keep_inventory"}}), IDCommandSuggestions, w)
}

// The vault's update tag: shared_data.display_item as an item compound.
func TestBlockDisplayVault(t *testing.T) {
	p, ok := BlockDisplay(attach.BlockDisplay{Pos: [3]int32{1, 2, 3}, Kind: attach.DisplayVault, Name: "minecraft:diamond", Count: 2})
	if !ok || p.ID != IDBlockEntityData {
		t.Fatal("no packet")
	}
	w := protocol.AppendPosition(nil, 1, 2, 3)
	w = protocol.AppendVarInt(w, 45)
	w = append(w, protocol.NBTRoot()...)
	w = protocol.NBTCompound(w, "shared_data")
	w = protocol.NBTCompound(w, "display_item")
	w = protocol.NBTString(w, "id", "minecraft:diamond")
	w = protocol.NBTEnd(protocol.NBTInt(w, "count", 2))
	w = protocol.NBTEnd(protocol.NBTEnd(w))
	eq(t, "vault", p, IDBlockEntityData, w)
}

// A player head's update tag (SkullBlockEntity.saveCustomOnly): profile in
// ResolvableProfile's map form — name, id as UUIDUtil's four-int array,
// properties as a list of {name, value, signature} — and note_block_sound.
// The profile bytes are written out by hand from the NBT format, not with
// the helpers under test.
func TestBlockDisplaySkull(t *testing.T) {
	uuid := [16]byte{0, 0, 0, 1, 0, 0, 0, 2, 0xff, 0xff, 0xff, 0xfd, 0x7f, 0, 0, 4}
	p, ok := BlockDisplay(attach.BlockDisplay{Pos: [3]int32{1, 2, 3}, Kind: attach.DisplaySkull,
		Name: "minecraft:block.note_block.harp",
		Profile: &attach.GameProfile{Name: "Legion", UUID: uuid,
			Properties: []attach.Property{{Name: "textures", Value: "e30=", Signature: "c2ln"}}}})
	if !ok {
		t.Fatal("no packet")
	}
	str := func(b []byte, s string) []byte { return append(append(b, byte(len(s)>>8), byte(len(s))), s...) }
	w := protocol.AppendPosition(nil, 1, 2, 3)
	w = protocol.AppendVarInt(w, 16) // minecraft:skull
	w = append(w, 10)                // root compound
	w = str(append(w, 10), "profile")
	w = str(str(append(w, 8), "name"), "Legion")
	w = str(append(w, 11), "id")
	w = append(w, 0, 0, 0, 4, 0, 0, 0, 1, 0, 0, 0, 2, 0xff, 0xff, 0xff, 0xfd, 0x7f, 0, 0, 4)
	w = str(append(w, 9), "properties")
	w = append(w, 10, 0, 0, 0, 1) // list of one compound
	w = str(str(append(w, 8), "name"), "textures")
	w = str(str(append(w, 8), "value"), "e30=")
	w = str(str(append(w, 8), "signature"), "c2ln")
	w = append(w, 0) // property compound
	w = append(w, 0) // profile compound
	w = str(str(append(w, 8), "note_block_sound"), "minecraft:block.note_block.harp")
	w = append(w, 0) // root
	eq(t, "skull", p, IDBlockEntityData, w)

	// A head with no owner: an empty tag.
	p, _ = BlockDisplay(attach.BlockDisplay{Pos: [3]int32{1, 2, 3}, Kind: attach.DisplaySkull})
	eq(t, "bare skull", p, IDBlockEntityData, append(protocol.AppendVarInt(protocol.AppendPosition(nil, 1, 2, 3), 16), 10, 0))
}

// chunks_biomes: VarInt count; per chunk ChunkPos.pack (x low, z high) as a
// long, then a VarInt-length byte array of one single-valued biome
// container per section (bits 0, VarInt id). The packet keeps id 13 through
// the 26.3 chain.
func TestChunksBiomes(t *testing.T) {
	ids := map[string]int32{"minecraft:plains": 1, "minecraft:desert": 7}
	p := ChunksBiomes(attach.ChunksBiomes{Chunks: []attach.ChunkBiomes{
		{CX: -2, CZ: 3, Biomes: []string{"minecraft:desert", "minecraft:plains"}},
	}}, func(n string) int32 { return ids[n] })
	w := []byte{1}                                    // one chunk
	w = append(w, 0, 0, 0, 3, 0xff, 0xff, 0xff, 0xfe) // z=3 high, x=-2 low
	w = append(w, 4, 0, 7, 0, 1)                      // 4 bytes: desert, plains
	eq(t, "chunks_biomes", p, IDChunksBiomes, w)
	id, body, drop := protocol.TranslatorFor(777).Clientbound(protocol.StatePlay, p.ID, p.Body)
	if drop || id != 13 || !bytes.Equal(body, w) {
		t.Errorf("26.3: id %d drop=%v %x", id, drop, body)
	}
}
