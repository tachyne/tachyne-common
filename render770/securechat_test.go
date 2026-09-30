package render770

import (
	"bytes"
	"io"
	"testing"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

func sig(fill byte) []byte { return bytes.Repeat([]byte{fill}, SignatureBytes) }

func mustVarInt(t *testing.T, r *bytes.Reader, want int32, what string) {
	t.Helper()
	v, err := protocol.ReadVarInt(r)
	if err != nil || v != want {
		t.Fatalf("%s = %d (%v), want %d", what, v, err, want)
	}
}

func mustBytes(t *testing.T, r *bytes.Reader, want []byte, what string) {
	t.Helper()
	got := make([]byte, len(want))
	if _, err := io.ReadFull(r, got); err != nil || !bytes.Equal(got, want) {
		t.Fatalf("%s = %x (%v), want %x", what, got, err, want)
	}
}

func mustI64(t *testing.T, r *bytes.Reader, want int64, what string) {
	t.Helper()
	var b [8]byte
	if _, err := io.ReadFull(r, b[:]); err != nil || int64(be64(b[:])) != want {
		t.Fatalf("%s = %d (%v), want %d", what, int64(be64(b[:])), err, want)
	}
}

func mustByte(t *testing.T, r *bytes.Reader, want byte, what string) {
	t.Helper()
	b, err := r.ReadByte()
	if err != nil || b != want {
		t.Fatalf("%s = %d (%v), want %d", what, b, err, want)
	}
}

// TestPlayerChatReparse re-reads player_chat field by field as
// ClientboundPlayerChatPacket.STREAM_CODEC does.
func TestPlayerChatReparse(t *testing.T) {
	sender := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	e := attach.PlayerChat{
		Sender: sender, SenderName: "Steve", Index: 7, Signature: sig(0xAB),
		Content: "hello there", Timestamp: 1700000000123, Salt: -42,
		Unsigned: "hello, decorated",
	}
	packed := []PackedSignature{{ID: 3}, {ID: -1, Full: sig(0x11)}}
	p := PlayerChat(12, e, packed)
	if p.ID != IDPlayerChat {
		t.Fatalf("id 0x%02x, want 0x%02x", p.ID, IDPlayerChat)
	}
	r := bytes.NewReader(p.Body)
	mustVarInt(t, r, 12, "global index")
	mustBytes(t, r, sender[:], "sender")
	mustVarInt(t, r, 7, "link index")
	mustByte(t, r, 1, "signature present")
	mustBytes(t, r, sig(0xAB), "signature")
	if s, err := protocol.ReadString(r); err != nil || s != "hello there" {
		t.Fatalf("content %q (%v)", s, err)
	}
	mustI64(t, r, 1700000000123, "timestamp")
	mustI64(t, r, -42, "salt")
	mustVarInt(t, r, 2, "last-seen count")
	mustVarInt(t, r, 4, "packed cache id (3+1)")
	mustVarInt(t, r, 0, "packed full marker (-1+1)")
	mustBytes(t, r, sig(0x11), "full last-seen signature")
	mustByte(t, r, 1, "unsigned content present")
	mustBytes(t, r, chatNBT("hello, decorated"), "unsigned content")
	mustVarInt(t, r, 0, "filter mask pass-through")
	mustVarInt(t, r, 1, "chat type minecraft:chat (0+1)")
	mustBytes(t, r, chatNBT("Steve"), "sender name")
	mustByte(t, r, 0, "no target")
	if r.Len() != 0 {
		t.Fatalf("%d trailing bytes", r.Len())
	}

	// Unsigned, as PlayerChatMessage.unsigned: no signature, no last seen.
	u := PlayerChat(0, attach.PlayerChat{Sender: sender, SenderName: "Alex", Content: "hi", Timestamp: 5, ChatType: "minecraft:say_command", Target: "Bob"}, nil)
	r = bytes.NewReader(u.Body)
	mustVarInt(t, r, 0, "global index")
	mustBytes(t, r, sender[:], "sender")
	mustVarInt(t, r, 0, "link index")
	mustByte(t, r, 0, "no signature")
	if s, _ := protocol.ReadString(r); s != "hi" {
		t.Fatalf("content %q", s)
	}
	mustI64(t, r, 5, "timestamp")
	mustI64(t, r, 0, "salt")
	mustVarInt(t, r, 0, "last-seen count")
	mustByte(t, r, 0, "no unsigned content")
	mustVarInt(t, r, 0, "filter mask")
	mustVarInt(t, r, 5, "say_command (4+1)")
	mustBytes(t, r, chatNBT("Alex"), "name")
	mustByte(t, r, 1, "target present")
	mustBytes(t, r, chatNBT("Bob"), "target")
	if r.Len() != 0 {
		t.Fatalf("%d trailing bytes", r.Len())
	}
}

// The chain carries the canonical ids to the 26.x packets of the same name.
func TestSecureChatIDsThroughTheChain(t *testing.T) {
	for _, c := range []struct {
		proto            int32
		playerChat, dele int32
	}{{776, 0x41, 0x1f}, {777, 0x42, 0x1f}} {
		tr := protocol.TranslatorFor(c.proto)
		if id, _, drop := tr.Clientbound(protocol.StatePlay, IDPlayerChat, PlayerChat(0, attach.PlayerChat{Content: "x"}, nil).Body); drop || id != c.playerChat {
			t.Errorf("%d: player_chat -> 0x%02x drop=%v, want 0x%02x", c.proto, id, drop, c.playerChat)
		}
		if id, _, drop := tr.Clientbound(protocol.StatePlay, IDDeleteChat, DeleteChat(PackedSignature{ID: 0}).Body); drop || id != c.dele {
			t.Errorf("%d: delete_chat -> 0x%02x drop=%v, want 0x%02x", c.proto, id, drop, c.dele)
		}
		// Serverbound: chat_ack 0x06, chat_command_signed 0x08, chat 0x09,
		// chat_session_update 0x0a on 26.2 and 26.3.
		for client, canon := range map[int32]int32{0x06: SIDChatAck, 0x08: SIDChatCommandSigned, 0x09: SIDChat, 0x0a: SIDChatSessionUpdate} {
			if id, _, drop := tr.Serverbound(protocol.StatePlay, client, []byte{0}); drop || id != canon {
				t.Errorf("%d: serverbound 0x%02x -> 0x%02x drop=%v, want 0x%02x", c.proto, client, id, drop, canon)
			}
		}
	}
}

func TestDeleteChatLayout(t *testing.T) {
	if p := DeleteChat(PackedSignature{ID: 5}); p.ID != IDDeleteChat || !bytes.Equal(p.Body, []byte{6}) {
		t.Fatalf("cached delete = 0x%02x %x", p.ID, p.Body)
	}
	p := DeleteChat(PackedSignature{ID: -1, Full: sig(0x22)})
	if !bytes.Equal(p.Body, append([]byte{0}, sig(0x22)...)) {
		t.Fatalf("full delete = %x", p.Body)
	}
}

func oracleChatSession(s attach.ChatSession) []byte {
	b := []byte{1}
	b = append(b, s.SessionID[:]...)
	b = protocol.AppendI64(b, s.ExpiresAt)
	b = protocol.AppendVarInt(b, int32(len(s.Key)))
	b = append(b, s.Key...)
	b = protocol.AppendVarInt(b, int32(len(s.KeySig)))
	return append(b, s.KeySig...)
}

// INITIALIZE_CHAT: alone, and inside a fresh entry right after ADD_PLAYER's
// fields (the action enum order).
func TestPlayerInfoChatSession(t *testing.T) {
	uuid := [16]byte{9, 9}
	s := attach.ChatSession{SessionID: [16]byte{7}, ExpiresAt: 1234567, Key: []byte{1, 2, 3}, KeySig: []byte{4, 5}}
	eq(t, "initialize chat", PlayerInfoChat(attach.PlayerInfoChat{UUID: uuid, Session: &s}),
		IDPlayerInfo, append(append([]byte{0x02, 1}, uuid[:]...), oracleChatSession(s)...))
	eq(t, "initialize chat cleared", PlayerInfoChat(attach.PlayerInfoChat{UUID: uuid}),
		IDPlayerInfo, append(append([]byte{0x02, 1}, uuid[:]...), 0))

	want := protocol.AppendU8(nil, 0x01|0x02|0x04|0x08|0x10)
	want = protocol.AppendVarInt(want, 1)
	want = append(want, uuid[:]...)
	want = protocol.AppendString(want, "Steve")
	want = protocol.AppendVarInt(want, 0) // no properties
	want = append(want, oracleChatSession(s)...)
	want = protocol.AppendVarInt(want, 1) // game mode
	want = protocol.AppendVarInt(want, 1) // listed
	want = protocol.AppendVarInt(want, 0) // latency
	eq(t, "add with chat", PlayerInfoAdd(attach.PlayerInfo{UUID: uuid, Name: "Steve", Gamemode: 1, Chat: &s}), IDPlayerInfo, want)
}

func lastSeenBytes(off int32, ack uint32, sum byte) []byte {
	b := protocol.AppendVarInt(nil, off)
	return append(b, byte(ack), byte(ack>>8), byte(ack>>16), sum)
}

func TestParseChatMessage(t *testing.T) {
	b := protocol.AppendString(nil, "hi all")
	b = protocol.AppendI64(b, 1700000000000)
	b = protocol.AppendI64(b, 99)
	b = append(b, 1)
	b = append(b, sig(0x33)...)
	b = append(b, lastSeenBytes(2, 1<<0|1<<19, 0x5a)...)
	m, ok := ParseChatMessage(b)
	if !ok || m.Text != "hi all" || m.Timestamp != 1700000000000 || m.Salt != 99 || !bytes.Equal(m.Signature, sig(0x33)) {
		t.Fatalf("parsed %+v ok=%v", m, ok)
	}
	if m.LastSeen != (LastSeenUpdate{Offset: 2, Acknowledged: 1 | 1<<19, Checksum: 0x5a}) {
		t.Fatalf("last seen %+v", m.LastSeen)
	}
	// Unsigned (no signature), then truncated and over-long.
	u := protocol.AppendString(nil, "x")
	u = protocol.AppendI64(u, 1)
	u = protocol.AppendI64(u, 0)
	u = append(u, 0)
	u = append(u, lastSeenBytes(0, 0, 0)...)
	if m, ok := ParseChatMessage(u); !ok || m.Signature != nil {
		t.Fatalf("unsigned parse %+v ok=%v", m, ok)
	}
	if _, ok := ParseChatMessage(u[:len(u)-1]); ok {
		t.Fatal("a truncated chat parsed")
	}
	if _, ok := ParseChatMessage(append(u, 0)); ok {
		t.Fatal("trailing bytes parsed")
	}
}

func TestParseChatCommandSigned(t *testing.T) {
	b := protocol.AppendString(nil, "say hi")
	b = protocol.AppendI64(b, 10)
	b = protocol.AppendI64(b, 20)
	b = protocol.AppendVarInt(b, 1)
	b = protocol.AppendString(b, "message")
	b = append(b, sig(0x44)...)
	b = append(b, lastSeenBytes(0, 0, 0)...)
	c, ok := ParseChatCommandSigned(b)
	if !ok || c.Command != "say hi" || c.Timestamp != 10 || c.Salt != 20 || len(c.Arguments) != 1 ||
		c.Arguments[0].Name != "message" || !bytes.Equal(c.Arguments[0].Signature, sig(0x44)) {
		t.Fatalf("parsed %+v ok=%v", c, ok)
	}
}

func TestParseChatAckAndSession(t *testing.T) {
	if off, ok := ParseChatAck(protocol.AppendVarInt(nil, 300)); !ok || off != 300 {
		t.Fatalf("ack %d %v", off, ok)
	}
	s := attach.ChatSession{SessionID: [16]byte{1, 2}, ExpiresAt: 42, Key: bytes.Repeat([]byte{3}, 294), KeySig: bytes.Repeat([]byte{4}, 512)}
	body := oracleChatSession(s)[1:] // the packet is the Data itself, not nullable
	got, ok := ParseChatSessionUpdate(body)
	if !ok || got.SessionID != s.SessionID || got.ExpiresAt != 42 || !bytes.Equal(got.Key, s.Key) || !bytes.Equal(got.KeySig, s.KeySig) {
		t.Fatalf("session %+v ok=%v", got, ok)
	}
	big := s
	big.Key = bytes.Repeat([]byte{3}, maxPublicKeyBytes+1)
	if _, ok := ParseChatSessionUpdate(oracleChatSession(big)[1:]); ok {
		t.Fatal("an over-long key parsed")
	}
}
