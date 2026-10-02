package gwsession

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/tachyne/tachyne-common/protocol"
	"github.com/tachyne/tachyne-common/render770"
)

// signedCmdTree is a Commands packet body with vanilla's message commands:
// msg <targets> <message>, say <message>, tell → msg (a redirect), and a
// plain list.
func signedCmdTree() []byte {
	b := protocol.AppendVarInt(nil, 8)
	node := func(flags byte, kids ...int32) {
		b = append(b, flags)
		b = protocol.AppendVarInt(b, int32(len(kids)))
		for _, k := range kids {
			b = protocol.AppendVarInt(b, k)
		}
	}
	node(0x00, 1, 4, 6, 7) // 0 root
	node(0x01, 2)          // 1 msg
	b = protocol.AppendString(b, "msg")
	node(0x02, 3) // 2 targets: minecraft:entity, players only
	b = protocol.AppendString(b, "targets")
	b = protocol.AppendVarInt(b, 6)
	b = append(b, 0x02)
	node(0x06) // 3 message: minecraft:message, executable
	b = protocol.AppendString(b, "message")
	b = protocol.AppendVarInt(b, render770.ParserMessage)
	node(0x01, 5) // 4 say
	b = protocol.AppendString(b, "say")
	node(0x06) // 5 message
	b = protocol.AppendString(b, "message")
	b = protocol.AppendVarInt(b, render770.ParserMessage)
	node(0x09) // 6 tell, redirected to msg
	b = protocol.AppendVarInt(b, 1)
	b = protocol.AppendString(b, "tell")
	node(0x05) // 7 list
	b = protocol.AppendString(b, "list")
	return protocol.AppendVarInt(b, 0)
}

// signArg is what the client sends for a signed command: one argument
// signature over the argument's value at the next link.
func (p *testPlayer) signArg(t *testing.T, cmd, name, value string, ts int64) render770.ChatCommandSigned {
	seen, u := p.cli.update()
	sum := sha256.Sum256(oracleMessagePayload(p.uuid, p.sess.SessionID, p.next, value, ts, 77, seen))
	sig, err := rsa.SignPKCS1v15(rand.Reader, p.key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	p.next++
	return render770.ChatCommandSigned{Command: cmd, Timestamp: ts, Salt: 77,
		Arguments: []render770.ArgumentSignature{{Name: name, Signature: sig}}, LastSeen: u}
}

func TestSignableArgumentsIndex(t *testing.T) {
	idx, ok := render770.SignableArguments(signedCmdTree())
	if !ok {
		t.Fatal("tree unreadable")
	}
	if a := idx["msg"]; len(a) != 1 || a[0].Name != "message" || a[0].Depth != 2 {
		t.Fatalf("msg: %+v", a)
	}
	if a := idx["tell"]; len(a) != 1 || a[0].Depth != 2 {
		t.Fatalf("tell (redirect): %+v", a)
	}
	if a := idx["say"]; len(a) != 1 || a[0].Depth != 1 {
		t.Fatalf("say: %+v", a)
	}
	if _, ok := idx["list"]; ok {
		t.Fatal("list has no message argument")
	}
	if len(idx.For("msg Bob")) != 0 || len(idx.For("msg Bob hi there")) != 1 {
		t.Fatal("reach by word count")
	}
}

// /msg and /say signed through the gateway's command path: the argument
// signature checks against the message's value in the command line,
// advances the chain like chat does, and rides to the world with its
// content; a wrong name breaks the chain; no signature from a key holder is
// refused; a plain chat_command for a message command is refused while
// secure chat is enforced.
func TestSignedCommandArguments(t *testing.T) {
	services := genKey(t)
	keys := StaticServicesKeys(&services.PublicKey)
	a := newTestPlayer(t, services, keys, 1)
	if _, kick := a.st.sessionUpdate(a.sess); kick != "" {
		t.Fatal(kick)
	}
	a.st.setCommandTree(signedCmdTree())
	now := time.Now()
	ts := now.UnixMilli()

	r := a.st.receiveSignedCommand(a.signArg(t, "msg Bob hello there", "message", "hello there", ts), now)
	if r.kick != "" || r.refuse != "" || len(r.args) != 1 {
		t.Fatalf("signed /msg: %+v", r)
	}
	if got := r.args[0]; got.Name != "message" || got.Content != "hello there" || got.Chat.Index != 0 || got.Chat.Timestamp != ts {
		t.Fatalf("argument %+v", got)
	}

	// The chain moved on: the next chat message signs link 1.
	if r := a.st.receiveChat(a.say(t, "after", ts+1), now); r.signed == nil || r.signed.Index != 1 {
		t.Fatalf("chat after a signed command: %+v", r)
	}

	r = a.st.receiveSignedCommand(a.signArg(t, "tell Bob a b", "message", "a b", ts+2), now)
	if r.refuse != "" || len(r.args) != 1 || r.args[0].Content != "a b" || r.args[0].Chat.Index != 2 {
		t.Fatalf("signed /tell: %+v", r)
	}

	r = a.st.receiveSignedCommand(a.signArg(t, "say hi all", "message", "hi all", ts+3), now)
	if r.refuse != "" || len(r.args) != 1 || r.args[0].Content != "hi all" {
		t.Fatalf("signed /say: %+v", r)
	}

	// Unsigned arguments from a player with a session.
	_, u := a.cli.update()
	if r := a.st.receiveSignedCommand(render770.ChatCommandSigned{Command: "say plain", LastSeen: u}, now); r.refuse != keyMissingProfileKey {
		t.Fatalf("unsigned /say from a key holder: %+v", r)
	}
	if !a.st.unsignedCommandRefused("say plain") || a.st.unsignedCommandRefused("list") {
		t.Fatal("chat_command refusal")
	}

	// A signature over an argument the command does not have.
	r = a.st.receiveSignedCommand(a.signArg(t, "list x", "message", "x", ts+4), now)
	if r.refuse != keyInvalidCommandSig {
		t.Fatalf("mismatched argument: %+v", r)
	}
	if r := a.st.receiveChat(a.say(t, "dead", ts+5), now); r.refuse != keyChainBroken {
		t.Fatalf("chain after a mismatch: %+v", r)
	}

	// A forged value: the signature covers other text.
	b := newTestPlayer(t, services, keys, 2)
	b.st.sessionUpdate(b.sess)
	b.st.setCommandTree(signedCmdTree())
	if r := b.st.receiveSignedCommand(b.signArg(t, "say forged", "message", "original", ts), now); r.refuse != keyInvalidSignature {
		t.Fatalf("forged value: %+v", r)
	}
}
