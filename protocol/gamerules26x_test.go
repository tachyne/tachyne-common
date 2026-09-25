package protocol

import (
	"bytes"
	"testing"
)

// The editor's Done button sends every changed rule in one packet; each
// becomes its own /gamerule.
func TestSetGameRuleBecomesCommands(t *testing.T) {
	b := AppendVarInt(nil, 2)
	b = AppendString(b, "minecraft:keep_inventory")
	b = AppendString(b, "true")
	b = AppendString(b, "minecraft:random_tick_speed")
	b = AppendString(b, "5")
	for _, v := range []int32{776, 777} {
		cmds, ok := SetGameRuleCommands(v, setGameRuleID(v), b)
		if !ok || len(cmds) != 2 || cmds[0] != "gamerule keep_inventory true" || cmds[1] != "gamerule random_tick_speed 5" {
			t.Fatalf("v%d: %v %v", v, cmds, ok)
		}
	}
	if _, ok := SetGameRuleCommands(777, 0x39, b); ok {
		t.Fatal("0x39 is not set_game_rule on 26.3")
	}
	// The chain drops set_game_rule (canonical 770 has none), so the gateway
	// must catch it first.
	if _, _, drop := chainFor(777).Serverbound(StatePlay, 0x3a, b); !drop {
		t.Fatal("expected the chain to drop set_game_rule")
	}
}

func TestGameRuleValuesBody(t *testing.T) {
	got := GameRuleValues(map[string]string{"keep_inventory": "false"})
	want := AppendVarInt(nil, 1)
	want = AppendString(want, "minecraft:keep_inventory")
	want = AppendString(want, "false")
	if !bytes.Equal(got, want) {
		t.Fatalf("got %x want %x", got, want)
	}
}
