package render770

import (
	"bytes"
	"testing"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// Oracles: verbatim copies of the gomc hub's presence builders at deletion
// time (stage 2 of the domain-events refactor).

func oracleChatNBT(s string) []byte {
	b := []byte{0x08, byte(len(s) >> 8), byte(len(s))}
	return append(b, s...)
}

func oracleSystemChat(text string, actionBar bool) []byte {
	return protocol.AppendBool(oracleChatNBT(text), actionBar)
}

func oracleBossBarAdd(uuid [16]byte, title string, health float32) []byte {
	b := append([]byte(nil), uuid[:]...)
	b = protocol.AppendVarInt(b, 0)
	b = append(b, oracleChatNBT(title)...)
	b = protocol.AppendF32(b, health)
	b = protocol.AppendVarInt(b, 5)
	b = protocol.AppendVarInt(b, 0)
	b = protocol.AppendU8(b, 0)
	return b
}

func oracleTimePacket(age, dayTime uint64) []byte {
	b := protocol.AppendI64(nil, int64(age))
	b = protocol.AppendI64(b, int64(dayTime%24000))
	return protocol.AppendBool(b, true)
}

func TestChatMatchesOracle(t *testing.T) {
	eq(t, "chat line", Chat(attach.Chat{Text: "hello world"}),
		IDSystemChat, oracleSystemChat("hello world", false))
	eq(t, "action bar", Chat(attach.Chat{Text: "N 12 64 -3", ActionBar: true}),
		IDSystemChat, oracleSystemChat("N 12 64 -3", true))
}

// oracleProfilelessChat builds the expected profileless_chat body: message nbt,
// ChatTypesHolder registry ref (minecraft:chat index 0 → varint 1), sender name
// nbt, absent target.
func oracleProfilelessChat(text, sender string) []byte {
	b := oracleChatNBT(text)
	b = protocol.AppendVarInt(b, 1) // index 0 + 1
	b = append(b, oracleChatNBT(sender)...)
	return protocol.AppendBool(b, false) // target absent
}

func TestChatSenderIsProfileless(t *testing.T) {
	// A Sender routes to profileless_chat (player-chat decoration "<sender> text"),
	// NOT system_chat.
	eq(t, "player chat", Chat(attach.Chat{Text: "hello world", Sender: "LegionZA"}),
		IDProfilelessChat, oracleProfilelessChat("hello world", "LegionZA"))

	// Re-parse round trip: message, holder=1, name, target-absent.
	got := Chat(attach.Chat{Text: "hi", Sender: "EdgeZA"})
	if got.ID != IDProfilelessChat {
		t.Fatalf("id = 0x%x, want profileless_chat 0x%x", got.ID, IDProfilelessChat)
	}
	body := got.Body
	msg, n := parseNBTString(t, body)
	body = body[n:]
	holder := body[0] // single-byte varint (1 = registry index 0 + 1)
	body = body[1:]
	name, nn := parseNBTString(t, body)
	body = body[nn:]
	if len(body) != 1 || body[0] != 0 {
		t.Fatalf("target not an absent optional: %x", body)
	}
	if msg != "hi" || name != "EdgeZA" || holder != 1 {
		t.Fatalf("round trip: msg=%q holder=%d name=%q", msg, holder, name)
	}

	// ActionBar overrides Sender — HUD stays a system message.
	if got := Chat(attach.Chat{Text: "N 1 2 3", Sender: "x", ActionBar: true}); got.ID != IDSystemChat {
		t.Fatalf("action-bar+sender id = 0x%x, want system_chat", got.ID)
	}
}

// parseNBTString reads a nameless-root TAG_String (0x08 + u16 len + bytes).
func parseNBTString(t *testing.T, b []byte) (string, int) {
	t.Helper()
	if len(b) < 3 || b[0] != 0x08 {
		t.Fatalf("not a TAG_String: %x", b)
	}
	l := int(b[1])<<8 | int(b[2])
	return string(b[3 : 3+l]), 3 + l
}

func TestChatSanitizes(t *testing.T) {
	// NUL and astral runes become '?', length caps at 256.
	got := Chat(attach.Chat{Text: "a\x00b\U0001F600c"})
	want := oracleSystemChat("a?b?c", false)
	if !bytes.Equal(got.Body, want) {
		t.Fatalf("sanitize mismatch\n got %x\nwant %x", got.Body, want)
	}
	long := make([]byte, 300)
	for i := range long {
		long[i] = 'x'
	}
	got = Chat(attach.Chat{Text: string(long)})
	if len(got.Body) != 3+256+1 { // tag+len + capped text + actionbar bool
		t.Fatalf("length cap: body %d bytes", len(got.Body))
	}
}

func TestBossBarMatchesOracle(t *testing.T) {
	uuid := [16]byte{0xdd, 1}
	eq(t, "add", BossBar(attach.BossBar{UUID: uuid, Op: attach.BossBarAdd, Title: "Ender Dragon", Health: 0.75}),
		IDBossBar, oracleBossBarAdd(uuid, "Ender Dragon", 0.75))
	wantHealth := append(append([]byte(nil), uuid[:]...), 2)
	wantHealth = protocol.AppendF32(wantHealth, 0.5)
	eq(t, "health", BossBar(attach.BossBar{UUID: uuid, Op: attach.BossBarHealth, Health: 0.5}),
		IDBossBar, wantHealth)
	wantRemove := append(append([]byte(nil), uuid[:]...), 1)
	eq(t, "remove", BossBar(attach.BossBar{UUID: uuid, Op: attach.BossBarRemove}),
		IDBossBar, wantRemove)
}

func TestTimeMatchesOracle(t *testing.T) {
	eq(t, "with age", Time(attach.Time{Age: 100000, Time: 30500}),
		IDUpdateTime, oracleTimePacket(100000, 30500))
	// No age → the clock stands in (what gateways did at join).
	eq(t, "age fallback", Time(attach.Time{Time: 9313}),
		IDUpdateTime, oracleTimePacket(9313, 9313))
}
