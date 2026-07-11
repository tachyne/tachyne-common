package render770

// advancements.go renders the advancement tree + per-player progress
// (attach.AdvTree / attach.AdvProgress) as clientbound update_advancements.
// Wire shape per the vanilla 1.21.5 ClientboundUpdateAdvancementsPacket:
// reset bool, added [](id RL, parent opt RL, display opt, requirements
// [][]string, telemetry bool), removed []RL, progress map(RL → map(criterion →
// nullable obtained-millis)), showAdvancements bool. DisplayInfo: title +
// description (network-NBT text components), icon Slot, frame VarInt enum
// (task 0 / challenge 1 / goal 2), flags i32 (1 bg, 2 toast, 4 hidden),
// optional background RL, layout x/y f32.

import (
	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// IDUpdateAdvancements is the canonical-770 clientbound update_advancements id.
const IDUpdateAdvancements = 0x7b

// translateNBT encodes {"translate": key} as a nameless network-NBT compound.
// A key with no client translation renders as its literal text, so literal
// titles (future custom advancements) ride the same field.
func translateNBT(key string) []byte {
	b := protocol.NBTRoot()
	b = protocol.NBTString(b, "translate", key)
	return protocol.NBTEnd(b)
}

func appendAdvNode(b []byte, n attach.AdvNode) []byte {
	b = protocol.AppendString(b, n.ID)
	b = protocol.AppendBool(b, n.Parent != "")
	if n.Parent != "" {
		b = protocol.AppendString(b, n.Parent)
	}
	b = protocol.AppendBool(b, n.HasDisplay)
	if n.HasDisplay {
		b = append(b, translateNBT(n.Title)...)
		b = append(b, translateNBT(n.Desc)...)
		icon := n.Icon
		if icon.Count == 0 { // the client rejects an empty icon slot
			icon.Count = 1
		}
		icon.Components = nil // icons are plain items; the chain walker relies on 0/0
		b = AppendItemStack(b, icon)
		b = protocol.AppendVarInt(b, n.Frame)
		var flags int32 = 0
		if n.Background != "" {
			flags |= 1
		}
		if n.ShowToast {
			flags |= 2
		}
		if n.Hidden {
			flags |= 4
		}
		b = protocol.AppendI32(b, flags)
		if n.Background != "" {
			b = protocol.AppendString(b, n.Background)
		}
		b = protocol.AppendF32(b, n.X)
		b = protocol.AppendF32(b, n.Y)
	}
	b = protocol.AppendVarInt(b, int32(len(n.Reqs)))
	for _, group := range n.Reqs {
		b = protocol.AppendVarInt(b, int32(len(group)))
		for _, c := range group {
			b = protocol.AppendString(b, c)
		}
	}
	// sends_telemetry_event: always false — tachyne feeds no client telemetry.
	return protocol.AppendBool(b, false)
}

// appendProgress writes the progress map. Every criterion an advancement
// requires is listed (vanilla behavior); ones absent from Done are written as
// not-yet-obtained (null instant). reqs supplies each advancement's criteria
// in stable (requirements) order.
func appendProgress(b []byte, reqs map[string][]string, p attach.AdvProgress) []byte {
	b = protocol.AppendVarInt(b, int32(len(p.Entries)))
	for _, e := range p.Entries {
		b = protocol.AppendString(b, e.ID)
		crits := reqs[e.ID]
		b = protocol.AppendVarInt(b, int32(len(crits)))
		for _, c := range crits {
			b = protocol.AppendString(b, c)
			if ms, ok := e.Done[c]; ok {
				b = protocol.AppendBool(b, true)
				b = protocol.AppendI64(b, ms)
			} else {
				b = protocol.AppendBool(b, false)
			}
		}
	}
	return b
}

// ReqIndex derives each advancement's criteria list (the ordered union of its
// requirement names) from the tree. The gateway builds it once per session
// from the MsgAdvTree frame and reuses it for every progress packet.
func ReqIndex(t attach.AdvTree) map[string][]string {
	idx := make(map[string][]string, len(t.Nodes))
	for _, n := range t.Nodes {
		var crits []string
		seen := map[string]bool{}
		for _, group := range n.Reqs {
			for _, c := range group {
				if !seen[c] {
					seen[c] = true
					crits = append(crits, c)
				}
			}
		}
		idx[n.ID] = crits
	}
	return idx
}

// AdvancementsInit renders the join-time packet: reset + the whole tree + the
// player's full progress snapshot.
func AdvancementsInit(t attach.AdvTree, p attach.AdvProgress) Packet {
	b := protocol.AppendBool(nil, true) // reset
	b = protocol.AppendVarInt(b, int32(len(t.Nodes)))
	for _, n := range t.Nodes {
		b = appendAdvNode(b, n)
	}
	b = protocol.AppendVarInt(b, 0) // removed
	b = appendProgress(b, ReqIndex(t), p)
	b = protocol.AppendBool(b, true) // showAdvancements
	return Packet{IDUpdateAdvancements, b}
}

// AdvancementsUpdate renders an incremental progress packet (grants after
// join). reqs is the session's ReqIndex.
func AdvancementsUpdate(reqs map[string][]string, p attach.AdvProgress) Packet {
	b := protocol.AppendBool(nil, false) // no reset
	b = protocol.AppendVarInt(b, 0)      // added
	b = protocol.AppendVarInt(b, 0)      // removed
	b = appendProgress(b, reqs, p)
	b = protocol.AppendBool(b, true)
	return Packet{IDUpdateAdvancements, b}
}
