package render770

// stats.go renders the statistics snapshot (attach.Stats) as clientbound
// award_stats: VarInt count × (VarInt statType, VarInt keyId, VarInt value) —
// the vanilla ClientboundAwardStatsPacket map codec, identical at 770 and
// 776. Key-id registry translation per client version happens in the chain
// (protocol.remapAwardStats).

import (
	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// IDAwardStats is the canonical-770 clientbound award_stats id.
const IDAwardStats = 0x03

func AwardStats(e attach.Stats) Packet {
	b := protocol.AppendVarInt(nil, int32(len(e.Entries)))
	for _, s := range e.Entries {
		b = protocol.AppendVarInt(b, s.T)
		b = protocol.AppendVarInt(b, s.K)
		b = protocol.AppendVarInt(b, s.V)
	}
	return Packet{IDAwardStats, b}
}
