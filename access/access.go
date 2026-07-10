// Package access is the gateway-side client for tachyne-access, the
// cluster's authorization service. The contract is FAIL CLOSED: if the
// service is unreachable or errors, Check returns a deny verdict — a gateway
// that cannot get a verdict must not admit anyone. Verdicts are cached
// briefly so access restarts don't cause login blips and repeat dials don't
// hammer the service; live revocation (kicking mid-session) arrives with the
// M2 event bus.
package access

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// Request describes a login attempt (mirrors the service's /v1/check body).
type Request struct {
	Name    string `json:"name"`
	UUID    string `json:"uuid"`
	IP      string `json:"ip"`
	Edition string `json:"edition"`
}

// Verdict is the service's decision.
type Verdict struct {
	Allow  bool     `json:"allow"`
	Reason string   `json:"reason"`
	Roles  []string `json:"roles"`
}

// failClosed is what a player sees when the service can't be reached.
var failClosed = Verdict{Allow: false, Reason: "Unable to verify access right now — please try again shortly."}

// Client checks logins against tachyne-access.
type Client struct {
	baseURL string
	token   string
	ttl     time.Duration
	http    *http.Client

	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	v     Verdict
	until time.Time
}

// New builds a client. ttl <= 0 defaults to 30s.
func New(baseURL, token string, ttl time.Duration) *Client {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &Client{
		baseURL: baseURL,
		token:   token,
		ttl:     ttl,
		http:    &http.Client{Timeout: 5 * time.Second},
		cache:   map[string]cached{},
	}
}

// Check returns the verdict for req, from cache when fresh. Never errors:
// failures ARE a verdict (deny).
func (c *Client) Check(ctx context.Context, req Request) Verdict {
	key := req.Name + "|" + req.UUID + "|" + req.IP
	now := time.Now()

	c.mu.Lock()
	if e, ok := c.cache[key]; ok && now.Before(e.until) {
		c.mu.Unlock()
		return e.v
	}
	c.mu.Unlock()

	v, err := c.fetch(ctx, req)
	if err != nil {
		log.Printf("access check %q: %v (failing closed)", req.Name, err)
		return failClosed
	}

	c.mu.Lock()
	if len(c.cache) > 4096 { // crude bound; a gateway sees few distinct logins
		c.cache = map[string]cached{}
	}
	c.cache[key] = cached{v: v, until: now.Add(c.ttl)}
	c.mu.Unlock()
	return v
}

func (c *Client) fetch(ctx context.Context, req Request) (Verdict, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return Verdict{}, err
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/check", bytes.NewReader(body))
	if err != nil {
		return Verdict{}, err
	}
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.http.Do(hreq)
	if err != nil {
		return Verdict{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Verdict{}, fmt.Errorf("status %d", resp.StatusCode)
	}
	var v Verdict
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return Verdict{}, err
	}
	return v, nil
}
