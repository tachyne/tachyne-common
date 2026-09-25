package protocol

import (
	"bytes"
	"sort"
	"strings"
)

// The 26.x in-game gamerule editor (26.1+). Canonical 770 has no packets for
// it, so the gateway handles both ends at the client's own version: the
// client's edits (set_game_rule) become the /gamerule commands they stand
// for — the op check is then the command's own, as it is for F3+F4 — and the
// values the editor asks for (client_command REQUEST_GAMERULE_VALUES) go back
// as game_rule_values.

// Packet ids at each 26.x version. 26.2 reuses 26.1's ids.
func setGameRuleID(version int32) int32 {
	if version >= 777 {
		return 0x3a
	}
	return 0x39
}

// GameRuleValuesID is clientbound game_rule_values at a 26.x version.
func GameRuleValuesID(version int32) int32 {
	if version >= 777 {
		return 0x28
	}
	return 0x27
}

// ClientCommandRequestGameRules is client_command's REQUEST_GAMERULE_VALUES.
const ClientCommandRequestGameRules = 2

// SetGameRuleCommands reads a 26.x set_game_rule (still at the client's
// version and id) into one /gamerule command per entry; ok false when the
// packet is not set_game_rule.
func SetGameRuleCommands(version, id int32, body []byte) ([]string, bool) {
	if version < 775 || id != setGameRuleID(version) {
		return nil, false
	}
	r := bytes.NewReader(body)
	n, err := ReadVarInt(r)
	if err != nil || n < 0 || n > 256 {
		return nil, true
	}
	var cmds []string
	for i := int32(0); i < n; i++ {
		key, err1 := ReadString(r)
		val, err2 := ReadString(r)
		if err1 != nil || err2 != nil {
			return cmds, true
		}
		name := strings.TrimPrefix(key, "minecraft:")
		if name == "" || strings.ContainsAny(name, " \n") || strings.ContainsAny(val, " \n") {
			continue
		}
		cmds = append(cmds, "gamerule "+name+" "+val)
	}
	return cmds, true
}

// GameRuleValues renders game_rule_values' body: rule key → value text.
func GameRuleValues(values map[string]string) []byte {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	b := AppendVarInt(nil, int32(len(keys)))
	for _, k := range keys {
		key := k
		if !strings.Contains(key, ":") {
			key = "minecraft:" + key
		}
		b = AppendString(b, key)
		b = AppendString(b, values[k])
	}
	return b
}
