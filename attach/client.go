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

// QueryStatus asks a world pod for the server-list roster: dial, Hello with
// Purpose "status", read the one Status frame, close. It opens no session and
// gives the world no player, so it is safe to call from the status path where
// nobody has logged in. The deadlines are short — a server-list ping must not
// hang on a world that is busy.
func QueryStatus(addr, token, gateway string) (Status, error) {
	c, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return Status{}, fmt.Errorf("attach dial %s: %w", addr, err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(3 * time.Second))
	if err := WriteJSON(c, MsgHello, Hello{Token: token, Gateway: gateway, Purpose: "status"}); err != nil {
		return Status{}, fmt.Errorf("%w: hello: %v", ErrRefused, err)
	}
	typ, payload, err := ReadFrame(c)
	if err != nil || typ != MsgStatus {
		return Status{}, fmt.Errorf("%w: status: typ=%#x err=%v", ErrRefused, typ, err)
	}
	var st Status
	if err := json.Unmarshal(payload, &st); err != nil {
		return Status{}, fmt.Errorf("%w: status decode: %v", ErrRefused, err)
	}
	return st, nil
}

// StatusCache is a gateway's view of the world's roster, refreshed on demand
// and shared by every server-list ping that gateway answers. A server list
// re-pings every few seconds and a client may hold dozens of entries, so the
// world is asked at most once per StatusTTL rather than once per ping.
type StatusCache struct {
	Backend string // world pod attach address
	Token   string // attach token
	Gateway string // this gateway's name, for the world's logs

	mu   sync.Mutex
	at   time.Time
	st   Status
	have bool
}

// StatusTTL bounds how stale an advertised roster may be.
const StatusTTL = 3 * time.Second

// Get returns the roster, refreshing it from the world when stale. A world
// that cannot be reached leaves the last good answer standing — and, if
// there has never been one, reports an empty server rather than failing the
// ping, since a server-list entry that will not draw is worse than a stale
// count. The second return says whether the answer is a live one.
func (s *StatusCache) Get() (Status, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.have && time.Since(s.at) < StatusTTL {
		return s.st, true
	}
	st, err := QueryStatus(s.Backend, s.Token, s.Gateway)
	if err != nil {
		return s.st, false
	}
	s.st, s.at, s.have = st, time.Now(), true
	return st, true
}
