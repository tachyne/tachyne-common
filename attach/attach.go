// Package attach is the tachyne domain attach protocol: how a gateway hands
// a player session to a world pod and streams the world back. It is
// deliberately Minecraft-wire-free — chunks are raw block-state/light arrays
// plus biome names, positions are ABSOLUTE floats, sounds/biomes are names,
// time is a tick counter. Gateways render this into whatever protocol their
// clients speak (render770 + optional translation); world pods never know or
// care. The engine emits these types DIRECTLY as its domain events — there
// is no separate internal event vocabulary.
//
// Framing: every message is  u32be length | u8 type | payload , length
// covering type+payload. Payloads are JSON except Chunk (u16be header length
// + JSON ChunkHeader + zlib body, see EncodeChunk) and Ping/Pong (8 raw
// bytes echoed verbatim).
//
// Session flow:
//
//	gw → w   Hello{token, gateway, name, uuid, roles, edition}
//	w  → gw  Welcome{eid, spawn, time, minY, sections}   (or Bye{reason})
//	                 Welcome MUST be the session's first frame.
//	gw → w   Want{dim, cx, cz, radius}   as the player's view moves;
//	w  → gw  Chunk …                     each not-yet-sent chunk in the window
//	gw → w   Move / Chat / Command / Dig / Place / HeldSlot, and the typed
//	         serverbound actions 0x34-0x3f (UseItem, UseEntity, WindowClick,
//	         Craft, PlayerAction, RespawnReq, CreativeSlot, …)
//	w  → gw  the typed event families (see entities.go: entities 0x0a-0x10,
//	         BlockSet 0x14, Dimension/Teleport 0x15/0x16, BossBar 0x1a,
//	         survival 0x1b-0x1f, items/windows 0x20-0x27, effects 0x28-0x2a,
//	         misc 0x2b-0x33, EntityStatus/Swing 0x40/0x41) + Time every 5s
//	either   Ping/Pong, Bye{reason}
//
// entities.go is the frame-catalog reference (typed structs + per-family
// notes). Design rules: events carry absolute state (viewer-side renderers
// derive deltas, so dropped frames self-heal); one json tag PER field and no
// custom marshalers on embedded structs (a promoted MarshalJSON once
// silently dropped Move.on_ground); opaque []byte fields ("canonical wire
// form") are deliberate scaffolding for payloads not yet worth typing
// (ItemStack.Components, EntityMeta.Meta, Trades.Data, CommandTree.Data,
// ChunkHeader.BEs).
package attach

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// Message types.
const (
	MsgHello   = 0x01 // gw→w JSON Hello
	MsgWelcome = 0x02 // w→gw JSON Welcome
	MsgWant    = 0x03 // gw→w JSON Want
	MsgChunk   = 0x04 // w→gw u16be headerLen | JSON ChunkHeader | zlib body
	MsgMove    = 0x05 // gw→w JSON Move
	MsgTime    = 0x06 // w→gw JSON Time
	MsgPing    = 0x07 // either: 8 opaque bytes
	MsgPong    = 0x08 // either: the same 8 bytes back
	MsgBye     = 0x09 // either: JSON Bye
)

// Sharding frames (0x50 block; the 0x0a–0x43 catalog is in entities.go, and
// 0x44–0x4f are left for ordinary event growth).
const (
	MsgRehome = 0x50 // w→gw JSON Rehome: this player is now owned by another pod
)

// Rehome tells the gateway a player it is rendering has been migrated to a
// different pod (over the world↔world peer link). The gateway keeps the client
// socket open and re-points its home backend to DestSID — promoting the warm
// viewer session it already holds to that neighbour — reconnecting there with
// Hello{Purpose:"resume", ResumeToken: Token}. State does NOT ride this frame;
// it already moved pod→pod. Token == the migration MigID.
type Rehome struct {
	DestSID int32  `json:"dest_sid"`
	Token   string `json:"token"`
}

// World shape (overworld v1; Welcome restates these so a future world can
// differ without a codec change).
const (
	Sections    = 24  // 16-block sections per chunk column
	MinY        = -64 // world floor
	BlocksPerCh = Sections * 4096
)

// MaxFrame bounds one frame; a raw chunk is ~590KB and compresses far below
// this, so anything bigger is a broken peer.
const MaxFrame = 4 << 20

// Pos is a position + look.
//
// NOTE: one json tag PER field, and no custom marshalers. Multi-field
// declarations share one tag (encoding/json then silently drops the
// colliders), and a custom MarshalJSON on an embedded struct is promoted to
// every embedder — Move once lost its on_ground on the wire exactly that way.
type Pos struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Z     float64 `json:"z"`
	Yaw   float32 `json:"yaw"`
	Pitch float32 `json:"pitch"`
}

// Hello opens a session. Token authenticates the gateway to the world;
// identity+roles are the gateway-stamped claims (already authorized via
// tachyne-access — worlds do not re-check).
type Hello struct {
	Token   string   `json:"token"`
	Gateway string   `json:"gateway"` // origin, e.g. "gw-java-770/0"
	Name    string   `json:"name"`
	UUID    string   `json:"uuid"`
	Roles   []string `json:"roles"`
	Edition string   `json:"edition"`
	// Purpose selects the session role for a sharded world (empty == "" behaves
	// as a normal owning/login session, back-compatible):
	//   ""/"login" — the player's home session on the owning pod
	//   "view"     — a read-only viewer stream from a neighbour pod for seamless
	//                cross-border rendering (no hub player)
	//   "resume"   — re-bind the client to a player already migrated here
	//                pod→pod; ResumeToken correlates the pending state.
	Purpose     string `json:"purpose,omitempty"`
	ResumeToken string `json:"resume_token,omitempty"` // == the migration MigID, on Purpose=="resume"
}

// Welcome accepts the session.
type Welcome struct {
	EID      int32 `json:"eid"` // the session's own entity id in the world
	Spawn    Pos   `json:"spawn"`
	Time     int64 `json:"time"`
	MinY     int   `json:"min_y"`
	Sections int   `json:"sections"`
	// Gamemode is the player's persisted game mode (0 survival, 1 creative,
	// 2 adventure, 3 spectator) so the gateway's join packet renders the right
	// HUD from the first frame instead of hardcoding survival.
	Gamemode int32 `json:"gamemode"`
	// SID/Topo identify the answering pod in a sharded world. The gateway
	// asserts Topo == its own shard.Map.TopoHash() so a mixed-topology cluster
	// fails at session start instead of corrupting a seam. Zero/empty on an
	// unsharded (single-pod) world.
	SID  int32  `json:"sid,omitempty"`
	Topo string `json:"topo,omitempty"`
}

// Want declares the chunk view the gateway needs.
type Want struct {
	CX     int32 `json:"cx"`
	CZ     int32 `json:"cz"`
	Radius int32 `json:"radius"`
	Dim    int32 `json:"dim"` // 0 overworld, 1 nether, 2 end
	// Force re-sends every chunk in the window even if already sent this
	// session — the /refresh fix for client-side render loss.
	Force bool `json:"force,omitempty"`
}

// Move is a player movement report.
type Move struct {
	Pos
	OnGround bool `json:"on_ground"`
}

// Time is the world clock (ticks; day = 24000).
type Time struct {
	Time int64 `json:"time"`
	// Age is the world age in ticks (drives no visuals but vanilla sends it).
	// Zero = unknown; renderers fall back to Time.
	Age int64 `json:"age,omitempty"`
}

// Bye closes a session with a player-visible reason.
type Bye struct {
	Reason string `json:"reason"`
}

// ChunkHeader describes one chunk; the binary body carries the arrays.
type ChunkHeader struct {
	CX     int32    `json:"cx"`
	CZ     int32    `json:"cz"`
	Dim    int32    `json:"dim"`
	Biomes []string `json:"biomes"` // one name per section, bottom→top
	// BEs is the chunk's block-entity section in canonical wire form (count +
	// entries) — scaffolding like MsgRaw; empty means none.
	BEs []byte `json:"bes,omitempty"`
}

// ChunkBody is the domain form of one chunk column. Index within a section:
// y*256 + z*16 + x; sections bottom→top.
type ChunkBody struct {
	BlockStates []uint32 // BlocksPerCh
	Heightmap   []int16  // 256, absolute world Y of highest motion-blocking block
	SkyLight    []uint8  // BlocksPerCh, 0-15
	BlockLight  []uint8  // BlocksPerCh, 0-15
}

// WriteFrame writes one frame.
func WriteFrame(w io.Writer, typ byte, payload []byte) error {
	if len(payload)+1 > MaxFrame {
		return fmt.Errorf("attach: frame too large (%d)", len(payload))
	}
	var hdr [5]byte
	binary.BigEndian.PutUint32(hdr[:4], uint32(len(payload)+1))
	hdr[4] = typ
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

// ReadFrame reads one frame.
func ReadFrame(r io.Reader) (typ byte, payload []byte, err error) {
	var hdr [4]byte
	if _, err = io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n < 1 || n > MaxFrame {
		return 0, nil, fmt.Errorf("attach: bad frame length %d", n)
	}
	buf := make([]byte, n)
	if _, err = io.ReadFull(r, buf); err != nil {
		return 0, nil, err
	}
	return buf[0], buf[1:], nil
}

// WriteJSON frames v as one JSON-payload message.
func WriteJSON(w io.Writer, typ byte, v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return WriteFrame(w, typ, payload)
}

// EncodeChunk frames one chunk (header + zlib-compressed body).
func EncodeChunk(h ChunkHeader, b *ChunkBody) ([]byte, error) {
	if len(b.BlockStates) != BlocksPerCh || len(b.Heightmap) != 256 ||
		len(b.SkyLight) != BlocksPerCh || len(b.BlockLight) != BlocksPerCh {
		return nil, fmt.Errorf("attach: bad chunk body dimensions")
	}
	hdr, err := json.Marshal(h)
	if err != nil {
		return nil, err
	}
	if len(hdr) > 0xffff {
		return nil, fmt.Errorf("attach: chunk header too large")
	}
	var out bytes.Buffer
	var l2 [2]byte
	binary.BigEndian.PutUint16(l2[:], uint16(len(hdr)))
	out.Write(l2[:])
	out.Write(hdr)

	zw := zlib.NewWriter(&out)
	raw := make([]byte, 0, 4*BlocksPerCh)
	for _, s := range b.BlockStates {
		raw = binary.LittleEndian.AppendUint32(raw, s)
	}
	zw.Write(raw)
	hm := make([]byte, 0, 512)
	for _, h := range b.Heightmap {
		hm = binary.LittleEndian.AppendUint16(hm, uint16(h))
	}
	zw.Write(hm)
	zw.Write(b.SkyLight)
	zw.Write(b.BlockLight)
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// DecodeChunk parses a MsgChunk payload.
func DecodeChunk(payload []byte) (ChunkHeader, *ChunkBody, error) {
	var h ChunkHeader
	if len(payload) < 2 {
		return h, nil, fmt.Errorf("attach: short chunk payload")
	}
	hn := int(binary.BigEndian.Uint16(payload[:2]))
	if len(payload) < 2+hn {
		return h, nil, fmt.Errorf("attach: truncated chunk header")
	}
	if err := json.Unmarshal(payload[2:2+hn], &h); err != nil {
		return h, nil, err
	}
	zr, err := zlib.NewReader(bytes.NewReader(payload[2+hn:]))
	if err != nil {
		return h, nil, err
	}
	defer zr.Close()
	const rawLen = 4*BlocksPerCh + 512 + BlocksPerCh + BlocksPerCh
	raw := make([]byte, rawLen)
	if _, err := io.ReadFull(zr, raw); err != nil {
		return h, nil, fmt.Errorf("attach: chunk body: %w", err)
	}
	b := &ChunkBody{
		BlockStates: make([]uint32, BlocksPerCh),
		Heightmap:   make([]int16, 256),
		SkyLight:    raw[4*BlocksPerCh+512 : 4*BlocksPerCh+512+BlocksPerCh],
		BlockLight:  raw[4*BlocksPerCh+512+BlocksPerCh:],
	}
	for i := range b.BlockStates {
		b.BlockStates[i] = binary.LittleEndian.Uint32(raw[i*4:])
	}
	for i := range b.Heightmap {
		b.Heightmap[i] = int16(binary.LittleEndian.Uint16(raw[4*BlocksPerCh+i*2:]))
	}
	return h, b, nil
}
