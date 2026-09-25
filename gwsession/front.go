package gwsession

import (
	"bufio"
	"context"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/tachyne/tachyne-common/access"
	"github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
	"github.com/tachyne/tachyne-common/proxyproto"
)

// front.go is THE shared Java gateway front door: listener, handshake parse,
// status ping, login (version gate + tachyne-access check), then the shared
// session pipeline (Run). Extracted 2026-07-10 from the per-repo gateway.go /
// login.go / status.go — a gateway repo is now just a main.go that fills in
// Server and calls Run(ctx).

// Handshake intents.
const (
	intentStatus   = 1
	intentLogin    = 2
	intentTransfer = 3 // client redirected here by a Transfer packet; treat as login
)

// Login-state packet IDs.
const (
	loginPktDisconnect = 0x00 // clientbound: disconnect with a JSON reason
	loginPktStart      = 0x00 // serverbound: player name + uuid
)

// Status-state packet IDs (identical in both directions).
const (
	statusPktResponse = 0x00 // clientbound: JSON status / serverbound: request
	statusPktPong     = 0x01 // clientbound: pong / serverbound: ping
)

// connTimeout bounds each handshake/status/login exchange.
const connTimeout = 30 * time.Second

// Server is one Java gateway instance: the pinned front door over the shared
// session pipeline. A gateway repo constructs one from env + its version
// pinning and calls Run.
type Server struct {
	Listen       string         // client-facing listen address, e.g. ":25565"
	Backend      string         // world pod attach address (login shard); "" = under-construction farewell
	WorldPattern string         // dial pattern for a neighbour shard on handover (%d = sid)
	AttachToken  string         // shared secret for the world attach protocol
	MOTD         string         // server-list description
	ResourcePack *ResourcePack  // server resource pack pushed in configuration (nil = none)
	SID          int            // this gateway's ordinal (StatefulSet pod name)
	Access       *access.Client // authorization service; nil = open mode (dev only)
	Auth         *Authenticator // online mode (Mojang session auth); nil = offline, as vanilla's online-mode=false

	Name        string // gateway name stamped into the attach Hello, e.g. "gw-java-770"
	VersionName string // human-readable release name, e.g. "1.21.5"
	Proto       int32  // pinned protocol: config-phase composition + status advertisement
	MinProto    int32  // accepted client protocol range (a single-version gateway sets Min == Max)
	MaxProto    int32
	ViewCap     int32 // max honored render distance in chunks (0 = default; capped at the attach limit 32)

	roster     attach.StatusCache // cached server-list roster, asked of the world
	rosterOnce sync.Once
	rosterWarn sync.Once
}

// emptyUUID is the all-zero id the sample entries carry.
const emptyUUID = "00000000-0000-0000-0000-000000000000"

// sessionConfig parameterizes the shared session pipeline with this gateway's
// pinning.
func (s *Server) sessionConfig() Config {
	return Config{
		Name: s.Name, Proto: s.Proto,
		Backend: s.Backend, WorldPattern: s.WorldPattern,
		AttachToken: s.AttachToken, SID: s.SID,
		ViewCap:      s.ViewCap,
		Online:       s.Auth != nil,
		MOTD:         s.MOTD,
		ResourcePack: s.ResourcePack,
	}
}

// Run listens on s.Listen and serves until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.Listen)
	if err != nil {
		return err
	}
	go func() { <-ctx.Done(); ln.Close() }()
	err = s.Serve(ln)
	if ctx.Err() != nil {
		return nil // clean shutdown
	}
	return err
}

// Serve accepts connections on ln until it is closed.
func (s *Server) Serve(ln net.Listener) error {
	for {
		c, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return err
			}
			log.Printf("accept: %v", err)
			continue
		}
		go s.handle(c)
	}
}

// handshake is the first packet of every connection.
type handshake struct {
	proto  int32
	addr   string // server address the client dialed
	port   uint16
	intent int32
}

func (s *Server) handle(c net.Conn) {
	defer c.Close()
	c.SetDeadline(time.Now().Add(connTimeout))
	br := bufio.NewReader(c)
	// A clean close before the first byte is a TCP health probe (kubelet
	// readiness/liveness) or a port scan — not worth a log line.
	if _, err := br.Peek(1); err != nil {
		return
	}
	remote := c.RemoteAddr().String()
	if real, err := proxyproto.ReadV1(br); err != nil {
		return
	} else if real != "" {
		remote = real // the ingress prepended the client's true address
	}
	hs, err := readHandshake(br)
	if err != nil {
		log.Printf("%s: handshake: %v", c.RemoteAddr(), err)
		return
	}
	switch hs.intent {
	case intentStatus:
		s.status(br, c, hs.proto)
	case intentLogin, intentTransfer:
		s.login(br, c, hs, remote)
	default:
		log.Printf("%s: unknown handshake intent %d", c.RemoteAddr(), hs.intent)
	}
}

func readHandshake(br *bufio.Reader) (*handshake, error) {
	pkt, err := protocol.ReadPacket(br)
	if err != nil {
		return nil, err
	}
	if pkt.ID != 0x00 {
		return nil, fmt.Errorf("expected handshake (0x00), got packet %#x", pkt.ID)
	}
	body := pkt.Body()
	var hs handshake
	if hs.proto, err = protocol.ReadVarInt(body); err != nil {
		return nil, err
	}
	if hs.addr, err = protocol.ReadString(body); err != nil {
		return nil, err
	}
	hi, err1 := body.ReadByte()
	lo, err2 := body.ReadByte()
	if err1 != nil || err2 != nil {
		return nil, errors.New("short handshake")
	}
	hs.port = uint16(hi)<<8 | uint16(lo)
	if hs.intent, err = protocol.ReadVarInt(body); err != nil {
		return nil, err
	}
	return &hs, nil
}

// statusJSON is the server-list response payload.
type statusJSON struct {
	Version struct {
		Name     string `json:"name"`
		Protocol int32  `json:"protocol"`
	} `json:"version"`
	Players struct {
		Max    int          `json:"max"`
		Online int          `json:"online"`
		Sample []sampleName `json:"sample,omitempty"`
	} `json:"players"`
	Description struct {
		Text string `json:"text"`
	} `json:"description"`
	EnforcesSecureChat bool `json:"enforcesSecureChat"`
}

// sampleName is one entry of the list's hover card. The id is required by
// the schema; the client only draws the name, so a zero uuid is honest for a
// roster the gateway did not authenticate itself.
type sampleName struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

// statusMaxPlayers is the slot count advertised to the server list. Nothing
// enforces it — the world has no join cap — so it is a display figure.
const statusMaxPlayers = 100

// status serves the server-list ping. A client inside the accepted range is
// answered with its own protocol (a multi-version gateway is compatible
// with every version it serves); one outside it sees the gateway's own,
// and so an incompatible entry in the list, which is honest.
func (s *Server) status(br *bufio.Reader, c net.Conn, clientProto int32) {
	for {
		c.SetDeadline(time.Now().Add(connTimeout))
		pkt, err := protocol.ReadPacket(br)
		if err != nil {
			return
		}
		switch pkt.ID {
		case statusPktResponse:
			var st statusJSON
			st.Version.Name = s.VersionName
			st.Version.Protocol = s.Proto
			if clientProto >= s.MinProto && clientProto <= s.MaxProto {
				st.Version.Protocol = clientProto
			}
			// The roster comes from the world: this gateway sees only the
			// clients pinned to its own protocol range, while the players
			// online may all have arrived through a different one.
			s.rosterOnce.Do(func() {
				s.roster.Backend, s.roster.Token, s.roster.Gateway = s.Backend, s.AttachToken, s.Name
			})
			ros, live := s.roster.Get()
			if !live {
				s.rosterWarn.Do(func() { log.Printf("status: world roster unavailable, advertising the last known count") })
			}
			st.Players.Max = statusMaxPlayers
			if ros.Max > 0 {
				st.Players.Max = ros.Max
			}
			st.Players.Online = ros.Online
			for _, pl := range ros.Sample {
				id := pl.ID
				if id == "" {
					id = emptyUUID
				}
				st.Players.Sample = append(st.Players.Sample, sampleName{Name: pl.Name, ID: id})
			}
			st.Description.Text = s.MOTD
			st.EnforcesSecureChat = s.Auth != nil // no "chat not verified" mark in the server list
			payload, err := json.Marshal(st)
			if err != nil {
				return
			}
			if protocol.WritePacket(c, statusPktResponse, protocol.AppendString(nil, string(payload))) != nil {
				return
			}
		case statusPktPong:
			// Echo the client's 8-byte timestamp payload verbatim.
			protocol.WritePacket(c, statusPktPong, pkt.Data)
			return // ping is always the final exchange
		default:
			return
		}
	}
}

// login gates the version and the authorization, then hands the connection to
// the shared session pipeline.
func (s *Server) login(br *bufio.Reader, c net.Conn, hs *handshake, remote string) {
	if hs.proto < s.MinProto || hs.proto > s.MaxProto {
		accepted := fmt.Sprintf("protocol %d", s.MinProto)
		if s.MinProto != s.MaxProto {
			accepted = fmt.Sprintf("protocols %d-%d", s.MinProto, s.MaxProto)
		}
		loginDisconnect(c, fmt.Sprintf(
			"This is the Minecraft %s gateway (%s); your client speaks protocol %d.",
			s.VersionName, accepted, hs.proto))
		return
	}

	// Login Start: player name + uuid (layout stable since 1.20.2).
	pkt, err := protocol.ReadPacket(br)
	if err != nil || pkt.ID != loginPktStart {
		return
	}
	name, err := protocol.ReadString(pkt.Body())
	if err != nil || !validPlayerName(name) {
		return // vanilla: "Invalid characters in username" — a protocol error
	}
	prof := Profile{UUID: offlineUUIDBytes(name), Name: name}
	if s.Auth != nil {
		// Online mode: the account proves itself before anything else is
		// asked of it, and from here on the connection is encrypted.
		ec, ebr, p, err := s.Auth.Login(br, c, name)
		c, br = ec, ebr
		if err != nil {
			var ae *authError
			if errors.As(err, &ae) {
				loginDisconnect(c, ae.reason)
			}
			log.Printf("%s: login %q not authenticated: %v", remote, name, err)
			return
		}
		prof = p
		log.Printf("%s: UUID of player %s is %s", remote, prof.Name, prof.UUIDString())
	}
	name, uuid, uuidStr := prof.Name, prof.UUID, prof.UUIDString()

	// Authorization gate (tachyne-access). Fail closed: no verdict, no entry.
	roles := []string{}
	if s.Access != nil {
		ip, _, _ := net.SplitHostPort(remote)
		v := s.Access.Check(context.Background(), access.Request{
			Name: name, UUID: uuidStr, IP: ip, Edition: "java",
		})
		if !v.Allow {
			log.Printf("%s: login %q DENIED: %s", remote, name, v.Reason)
			loginDisconnect(c, v.Reason)
			return
		}
		roles = v.Roles
	}

	if s.Backend == "" { // no world wired (dev / staging placeholder)
		loginDisconnect(c, fmt.Sprintf(
			"tachyne: the %s world link is under construction — check back soon, %s.",
			s.VersionName, name))
		return
	}
	log.Printf("%s: login %q allowed (roles %v) — attaching to %s", remote, name, roles, s.Backend)
	if err := Run(s.sessionConfig(), br, c, name, uuid, uuidStr, roles, prof.Props, hs.proto); err != nil {
		log.Printf("%s: session %q ended: %v", c.RemoteAddr(), name, err)
	}
}

// validPlayerName is StringUtil.isValidPlayerName: at most 16 characters,
// none of them a control character, a space or beyond ASCII.
func validPlayerName(name string) bool {
	if name == "" || len(name) > 16 {
		return false
	}
	for i := 0; i < len(name); i++ {
		if name[i] <= 32 || name[i] >= 127 {
			return false
		}
	}
	return true
}

// offlineUUIDBytes is the vanilla offline-mode UUIDv3 of "OfflinePlayer:<name>".
func offlineUUIDBytes(name string) [16]byte {
	sum := md5.Sum([]byte("OfflinePlayer:" + name))
	sum[6] = (sum[6] & 0x0f) | 0x30 // version 3 (name-based, MD5)
	sum[8] = (sum[8] & 0x3f) | 0x80 // IETF variant
	return sum
}

// OfflineUUID derives the offline-mode UUID for a player name, exactly as
// vanilla does, formatted with dashes (how tachyne-access stores principals).
func OfflineUUID(name string) string {
	sum := offlineUUIDBytes(name)
	return fmt.Sprintf("%x-%x-%x-%x-%x", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}
