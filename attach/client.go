package attach

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

// Client-side session machinery shared by every gateway (Java and Bedrock):
// dialing a world pod (login or handover resume) and the swappable backend
// connection that makes a shard crossing invisible to the client. The client
// transport (raw Java frames, gophertunnel) differs per gateway; this attach
// side is identical, so it lives here — TODO.md dedup Tier 2.

// ErrRefused reports that the world pod answered but refused or failed the
// session (bad token, missing resume state, malformed welcome) — as opposed
// to a plain connect failure (returned as the underlying net error).
var ErrRefused = errors.New("attach: session refused")

// DialSession opens an attach session on a world pod: dial, send the Hello,
// read the Welcome (which MUST be the world's first frame). Used both for a
// login (Purpose empty) and a handover resume (Purpose "resume" + ResumeToken)
// — the caller composes the Hello. On error the connection is closed; connect
// failures return the net error, post-connect failures wrap ErrRefused.
func DialSession(addr string, hello Hello) (net.Conn, Welcome, error) {
	c, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, Welcome{}, fmt.Errorf("attach dial %s: %w", addr, err)
	}
	if err := WriteJSON(c, MsgHello, hello); err != nil {
		c.Close()
		return nil, Welcome{}, fmt.Errorf("%w: hello: %v", ErrRefused, err)
	}
	c.SetReadDeadline(time.Now().Add(10 * time.Second))
	typ, payload, err := ReadFrame(c)
	if err != nil || typ != MsgWelcome {
		c.Close()
		return nil, Welcome{}, fmt.Errorf("%w: welcome: typ=%#x err=%v", ErrRefused, typ, err)
	}
	c.SetReadDeadline(time.Time{})
	var wel Welcome
	if err := json.Unmarshal(payload, &wel); err != nil {
		c.Close()
		return nil, Welcome{}, fmt.Errorf("%w: welcome decode: %v", ErrRefused, err)
	}
	return c, wel, nil
}

// Backend is a gateway session's CURRENT world-pod connection. It is
// swappable: on a shard handover (MsgRehome) the world→client reader dials
// the destination pod and Swaps the conn under this lock, while the
// client→world writer keeps sending through Write — so the client socket
// never drops. Write holds the lock across the frame write, so a swap
// serialises cleanly behind any in-flight write (brief; frames are small).
type Backend struct {
	mu   sync.Mutex
	conn net.Conn
}

// NewBackend wraps the session's initial world connection.
func NewBackend(c net.Conn) *Backend { return &Backend{conn: c} }

// Get returns the current world connection (for reads on the world→client
// pump; re-fetch after any error — a swap may have replaced it).
func (b *Backend) Get() net.Conn { b.mu.Lock(); defer b.mu.Unlock(); return b.conn }

// Write sends one JSON frame to the current world connection.
func (b *Backend) Write(typ byte, v any) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return WriteJSON(b.conn, typ, v)
}

// Swap installs a new world connection and closes the old one (after the
// pointer swap, so no in-flight write targets a closed conn).
func (b *Backend) Swap(nw net.Conn) {
	b.mu.Lock()
	old := b.conn
	b.conn = nw
	b.mu.Unlock()
	old.Close()
}
