package attach

import (
	"encoding/json"
	"net"
	"sync/atomic"
	"testing"
)

// statusWorld answers one status query per connection with st.
func statusWorld(t *testing.T, st Status, calls *int64) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				typ, payload, err := ReadFrame(c)
				if err != nil || typ != MsgHello {
					return
				}
				var h Hello
				if json.Unmarshal(payload, &h) != nil || h.Purpose != "status" {
					return
				}
				atomic.AddInt64(calls, 1)
				WriteJSON(c, MsgStatus, st)
			}()
		}
	}()
	return ln.Addr().String()
}

// A gateway asks the world for the roster it cannot know itself.
func TestQueryStatus(t *testing.T) {
	var calls int64
	want := Status{Online: 2, Max: 100, Sample: []StatusPlayer{{Name: "LegionZA", ID: "x"}}}
	addr := statusWorld(t, want, &calls)

	got, err := QueryStatus(addr, "tok", "gw-test")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if got.Online != 2 || got.Max != 100 || len(got.Sample) != 1 || got.Sample[0].Name != "LegionZA" {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

// The cache answers repeat pings without re-asking the world, and a world
// that goes away leaves the last good answer standing rather than blanking
// the server-list entry.
func TestStatusCacheHoldsAndSurvivesTheWorld(t *testing.T) {
	var calls int64
	addr := statusWorld(t, Status{Online: 3, Max: 100}, &calls)
	sc := &StatusCache{Backend: addr, Token: "tok", Gateway: "gw-test"}

	for i := 0; i < 5; i++ {
		st, live := sc.Get()
		if !live || st.Online != 3 {
			t.Fatalf("ping %d: %+v live=%v", i, st, live)
		}
	}
	if n := atomic.LoadInt64(&calls); n != 1 {
		t.Errorf("five pings asked the world %d times, want 1", n)
	}

	// The world goes away and the cache goes stale: the count still draws.
	sc.at = sc.at.Add(-2 * StatusTTL)
	sc.Backend = "127.0.0.1:1" // nothing listening
	st, live := sc.Get()
	if live {
		t.Error("an unreachable world should not report a live roster")
	}
	if st.Online != 3 {
		t.Errorf("the last known count is %d, want it held at 3", st.Online)
	}
}

// A cache that has never reached the world reports an empty server rather
// than failing the ping.
func TestStatusCacheColdMiss(t *testing.T) {
	sc := &StatusCache{Backend: "127.0.0.1:1", Token: "tok", Gateway: "gw-test"}
	if st, live := sc.Get(); live || st.Online != 0 {
		t.Errorf("a cold miss gave %+v live=%v, want an empty roster", st, live)
	}
}
