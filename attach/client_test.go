package attach

import (
	"encoding/json"
	"errors"
	"net"
	"testing"
)

// fakeWorld accepts one attach session: reads the Hello, replies with a
// Welcome (or refuses), and hands the received Hello to the test.
func fakeWorld(t *testing.T, accept bool) (addr string, got chan Hello) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	got = make(chan Hello, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		typ, payload, err := ReadFrame(c)
		if err != nil || typ != MsgHello {
			return
		}
		var h Hello
		if json.Unmarshal(payload, &h) != nil {
			return
		}
		got <- h
		if !accept {
			WriteJSON(c, MsgBye, Bye{Reason: "no"})
			return
		}
		WriteJSON(c, MsgWelcome, Welcome{EID: 42, SID: 1})
	}()
	return ln.Addr().String(), got
}

func TestDialSession(t *testing.T) {
	addr, got := fakeWorld(t, true)
	hello := Hello{Token: "tok", Gateway: "gw-test/0", Name: "wesley",
		Purpose: "resume", ResumeToken: "0.1", Edition: "java"}
	c, wel, err := DialSession(addr, hello)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if wel.EID != 42 || wel.SID != 1 {
		t.Errorf("welcome = %+v", wel)
	}
	h := <-got
	if h.Token != "tok" || h.Gateway != "gw-test/0" || h.Purpose != "resume" || h.ResumeToken != "0.1" {
		t.Errorf("hello not delivered verbatim: %+v", h)
	}
}

func TestDialSessionRefused(t *testing.T) {
	addr, _ := fakeWorld(t, false)
	if _, _, err := DialSession(addr, Hello{}); !errors.Is(err, ErrRefused) {
		t.Errorf("refusal not marked ErrRefused: %v", err)
	}
	// A dead address is a plain connect failure, NOT ErrRefused.
	if _, _, err := DialSession("127.0.0.1:1", Hello{}); err == nil || errors.Is(err, ErrRefused) {
		t.Errorf("connect failure mis-typed: %v", err)
	}
}

func TestBackendSwap(t *testing.T) {
	a1, b1 := net.Pipe()
	a2, b2 := net.Pipe()
	defer b1.Close()
	defer b2.Close()
	bk := NewBackend(a1)
	if bk.Get() != a1 {
		t.Fatal("Get before swap")
	}
	bk.Swap(a2)
	if bk.Get() != a2 {
		t.Fatal("Get after swap")
	}
	// The old conn was closed by Swap.
	if _, err := a1.Write([]byte{0}); err == nil {
		t.Error("old conn still open after swap")
	}
	// Write goes to the NEW conn.
	done := make(chan error, 1)
	go func() { done <- bk.Write(MsgPing, []byte{1, 2, 3, 4, 5, 6, 7, 8}) }()
	typ, payload, err := ReadFrame(b2)
	if err != nil || typ != MsgPing {
		t.Fatalf("frame after swap: typ=%#x err=%v", typ, err)
	}
	_ = payload
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
