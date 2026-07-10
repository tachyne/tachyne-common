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
