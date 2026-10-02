package gwsession

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"testing"
	"time"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
	"github.com/tachyne/tachyne-common/render770"
)

// oracleMessagePayload spells out PlayerChatMessage.updateSignature byte by
// byte: Ints(1) · sender · session · Ints(index) · Longs(salt) ·
// Longs(epochSecond) · Ints(len) · UTF-8 · Ints(count) · signatures.
func oracleMessagePayload(sender, session [16]byte, index int32, content string, tsMillis, salt int64, lastSeen [][]byte) []byte {
	var b bytes.Buffer
	w := func(v any) { binary.Write(&b, binary.BigEndian, v) }
	w(int32(1))
	b.Write(sender[:])
	b.Write(session[:])
	w(index)
	w(salt)
	w(tsMillis / 1000)
	w(int32(len([]byte(content))))
	b.WriteString(content)
	w(int32(len(lastSeen)))
	for _, s := range lastSeen {
		b.Write(s)
	}
	return b.Bytes()
}

func TestMessagePayloadMatchesVanilla(t *testing.T) {
	sender, session := [16]byte{1, 2, 3}, [16]byte{9, 8, 7}
	ls := [][]byte{bytes.Repeat([]byte{5}, 256)}
	got := messagePayload(sender, session, 3, "héllo", 1700000001999, -7, ls)
	if want := oracleMessagePayload(sender, session, 3, "héllo", 1700000001999, -7, ls); !bytes.Equal(got, want) {
		t.Fatalf("payload\n got %x\nwant %x", got, want)
	}
}

// Arrays.hashCode over signed bytes, then LastSeenMessages' 31-fold with
// the 0 → 1 escape.
func TestLastSeenChecksum(t *testing.T) {
	if got := sigChecksum([]byte{1, 2}); got != 994 { // 31*(31*1+1)+2
		t.Fatalf("sigChecksum = %d, want 994", got)
	}
	if got := sigChecksum([]byte{0xff}); got != 30 { // 31*1 + (-1)
		t.Fatalf("sigChecksum signed byte = %d, want 30", got)
	}
	if got := lastSeenChecksum([][]byte{{1, 2}}); got != byte((31+994)&0xff) {
		t.Fatalf("lastSeenChecksum = %d", got)
	}
	if got := lastSeenChecksum(nil); got != 1 {
		t.Fatalf("empty checksum = %d, want 1", got)
	}
}

// MessageSignatureCache.push: last-seen then the signature land newest
// first, displaced entries shift back.
func TestSigCachePush(t *testing.T) {
	a, b, c, d := []byte("a"), []byte("b"), []byte("c"), []byte("d")
	var cache sigCache
	cache.push([][]byte{a, b}, c)
	for i, want := range [][]byte{c, b, a} {
		if !bytes.Equal(cache.entries[i], want) {
			t.Fatalf("after first push entry %d = %q, want %q", i, cache.entries[i], want)
		}
	}
	cache.push(nil, d)
	for i, want := range [][]byte{d, c, b, a} {
		if !bytes.Equal(cache.entries[i], want) {
			t.Fatalf("after second push entry %d = %q, want %q", i, cache.entries[i], want)
		}
	}
	if p := cache.pack(b); p.ID != 2 {
		t.Fatalf("pack(b) = %+v, want index 2", p)
	}
	if p := cache.pack([]byte("z")); p.ID != -1 || string(p.Full) != "z" {
		t.Fatalf("pack(unknown) = %+v, want full", p)
	}
}

// clientTracker is the client's LastSeenMessagesTracker, the other end of
// the validator: what it acknowledges is what the gateway must resolve.
type clientTracker struct {
	entries [20][]byte
	tail    int
	offset  int32
}

func (c *clientTracker) add(sig []byte) {
	c.entries[c.tail] = sig
	c.tail = (c.tail + 1) % len(c.entries)
	c.offset++
}

func (c *clientTracker) update() ([][]byte, render770.LastSeenUpdate) {
	u := render770.LastSeenUpdate{Offset: c.offset}
	c.offset = 0
	var seen [][]byte
	for i := range c.entries {
		if s := c.entries[(c.tail+i)%len(c.entries)]; s != nil {
			u.Acknowledged |= 1 << uint(i)
			seen = append(seen, s)
		}
	}
	u.Checksum = lastSeenChecksum(seen)
	return seen, u
}

type testPlayer struct {
	uuid [16]byte
	key  *rsa.PrivateKey
	sess attach.ChatSession
	st   *chatState
	next int32
	cli  clientTracker
}

func genKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// mojangSign is the services key's SHA1withRSA over the profile payload.
func mojangSign(t *testing.T, services *rsa.PrivateKey, uuid [16]byte, s attach.ChatSession) []byte {
	payload := append(append([]byte(nil), uuid[:]...), protocol.AppendI64(nil, s.ExpiresAt)...)
	payload = append(payload, s.Key...)
	sum := sha1.Sum(payload)
	sig, err := rsa.SignPKCS1v15(rand.Reader, services, crypto.SHA1, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return sig
}

func newTestPlayer(t *testing.T, services *rsa.PrivateKey, keys *ServicesKeys, id byte) *testPlayer {
	p := &testPlayer{uuid: [16]byte{id, 0xaa}, key: genKey(t)}
	der, err := x509.MarshalPKIXPublicKey(&p.key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	p.sess = attach.ChatSession{SessionID: [16]byte{id, 0x55}, ExpiresAt: time.Now().Add(time.Hour).UnixMilli(), Key: der}
	p.sess.KeySig = mojangSign(t, services, p.uuid, p.sess)
	p.st = newChatState(p.uuid, true, keys)
	return p
}

// say composes what the client sends: its tracker's update and a signature
// over the next link with the acknowledged signatures.
func (p *testPlayer) say(t *testing.T, text string, ts int64) render770.ChatMessage {
	seen, u := p.cli.update()
	sum := sha256.Sum256(oracleMessagePayload(p.uuid, p.sess.SessionID, p.next, text, ts, 77, seen))
	sig, err := rsa.SignPKCS1v15(rand.Reader, p.key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	p.next++
	return render770.ChatMessage{Text: text, Timestamp: ts, Salt: 77, Signature: sig, LastSeen: u}
}

// relay is the world's broadcast: one PlayerChat per recipient, each
// rendered by that recipient's gateway (and seen by its client).
func relay(t *testing.T, from *testPlayer, text string, signed *attach.SignedChat, to ...*testPlayer) {
	e := attach.PlayerChat{Sender: from.uuid, SenderName: "p", Index: signed.Index, Signature: signed.Signature,
		Content: text, Timestamp: signed.Timestamp, Salt: signed.Salt, LastSeen: signed.LastSeen}
	for _, r := range to {
		if _, kick := r.st.render(e); kick != "" {
			t.Fatalf("render kicked: %s", kick)
		}
		r.cli.add(signed.Signature)
	}
}

// A signed conversation through the gateway entry points: the session
// update, each message decoded through the chain with the last-seen window
// the client acknowledged, and every recipient's render feeding the next
// acknowledgement.
func TestSecureChatConversation(t *testing.T) {
	services := genKey(t)
	keys := StaticServicesKeys(&services.PublicKey)
	a := newTestPlayer(t, services, keys, 1)
	b := newTestPlayer(t, services, keys, 2)
	for _, p := range []*testPlayer{a, b} {
		fwd, kick := p.st.sessionUpdate(p.sess)
		if kick != "" || fwd == nil || fwd.SessionID != p.sess.SessionID {
			t.Fatalf("session update: fwd=%v kick=%q", fwd, kick)
		}
		if again, kick := p.st.sessionUpdate(p.sess); again != nil || kick != "" {
			t.Fatalf("the same session again was re-broadcast (%v, %q)", again, kick)
		}
	}
	now := time.Now()
	ts := now.UnixMilli()

	r1 := a.st.receiveChat(a.say(t, "hello", ts), now)
	if r1.kick != "" || r1.refuse != "" || r1.signed == nil || r1.signed.Index != 0 {
		t.Fatalf("first message: %+v", r1)
	}
	relay(t, a, "hello", r1.signed, a, b)

	r2 := b.st.receiveChat(b.say(t, "hi a", ts+10), now)
	if r2.kick != "" || r2.refuse != "" || r2.signed == nil {
		t.Fatalf("reply: %+v", r2)
	}
	if len(r2.signed.LastSeen) != 1 || !bytes.Equal(r2.signed.LastSeen[0], r1.signed.Signature) {
		t.Fatalf("reply last seen = %d entries, want a's message", len(r2.signed.LastSeen))
	}
	relay(t, b, "hi a", r2.signed, a, b)

	// a's cache holds both signatures now: the reply's last-seen entry packs
	// to an index on a's next render of it.
	if p := a.st.cache.pack(r1.signed.Signature); p.ID < 0 {
		t.Fatal("a's cache lost its own message")
	}

	r3 := a.st.receiveChat(a.say(t, "second", ts+20), now)
	if r3.kick != "" || r3.refuse != "" || r3.signed == nil || r3.signed.Index != 1 || len(r3.signed.LastSeen) != 2 {
		t.Fatalf("a's second message: %+v", r3)
	}
	if kick := a.st.ack(0); kick != "" {
		t.Fatalf("chat_ack 0: %s", kick)
	}
}

func TestSecureChatRefusals(t *testing.T) {
	services := genKey(t)
	keys := StaticServicesKeys(&services.PublicKey)
	now := time.Now()

	// A key Mojang did not sign disconnects (handleChatSessionUpdate).
	p := newTestPlayer(t, services, keys, 3)
	forged := p.sess
	forged.KeySig = bytes.Repeat([]byte{1}, len(forged.KeySig))
	if _, kick := p.st.sessionUpdate(forged); kick != keyInvalidKeySignature {
		t.Fatalf("forged key: kick %q", kick)
	}
	// A replacement key expiring before the current one disconnects.
	if _, kick := p.st.sessionUpdate(p.sess); kick != "" {
		t.Fatalf("valid key: kick %q", kick)
	}
	older := p.sess
	older.ExpiresAt -= 1000
	older.KeySig = mojangSign(t, services, p.uuid, older)
	if _, kick := p.st.sessionUpdate(older); kick != keyExpiredPublicKey {
		t.Fatalf("older key: kick %q", kick)
	}

	// A tampered message breaks the chain; later ones say so.
	m := p.say(t, "real", now.UnixMilli())
	m.Text = "forged"
	if r := p.st.receiveChat(m, now); r.refuse != keyInvalidSignature {
		t.Fatalf("tampered: %+v", r)
	}
	if r := p.st.receiveChat(p.say(t, "after", now.UnixMilli()), now); r.refuse != keyChainBroken {
		t.Fatalf("after a broken chain: %+v", r)
	}

	// Out-of-order timestamps.
	q := newTestPlayer(t, services, keys, 4)
	q.st.sessionUpdate(q.sess)
	if r := q.st.receiveChat(q.say(t, "one", now.UnixMilli()), now); r.signed == nil {
		t.Fatalf("first: %+v", r)
	}
	if r := q.st.receiveChat(q.say(t, "two", now.UnixMilli()-5000), now); r.refuse != keyOutOfOrderChat {
		t.Fatalf("out of order: %+v", r)
	}

	// No session on an enforcing server: refused; offline: unsigned passes.
	n := newChatState([16]byte{5}, true, keys)
	if r := n.receiveChat(render770.ChatMessage{Text: "plain"}, now); r.refuse != keyMissingProfileKey {
		t.Fatalf("unsigned on an enforcing server: %+v", r)
	}
	off := newChatState([16]byte{6}, false, nil)
	if r := off.receiveChat(render770.ChatMessage{Text: "plain"}, now); r.kick != "" || r.refuse != "" || r.signed != nil {
		t.Fatalf("offline unsigned: %+v", r)
	}
	// With no services keys the server does not enforce either.
	nokeys := newChatState([16]byte{7}, true, StaticServicesKeys())
	if r := nokeys.receiveChat(render770.ChatMessage{Text: "plain"}, now); r.refuse != "" || r.kick != "" {
		t.Fatalf("keyless server refused unsigned chat: %+v", r)
	}

	// Illegal characters and a bad acknowledgement disconnect.
	if r := off.receiveChat(render770.ChatMessage{Text: "a§b"}, now); r.kick != keyIllegalCharacters {
		t.Fatalf("section sign: %+v", r)
	}
	if kick := off.ack(1); kick != keyValidationFailed {
		t.Fatalf("ack past the window: %q", kick)
	}
	if r := off.receiveChat(render770.ChatMessage{Text: "x", LastSeen: render770.LastSeenUpdate{Acknowledged: 1}}, now); r.kick != keyValidationFailed {
		t.Fatalf("acknowledged an unknown message: %+v", r)
	}

	// A signed command with argument signatures the tree cannot have.
	if r := q.st.receiveSignedCommand(render770.ChatCommandSigned{Command: "say hi", Arguments: []render770.ArgumentSignature{{Name: "message"}}}, now); r.refuse != keyInvalidCommandSig {
		t.Fatalf("signed argument: %+v", r)
	}
	if r := off.receiveSignedCommand(render770.ChatCommandSigned{Command: "list"}, now); r.kick != "" || r.refuse != "" {
		t.Fatalf("plain signed command: %+v", r)
	}
}

// Checksum disagreement is a desync: vanilla disconnects.
func TestLastSeenChecksumMismatch(t *testing.T) {
	st := newChatState([16]byte{8}, false, nil)
	s1 := bytes.Repeat([]byte{9}, 256)
	st.render(attach.PlayerChat{Content: "x", Signature: s1})
	u := render770.LastSeenUpdate{Offset: 1, Acknowledged: 1 << 19, Checksum: lastSeenChecksum([][]byte{s1}) + 1}
	if u.Checksum == 0 {
		u.Checksum = 2
	}
	if r := st.receiveChat(render770.ChatMessage{Text: "y", LastSeen: u}, time.Now()); r.kick != keyValidationFailed {
		t.Fatalf("checksum mismatch: %+v", r)
	}
}
