package access

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fakeService(t *testing.T, calls *atomic.Int64) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req Request
		json.NewDecoder(r.Body).Decode(&req)
		v := Verdict{Allow: true, Roles: []string{}}
		if strings.EqualFold(req.Name, "griefer") {
			v = Verdict{Allow: false, Reason: "banned", Roles: []string{}}
		}
		json.NewEncoder(w).Encode(v)
	}))
	t.Cleanup(ts.Close)
	return ts
}

func TestVerdictsAndCache(t *testing.T) {
	var calls atomic.Int64
	ts := fakeService(t, &calls)
	c := New(ts.URL, "tok", time.Minute)
	ctx := context.Background()

	if v := c.Check(ctx, Request{Name: "wesley"}); !v.Allow {
		t.Fatalf("wesley should be allowed: %+v", v)
	}
	if v := c.Check(ctx, Request{Name: "griefer"}); v.Allow || v.Reason != "banned" {
		t.Fatalf("griefer should be denied: %+v", v)
	}
	// Repeat checks hit the cache, not the service.
	before := calls.Load()
	c.Check(ctx, Request{Name: "wesley"})
	c.Check(ctx, Request{Name: "griefer"})
	if calls.Load() != before {
		t.Fatalf("repeat checks should be cached: %d calls before, %d after", before, calls.Load())
	}
	// A different key is a fresh check.
	c.Check(ctx, Request{Name: "wesley", IP: "10.0.0.9"})
	if calls.Load() != before+1 {
		t.Fatalf("distinct key should fetch: %d calls", calls.Load())
	}
}

func TestFailClosed(t *testing.T) {
	c := New("http://127.0.0.1:1", "tok", time.Minute) // nothing listens there
	v := c.Check(context.Background(), Request{Name: "wesley"})
	if v.Allow {
		t.Fatalf("unreachable service must deny: %+v", v)
	}
	if v.Reason == "" {
		t.Fatal("fail-closed deny needs a player-facing reason")
	}
}

func TestBadTokenFailsClosed(t *testing.T) {
	var calls atomic.Int64
	ts := fakeService(t, &calls)
	c := New(ts.URL, "wrong-token", time.Minute)
	if v := c.Check(context.Background(), Request{Name: "wesley"}); v.Allow {
		t.Fatalf("401 from service must deny: %+v", v)
	}
}
