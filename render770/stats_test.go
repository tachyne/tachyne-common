package render770

// Strict re-parse of award_stats at 770 and through the chain to 776,
// asserting each stat type's key id lands in the right per-version registry.

import (
	"bytes"
	"testing"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

func statsFixture() attach.Stats {
	return attach.Stats{Entries: []attach.StatEntry{
		{T: attach.StatMined, K: 1000, V: 42},   // block-registry key (shifts at 770 and 776)
		{T: attach.StatCrafted, K: 913, V: 3},   // item key
		{T: attach.StatKilled, K: 149, V: 7},    // entity key
		{T: attach.StatCustom, K: 20, V: 12345}, // custom_stat key (shifts at ≤773)
	}}
}

func reparseStats(t *testing.T, body []byte) map[[2]int32]int32 {
	t.Helper()
	r := bytes.NewReader(body)
	n, err := protocol.ReadVarInt(r)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	out := map[[2]int32]int32{}
	for i := int32(0); i < n; i++ {
		typ, e1 := protocol.ReadVarInt(r)
		key, e2 := protocol.ReadVarInt(r)
		val, e3 := protocol.ReadVarInt(r)
		if e1 != nil || e2 != nil || e3 != nil {
			t.Fatalf("entry %d truncated", i)
		}
		out[[2]int32{typ, key}] = val
	}
	if r.Len() != 0 {
		t.Fatalf("%d trailing bytes", r.Len())
	}
	return out
}

func TestAwardStatsReparse770(t *testing.T) {
	pkt := AwardStats(statsFixture())
	if pkt.ID != IDAwardStats {
		t.Fatalf("id 0x%x", pkt.ID)
	}
	tr := protocol.TranslatorFor(770)
	id, body, drop := tr.Clientbound(protocol.StatePlay, pkt.ID, pkt.Body)
	if drop || id != IDAwardStats {
		t.Fatalf("770: id 0x%x drop %v", id, drop)
	}
	got := reparseStats(t, body)
	// keys land in the 770 registries
	wantBlock := protocol.RemapID(protocol.RegBlock, 770, 1000)
	wantCustom := protocol.RemapID(protocol.RegCustomStat, 770, 20)
	if got[[2]int32{attach.StatMined, wantBlock}] != 42 {
		t.Fatalf("mined key not remapped for 770: %v", got)
	}
	if got[[2]int32{attach.StatCustom, wantCustom}] != 12345 {
		t.Fatalf("custom key not remapped for 770: %v", got)
	}
	if wantCustom == 20 {
		t.Fatal("custom_stat 20 should shift at 770 (happy_ghast_one_cm insert)")
	}
}

func TestAwardStatsChainTo776(t *testing.T) {
	tr := protocol.TranslatorFor(776)
	pkt := AwardStats(statsFixture())
	id, body, drop := tr.Clientbound(protocol.StatePlay, pkt.ID, pkt.Body)
	if drop {
		t.Fatal("dropped")
	}
	if id != 0x03 { // award_stats keeps id 3 through the chain (registration head)
		t.Fatalf("776 id 0x%x", id)
	}
	got := reparseStats(t, body)
	wantBlock := protocol.RemapID(protocol.RegBlock, 776, 1000)
	wantItem := protocol.RemapID(protocol.RegItem, 776, 913)
	wantEnt := protocol.RemapID(protocol.RegEntity, 776, 149)
	if got[[2]int32{attach.StatMined, wantBlock}] != 42 ||
		got[[2]int32{attach.StatCrafted, wantItem}] != 3 ||
		got[[2]int32{attach.StatKilled, wantEnt}] != 7 {
		t.Fatalf("776 keys wrong: %v", got)
	}
	if wantBlock == 1000 {
		t.Fatal("block 1200 should shift at 776")
	}
	// custom_stat is append-only after 774 → identity at 776
	if got[[2]int32{attach.StatCustom, 20}] != 12345 {
		t.Fatalf("custom key should be identity at 776: %v", got)
	}
}
