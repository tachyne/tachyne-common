package render770

import (
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
