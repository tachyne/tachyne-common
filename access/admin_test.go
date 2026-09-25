package access

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAdminCalls(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.Path+" "+r.Header.Get("X-Actor")+" "+r.Header.Get("Authorization"))
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/bans":
			json.NewEncoder(w).Encode(map[string]int64{"id": 7})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/bans":
			json.NewEncoder(w).Encode([]Ban{{ID: 7, Kind: "ip", Value: "10.0.0.1", Reason: "spam"}})
		}
	}))
	defer srv.Close()
	c := New(srv.URL, "tok", time.Second)
	ctx := context.Background()
	if id, err := c.AddBan(ctx, "alice", "ip", "10.0.0.1", "spam"); err != nil || id != 7 {
		t.Fatalf("AddBan %d %v", id, err)
	}
	bans, err := c.Bans(ctx)
	if err != nil || len(bans) != 1 || bans[0].Value != "10.0.0.1" {
		t.Fatalf("Bans %+v %v", bans, err)
	}
	if err := c.Grant(ctx, "alice", "u-1", "op"); err != nil {
		t.Fatal(err)
	}
	if got[0] != "POST /v1/bans alice Bearer tok" || got[2] != "POST /v1/principals/u-1/roles alice Bearer tok" {
		t.Fatalf("requests %q", got)
	}
}
