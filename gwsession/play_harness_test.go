package gwsession

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net"
	"sync"
	"testing"
	"time"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
	"github.com/tachyne/tachyne-common/render770"
)

// playHarness runs the real play bridge between a pipe "client" and a pipe
// "world": what the client is sent is collected, what the gateway writes to
// the world arrives on frames.
type playHarness struct {
	t      *testing.T
	proto  int32
	tr     protocol.Translator
	world  net.Conn // the world's end
	mu     sync.Mutex
	pkts   []protocol.Packet
	frames chan worldFrame
	done   chan error
	cc     *clientConn
}

type worldFrame struct {
	typ     byte
	payload []byte
}

func startPlay(t *testing.T, proto int32, cfg Config, welcome attach.Welcome, chat *chatState) *playHarness {
	t.Helper()
	gwSide, clientSide := net.Pipe()
	gwWorld, worldSide := net.Pipe()
	h := &playHarness{t: t, proto: proto, tr: protocol.TranslatorFor(proto), world: worldSide,
		frames: make(chan worldFrame, 256), done: make(chan error, 1)}
	h.cc = &clientConn{c: gwSide, tr: h.tr, entTypes: map[int32]int32{}, menus: map[int32]int32{}, chat: chat,
		props: []attach.Property{{Name: "textures", Value: "skin", Signature: "sig"}}}
	go func() { // the client: reads everything it is sent
		br := bufio.NewReader(clientSide)
		for {
			p, err := protocol.ReadCompressed(br)
			if err != nil {
				return
			}
			h.mu.Lock()
			h.pkts = append(h.pkts, *p)
			h.mu.Unlock()
		}
	}()
	go h.readWorld(worldSide)
	go func() {
		h.done <- play(cfg, bufio.NewReader(gwSide), h.cc, gwWorld, "Legion", "00000000-0000-0000-0000-000000000001", nil, welcome, 8, proto)
	}()
	t.Cleanup(func() {
		gwSide.Close()
		clientSide.Close()
		worldSide.Close()
		gwWorld.Close()
	})
	return h
}

func (h *playHarness) readWorld(c net.Conn) {
	for {
		typ, payload, err := attach.ReadFrame(c)
		if err != nil {
			return
		}
		h.frames <- worldFrame{typ, payload}
	}
}

// send writes a world frame to the gateway.
func (h *playHarness) send(typ byte, v any) {
	h.t.Helper()
	if err := attach.WriteJSON(h.world, typ, v); err != nil {
		h.t.Fatal(err)
	}
}

// waitPacket waits for the n-th (0-based) client packet with the canonical
// play id, returning its body.
func (h *playHarness) waitPacket(canonID int32, body0 []byte, n int) []byte {
	h.t.Helper()
	want, _, _ := h.tr.Clientbound(protocol.StatePlay, canonID, body0)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		h.mu.Lock()
		seen := 0
		for _, p := range h.pkts {
			if p.ID == want {
				if seen == n {
					h.mu.Unlock()
					return p.Data
				}
				seen++
			}
		}
		h.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	h.t.Fatalf("no packet 0x%x (#%d)", want, n)
	return nil
}

// waitFrame waits for a frame of the type from the world connection.
func (h *playHarness) waitFrame(frames chan worldFrame, typ byte) []byte {
	h.t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case f := <-frames:
			if f.typ == typ {
				return f.payload
			}
		case <-timeout:
			h.t.Fatalf("no world frame 0x%x", typ)
			return nil
		}
	}
}

// update_recipes through the session: the Welcome's recipes at join, the
// MsgUpdateRecipes frame's after a /reload.
func TestPlayUpdateRecipesFromWorld(t *testing.T) {
	stone := protocol.CanonicalItem("stone")
	mine := &attach.UpdateRecipes{ItemSets: []attach.RecipePropertySet{{Key: "minecraft:furnace_input", Items: []int32{stone}}}}
	h := startPlay(t, 777, Config{}, attach.Welcome{EID: 1, Sections: 24, Recipes: mine}, nil)
	got := h.waitPacket(render770.IDUpdateRecipes, nil, 0)
	if want := render770.UpdateRecipesFrom(777, mine).Body; !bytes.Equal(got, want) {
		t.Fatalf("join update_recipes differs from the world's")
	}
	if bytes.Equal(got, render770.UpdateRecipes(777).Body) {
		t.Fatal("the join sent the generated table")
	}
	h.send(attach.MsgUpdateRecipes, attach.UpdateRecipes{Stonecutter: []attach.StonecutterRecipe{}})
	got = h.waitPacket(render770.IDUpdateRecipes, nil, 1)
	if want := render770.UpdateRecipesFrom(777, &attach.UpdateRecipes{Stonecutter: []attach.StonecutterRecipe{}}).Body; !bytes.Equal(got, want) {
		t.Fatalf("reload update_recipes: %x", got)
	}
}

// A shard handover (MsgRehome): the destination is resumed with the full
// Hello (skin, address), told the player's chat session — the client's
// game listener carries on, so the gateway's chain, last-seen window and
// signature cache must be the same objects after the swap.
func TestRehomeKeepsSecureChat(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	dest := make(chan worldFrame, 256)
	hello := make(chan attach.Hello, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		typ, payload, err := attach.ReadFrame(c)
		if err != nil || typ != attach.MsgHello {
			c.Close()
			return
		}
		var hl attach.Hello
		json.Unmarshal(payload, &hl)
		hello <- hl
		attach.WriteJSON(c, attach.MsgWelcome, attach.Welcome{EID: 1, Sections: 24, Spawn: attach.Pos{X: 600, Y: 70, Z: 8}})
		for {
			typ, payload, err := attach.ReadFrame(c)
			if err != nil {
				return
			}
			dest <- worldFrame{typ, payload}
		}
	}()

	chat := newChatState([16]byte{1}, true, nil)
	sess := attach.ChatSession{SessionID: [16]byte{9}, ExpiresAt: 1 << 41, Key: []byte{1, 2, 3}, KeySig: []byte{4}}
	chat.session = &sess
	chat.chain = &chatChain{sessionID: sess.SessionID, expiresAt: sess.ExpiresAt}
	chat.lastSeen.addPending([]byte("a signature"))
	chat.nextIndex = 7
	chain, lastSeen := chat.chain, chat.lastSeen

	cfg := Config{WorldPattern: "127.0.0.1:%d", ClientIP: "203.0.113.5"}
	h := startPlay(t, 777, cfg, attach.Welcome{EID: 1, Sections: 24}, chat)
	h.waitFrame(h.frames, attach.MsgWant) // the join's window
	h.send(attach.MsgRehome, attach.Rehome{DestSID: int32(port), Token: "mig-1"})

	select {
	case hl := <-hello:
		if hl.Purpose != "resume" || hl.ResumeToken != "mig-1" || hl.IP != "203.0.113.5" ||
			len(hl.Props) != 1 || hl.Props[0].Signature != "sig" {
			t.Fatalf("resume hello %+v", hl)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no resume")
	}
	var got attach.ChatSession
	if err := json.Unmarshal(h.waitFrame(dest, attach.MsgChatSession), &got); err != nil {
		t.Fatal(err)
	}
	if got.SessionID != sess.SessionID || !bytes.Equal(got.Key, sess.Key) || got.ExpiresAt != sess.ExpiresAt {
		t.Fatalf("destination told %+v", got)
	}
	h.waitFrame(dest, attach.MsgWant)
	chat.mu.Lock()
	defer chat.mu.Unlock()
	if chat.chain != chain || chat.lastSeen != lastSeen || chat.nextIndex != 7 || chat.session == nil {
		t.Fatal("the handover reset the secure chat state")
	}
}

// A configuration-phase custom click action (a dialog shown during
// configuration) reaches the world, as in play.
func TestConfigurationCustomClickForwarded(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	server.SetDeadline(time.Now().Add(20 * time.Second))
	client.SetDeadline(time.Now().Add(20 * time.Second))
	tr := protocol.TranslatorFor(777)
	clicks := make(chan attach.CustomClickAction, 1)
	done := make(chan error, 1)
	go func() {
		_, err := configure(Config{}, bufio.NewReader(server), server, tr, 384, 777, protocol.ConfigExtras{}, func(e attach.CustomClickAction) { clicks <- e })
		done <- err
	}()
	cbr := bufio.NewReader(client)
	readCfg(t, cbr) // known packs
	// custom_click_action: identifier, then the payload as a length-prefixed
	// optional tag (here a compound {a:1b}).
	tag := protocol.NBTEnd(protocol.NBTByte(protocol.NBTRoot(), "a", 1))
	body := protocol.AppendString(nil, "tachyne:accept")
	body = protocol.AppendVarInt(body, int32(len(tag)))
	body = append(body, tag...)
	if err := protocol.WriteCompressed(client, render770.SIDConfigCustomClickAction26x, body, compressThreshold); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-clicks:
		if e.ID != "tachyne:accept" || !bytes.Contains(e.Payload, []byte(`"a"`)) {
			t.Fatalf("click %+v (%s)", e, e.Payload)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the click was not forwarded")
	}
	// The phase carries on to its end.
	go func() {
		for {
			if _, err := protocol.ReadCompressed(cbr); err != nil {
				return
			}
		}
	}()
	protocol.WriteCompressed(client, cfgServerKnownPacks, protocol.AppendVarInt(nil, 0), compressThreshold)
	time.Sleep(50 * time.Millisecond)
	protocol.WriteCompressed(client, cfgServerFinish, nil, compressThreshold)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
