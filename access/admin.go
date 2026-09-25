package access

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Admin calls: what the world's operator commands change in the policy
// store (/op, /deop, /ban, /ban-ip, /pardon, /pardon-ip, /banlist). Each
// carries the acting player as X-Actor, so the audit log says who.

// Ban is one ban row.
type Ban struct {
	ID        int64      `json:"id"`
	Kind      string     `json:"kind"` // "uuid" | "name" | "ip"
	Value     string     `json:"value"`
	Reason    string     `json:"reason"`
	IssuedBy  string     `json:"issued_by"`
	IssuedAt  time.Time  `json:"issued_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Revoked   bool       `json:"revoked"`
}

func (c *Client) admin(ctx context.Context, method, path, actor string, body, out any) error {
	var rd *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if actor != "" {
		req.Header.Set("X-Actor", actor)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("access: %s %s: %s", method, path, resp.Status)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// AddBan bans a uuid, name or ip.
func (c *Client) AddBan(ctx context.Context, actor, kind, value, reason string) (int64, error) {
	var r struct {
		ID int64 `json:"id"`
	}
	err := c.admin(ctx, http.MethodPost, "/v1/bans", actor,
		map[string]any{"kind": kind, "value": value, "reason": reason}, &r)
	return r.ID, err
}

// Bans lists the bans in force.
func (c *Client) Bans(ctx context.Context) ([]Ban, error) {
	var out []Ban
	err := c.admin(ctx, http.MethodGet, "/v1/bans", "", nil, &out)
	return out, err
}

// RevokeBan lifts one ban.
func (c *Client) RevokeBan(ctx context.Context, actor string, id int64) error {
	return c.admin(ctx, http.MethodDelete, "/v1/bans/"+strconv.FormatInt(id, 10), actor, nil, nil)
}

// Grant gives a principal a role; Revoke takes it away.
func (c *Client) Grant(ctx context.Context, actor, uuid, role string) error {
	return c.admin(ctx, http.MethodPost, "/v1/principals/"+url.PathEscape(uuid)+"/roles", actor,
		map[string]string{"role": role}, nil)
}

func (c *Client) Revoke(ctx context.Context, actor, uuid, role string) error {
	return c.admin(ctx, http.MethodDelete, "/v1/principals/"+url.PathEscape(uuid)+"/roles/"+url.PathEscape(role), actor, nil, nil)
}
