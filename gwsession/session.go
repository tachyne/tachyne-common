// Package gwsession is THE shared Java gateway session pipeline: login
// success → configuration → the play bridge (world attach frames → rendered
// packets, client packets → typed serverbound frames), including the shard
// handover backend-swap (silent crossing). It was extracted verbatim from
// tachyne-gw-java-770 (whose copy in tachyne-gw-java-776 had already diverged
// subtly twice — see ~/minecraft/TODO.md for the history).
//
// Everything is composed at canonical 770 and translated per connection by
// protocol.TranslatorFor(clientProto) — Identity for a 770 client, packet-id
// renumbering for 771-772, the full chain for 776. Version-specific entity-
// metadata policies (substitution drops, cube-mob index shifts, copper-golem
// serializer fix) are applied unconditionally: each helper self-no-ops at
// versions it doesn't concern, so ONE pipeline serves every pinned gateway.
//
// The per-gateway repos keep only their front door: listener, status ping,
// login/version gate, access check — then hand the connection to Run.
package gwsession

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"sync"
	"sync/atomic"
	"time"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
	"github.com/tachyne/tachyne-common/render770"
)

// Config is the per-gateway parameterization of the shared pipeline — the ONLY
// things that legitimately differ between the pinned Java gateways.
type Config struct {
	Name         string // gateway name stamped into the attach Hello, e.g. "gw-java-770"
	Proto        int32  // protocol the config phase COMPOSES at (registries/tags): the gateway's pinned version
	Backend      string // world pod attach address (login shard)
	WorldPattern string // dial pattern for a neighbour shard on handover (%d = sid)
	AttachToken  string
	SID          int   // this gateway's ordinal (Hello stamp)
	ViewCap      int32 // max honored render distance in chunks; 0 = defaultViewCap
}

// viewCap resolves the deployment's render-distance ceiling: the client's
// slider is honored up to this. Kept well below the engine's hard attach
// limit (32) by default — honoring a maxed 32-slider means a 65×65 = 4225
// chunk window, ~10× a vanilla server's default cap of 10, which is exactly
// the "sluggish flight + long Loading terrain" regression. Deployments with a
// reason (earth-mode vistas) raise it explicitly via TACHYNE_VIEW_CAP.
func (cfg Config) viewCap() int32 {
	switch {
	case cfg.ViewCap <= 0:
		return defaultViewCap
	case cfg.ViewCap > attachMaxRadius:
		return attachMaxRadius
	}
	return cfg.ViewCap
}

// 770 packet IDs the session uses (see minecraft/server play.go for the map).
const (
	loginSuccess        = 0x02
	loginSetCompression = 0x03
	compressThreshold   = 256

	cfgClientKnownPacks    = 0x0e
	cfgClientCustomPayload = 0x01
	cfgClientFeatures      = 0x0c
	cfgClientUpdateTags    = 0x0d
	cfgClientRegistryData  = 0x07
	cfgClientFinish        = 0x03
	cfgServerKnownPacks    = 0x07
	cfgServerFinish        = 0x03
	cfgServerClientInfo    = 0x00 // Client Information (locale, view distance, …)

	playClientLogin            = 0x2b
	playClientGameEvent        = 0x22
	playClientCenterChunk      = 0x57
	playClientChunkData        = 0x27
	playClientChunkBatchStart  = 0x0c // empty body; opens a paced chunk batch
	playClientChunkBatchFinish = 0x0b // VarInt batch size; client replies chunk_batch_received
	playClientSyncPosition     = 0x41
	playClientKeepAlive        = 0x26
	playClientUpdateTime       = 0x6a

	playClientAckDig = 0x04

	playServerChatMessage  = 0x07
	playServerChatCommand  = 0x05
	playServerBlockDig     = 0x27
	playServerHeldItem     = 0x33
	playServerBlockPlace   = 0x3e
	playServerPosition     = 0x1c
	playServerPositionLook = 0x1d
	playServerLook         = 0x1e
	playServerClientInfo   = 0x0c // Client Information (mid-game render-distance change)

	viewRadius      = 6  // default chunk radius when the client hasn't sent Client Information
	defaultViewCap  = 12 // default honored render-distance ceiling (vanilla server default is 10)
	attachMaxRadius = 32 // the world pod's hard attach radius cap — Config.ViewCap can't exceed it
)

// Chunk-batch pacing (vanilla 1.20.2+ flow control): chunks are delivered in
// chunk_batch_start/finished batches and the client acks each batch with its
// desired chunks-per-tick, so a client that is still meshing slows the stream
// instead of drowning. Values mirror vanilla's ChunkSender.
const (
	batchInterval    = 50 * time.Millisecond // one game tick
	startChunksTick  = 9.0                   // rate before the client's first ack
	maxChunksTick    = 64.0                  // per-batch ceiling regardless of ack
	maxUnackedBatch  = 10                    // in-flight batches before we hold
	batchAckDeadline = 2 * time.Second       // no ack for this long → assume a non-batching client, stop holding
)

// Canonical entity-type ids whose metadata needs version-specific surgery (see
// the MsgEntityMeta case): cube mobs get the 26.2 SIZE index shift, the copper
// golem gets its WEATHERING_COPPER_STATE serializer restored.
//
// The id space is canonical 1.21.11 (proto 774) — the values MUST match the
// engine's generated registry (tachyne-world internal/server/entityids_gen.go).
// These once carried 1.21.5-era ids (slime 111, magma_cube 77) after the
// canonical retarget shifted the registry: 111 is a SHEEP in 1.21.11 (its
// meta was silently mis-shifted on 26.2) and 77 a lightning bolt, while real
// magma cubes reached 26.2 clients unshifted — a type-mismatch disconnect the
// moment one spawned in the nether.
const (
	typeSlime       = 117
	typeMagmaCube   = 80
	typeCopperGolem = 28
	typePainting    = 93
	typeItemFrame   = 73
	typeGlowFrame   = 60
)

// clientConn serializes writes to the Minecraft client. tr is the per-
// connection translation chain (Identity for a 770 client; renumbers packet
// ids for 771-772), applied to every clientbound play packet.
type clientConn struct {
	mu sync.Mutex
	c  net.Conn
	tr protocol.Translator
	// entTypes maps entity id → canonical entity-type id, learned from
	// EntityAdd — the key for every version-specific metadata policy
	// (substitution drops for old clients, cube-mob/copper-golem fixups for
	// 26.2): set_entity_data carries no type, so the session must remember it.
	entTypes map[int32]int32
}

func (cc *clientConn) send(id int32, data []byte) error {
	id, data, drop := cc.tr.Clientbound(protocol.StatePlay, id, data)
	if drop {
		return nil
	}
	cc.mu.Lock()
	defer cc.mu.Unlock()
	return protocol.WriteCompressed(cc.c, id, data, compressThreshold)
}

// Run bridges one authorized client to the world over the attach protocol.
// clientProto is the client's negotiated version; the world is always rendered
// as canonical 770 and translated to it per connection.
func Run(cfg Config, br *bufio.Reader, c net.Conn, name string, uuid [16]byte, uuidStr string, roles []string, clientProto int32) error {
	tr := protocol.TranslatorFor(clientProto)
	// Attach to the world FIRST — if there is no world, disconnect at login
	// cleanly instead of mid-join.
	w, welcome, err := attach.DialSession(cfg.Backend, attach.Hello{
		Token: cfg.AttachToken, Gateway: fmt.Sprintf("%s/%d", cfg.Name, cfg.SID),
		Name: name, UUID: uuidStr, Roles: roles, Edition: "java",
	})
	if err != nil {
		if errors.Is(err, attach.ErrRefused) {
			loginDisconnect(c, "The world refused the session.")
		} else {
			loginDisconnect(c, "The world is unreachable right now — please try again shortly.")
		}
		return err
	}
	defer w.Close()

	// Set Compression (uncompressed), then everything runs compressed framing.
	if err := protocol.WritePacket(c, loginSetCompression, protocol.AppendVarInt(nil, compressThreshold)); err != nil {
		return err
	}
	// Login Success → client acks → Configuration → Play.
	ls := append([]byte(nil), uuid[:]...)
	ls = protocol.AppendString(ls, name)
	ls = protocol.AppendVarInt(ls, 0)
	lsID, lsB, _ := tr.Clientbound(protocol.StateLogin, loginSuccess, ls)
	if err := protocol.WriteCompressed(c, lsID, lsB, compressThreshold); err != nil {
		return err
	}
	c.SetDeadline(time.Now().Add(30 * time.Second))
	ack, err := protocol.ReadCompressed(br)
	if err != nil || ack.ID != 0x03 {
		return fmt.Errorf("login ack: %v", err)
	}
	clientView, err := configure(cfg, br, c, tr, int32(welcome.Sections)*16)
	if err != nil {
		return fmt.Errorf("configuration: %w", err)
	}
	c.SetDeadline(time.Time{})
	log.Printf("%s: %q entering play (spawn %.1f,%.1f,%.1f)", c.RemoteAddr(), name, welcome.Spawn.X, welcome.Spawn.Y, welcome.Spawn.Z)
	return play(cfg, br, &clientConn{c: c, tr: tr, entTypes: map[int32]int32{}}, w, name, uuidStr, roles, welcome, clientView, clientProto)
}

// loginDisconnect sends a clientbound Login Disconnect with a JSON text reason.
// The packet's layout is stable across protocol versions.
func loginDisconnect(c net.Conn, msg string) {
	reason, _ := json.Marshal(map[string]string{"text": msg})
	protocol.WritePacket(c, 0x00, protocol.AppendString(nil, string(reason)))
}

// configure runs the Configuration state: known packs → registries
// (minecraft:enchantment skipped — its data needs Update Tags) → finish. It
// returns the client's requested view distance (0 = the client never sent
// Client Information, so the caller falls back to the default) so the join
// packet and chunk window honor the player's render-distance slider.
//
// Content is composed at cfg.Proto (the gateway's pinned version — 770 data
// translated per client for gw-770's 770-772 range; native 26.x data for 776)
// and passed through the client's translator, exactly as each gateway did
// standalone.
func configure(cfg Config, br *bufio.Reader, c net.Conn, tr protocol.Translator, worldHeight int32) (int32, error) {
	// send composes at cfg.Proto and translates to the client version.
	send := func(id int32, data []byte) error {
		id, data, drop := tr.Clientbound(protocol.StateConfiguration, id, data)
		if drop {
			return nil
		}
		return protocol.WriteCompressed(c, id, data, compressThreshold)
	}
	kp := protocol.AppendVarInt(nil, 1)
	kp = protocol.AppendString(kp, "minecraft")
	kp = protocol.AppendString(kp, "core")
	kp = protocol.AppendString(kp, protocol.MCVersion)
	if err := send(cfgClientKnownPacks, kp); err != nil {
		return 0, err
	}
	sent := false
	var clientView int32
	for {
		pkt, err := protocol.ReadCompressed(br)
		if err != nil {
			return 0, err
		}
		switch pkt.ID {
		case cfgServerClientInfo:
			// Client Information (locale string, then view-distance byte). The
			// client sends it during configuration; capture the view distance
			// so the join packet advertises the right value from the start.
			if v, ok := clientViewDist(pkt.Data); ok {
				clientView = v
			}
		case cfgServerKnownPacks:
			// Shared composition from tachyne-common — identical to what the
			// monolith sends a 770 client (registries incl. enchantment, tags,
			// brand, features). Composed at canonical 770; send() translates.
			if err := send(cfgClientCustomPayload, protocol.BrandPayload()); err != nil {
				return 0, err
			}
			if err := send(cfgClientFeatures, protocol.FeatureFlags()); err != nil {
				return 0, err
			}
			// The overworld dimension declares the WORLD's real height
			// (attach Welcome) — a tall earth world tells the client its
			// true ceiling so chunk columns and the build limit match.
			for _, data := range protocol.ConfigRegistryPacketsFor(cfg.Proto, worldHeight) {
				if err := send(cfgClientRegistryData, data); err != nil {
					return 0, err
				}
			}
			if err := send(cfgClientUpdateTags, protocol.UpdateTagsPacket(cfg.Proto)); err != nil {
				return 0, err
			}
			if err := send(cfgClientFinish, nil); err != nil {
				return 0, err
			}
			sent = true
		case cfgServerFinish:
			if !sent {
				return 0, errors.New("finish before registries")
			}
			return clientView, nil
		}
	}
}

// clientViewDist reads the view-distance byte out of a serverbound Client
// Information packet body (a locale string, then the view distance). Returns
// false if the body is malformed.
func clientViewDist(data []byte) (int32, bool) {
	r := bytes.NewReader(data)
	if _, err := protocol.ReadString(r); err != nil {
		return 0, false
	}
	vd, err := r.ReadByte()
	if err != nil {
		return 0, false
	}
	return int32(vd), true
}

// effectiveView clamps a client's requested view distance to the range the
// gateway will actually stream. 0 (client never told us) falls back to the
// default; cap is the deployment ceiling (Config.viewCap).
func effectiveView(clientView, cap int32) int32 {
	v := clientView
	if v <= 0 {
		v = viewRadius
	}
	if v < 2 {
		v = 2
	}
	if v > cap {
		v = cap
	}
	return v
}

// play runs the bridge: join sequence, then two pumps — world frames →
// rendered packets, client packets → Move/Want frames. The world side rides
// attach.Backend (the swappable world-pod connection) and attach.DialSession
// (login + handover resume) — shared with the Bedrock gateway.

// dialResume opens the destination pod on a handover and resumes the player
// there (Hello{Purpose:"resume", token}). Returns the new conn + its Welcome.
func dialResume(cfg Config, destSID int32, token, name, uuidStr string, roles []string) (net.Conn, attach.Welcome, error) {
	return attach.DialSession(fmt.Sprintf(cfg.WorldPattern, destSID), attach.Hello{
		Token: cfg.AttachToken, Gateway: fmt.Sprintf("%s/%d", cfg.Name, cfg.SID),
		Name: name, UUID: uuidStr, Roles: roles, Edition: "java",
		Purpose: "resume", ResumeToken: token,
	})
}

func play(cfg Config, br *bufio.Reader, cc *clientConn, w net.Conn, name, uuidStr string, roles []string, welcome attach.Welcome, clientView, clientProto int32) error {
	b := attach.NewBackend(w)
	defer func() { b.Get().Close() }() // close the CURRENT backend (post-swap) on exit
	pos := welcome.Spawn
	var curDim atomic.Int32
	ccx, ccz := int32(math.Floor(pos.X))>>4, int32(math.Floor(pos.Z))>>4
	// viewDist is the honored render distance (clamped). Atomic because the
	// client→world reader updates it on a video-settings change while the
	// world→client reader reads it for teleport/refresh re-Wants.
	var viewDist atomic.Int32
	viewDist.Store(effectiveView(clientView, cfg.viewCap()))
	onGround := true // updated from every movement packet's flag bit
	// The join-time center chunk, immutable: its arrival releases the client
	// from "Loading terrain" (spawnSync). ccx/ccz drift with movement and
	// belong to the reader goroutines; the pacer must not race on them.
	spawnCX, spawnCZ := ccx, ccz

	cc.send(playClientLogin, joinPacket(welcome.EID, welcome.Gamemode, viewDist.Load()))
	cc.send(playClientGameEvent, []byte{13, 0, 0, 0, 0})
	cc.send(playClientCenterChunk, protocol.AppendVarInt(protocol.AppendVarInt(nil, ccx), ccz))
	tp := render770.Time(attach.Time{Time: welcome.Time})
	cc.send(tp.ID, tp.Body)
	if err := b.Write(attach.MsgWant, attach.Want{CX: ccx, CZ: ccz, Radius: viewDist.Load(), Dim: 0}); err != nil {
		return err
	}

	// Per-viewer entity renderer, shared with every consumer of the domain
	// event stream (tachyne-common/render770): relative moves vs absolute
	// resyncs, NoSync entities, skins, projectile launch arcs.
	view := render770.NewEntityView()

	errs := make(chan error, 4)
	var once sync.Once
	spawnSync := func() {
		once.Do(func() {
			cc.send(playClientSyncPosition, syncPositionBody(pos))
		})
	}

	// Chunk delivery is PACED (vanilla 1.20.2+ chunk batches): the world
	// reader queues chunk frames and the pacer sends them once per tick in
	// chunk_batch_start/finished batches, sized by the client's
	// chunk_batch_received acks — a client still meshing slows the stream
	// instead of drowning in it. Full queues block the world reader, which
	// is deliberate backpressure through the attach socket to the engine's
	// chunk build pool.
	//
	// Decode+render runs in a small worker pool BETWEEN the reader and the
	// pacer (a tall earth chunk costs ~10× a vanilla one to decode and
	// re-render — single-threaded it caps the whole stream), with order
	// preserved by queueing a promise per chunk: workers race ahead, the
	// pacer consumes strictly in arrival (nearest-first) order.
	type pacedChunk struct {
		done        chan struct{}
		dim, cx, cz int32
		pkt         []byte // rendered Chunk Data wire body
		err         error
	}
	// Queue depth bounds gateway memory, not just latency: a RENDERED tall
	// chunk is ~0.5 MB (light arrays dominate), so 128 in flight ≈ 64 MB per
	// slow session worst-case; beyond that the world reader blocks and the
	// backpressure reaches the engine.
	renderQ := make(chan *pacedChunk, 128) // ordered hand-off to the pacer
	type chunkJob struct {
		payload []byte
		out     *pacedChunk
	}
	jobs := make(chan chunkJob, 128)
	stop := make(chan struct{})
	defer close(stop)
	for range 4 {
		go func() {
			for {
				var j chunkJob
				select {
				case <-stop:
					return
				case j = <-jobs:
				}
				h, body, err := attach.DecodeChunk(j.payload)
				if err != nil {
					j.out.err = err
				} else {
					j.out.dim, j.out.cx, j.out.cz = h.Dim, h.CX, h.CZ
					j.out.pkt = chunkPacket(h, body)
				}
				close(j.out.done)
			}
		}()
	}
	var desired atomic.Uint32 // client's desired chunks/tick (float32 bits)
	desired.Store(math.Float32bits(startChunksTick))
	var unacked atomic.Int32
	var lastAck atomic.Int64 // unix nanos of the newest batch ack
	lastAck.Store(time.Now().UnixNano())
	go func() {
		t := time.NewTicker(batchInterval)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
			}
			if len(renderQ) == 0 {
				continue
			}
			if unacked.Load() >= maxUnackedBatch {
				// Hold for acks — but a client that never sends them must
				// not leave the session holding chunks forever.
				if time.Since(time.Unix(0, lastAck.Load())) < batchAckDeadline {
					continue
				}
				unacked.Store(0)
			}
			budget := int(math.Ceil(float64(math.Float32frombits(desired.Load()))))
			if budget < 1 {
				budget = 1
			}
			if budget > maxChunksTick {
				budget = maxChunksTick
			}
			if err := cc.send(playClientChunkBatchStart, nil); err != nil {
				errs <- err
				return
			}
			var n int32
		drain:
			for int(n) < budget {
				select {
				case pc := <-renderQ:
					select { // rendered by the worker pool; usually already done
					case <-pc.done:
					case <-stop:
						return
					}
					if pc.err != nil {
						errs <- pc.err
						return
					}
					if pc.dim != curDim.Load() {
						continue // stale chunk from before a dimension switch
					}
					if err := cc.send(playClientChunkData, pc.pkt); err != nil {
						errs <- err
						return
					}
					if pc.cx == spawnCX && pc.cz == spawnCZ {
						spawnSync() // ground under the player's feet exists now
					}
					n++
				default:
					break drain
				}
			}
			if err := cc.send(playClientChunkBatchFinish, protocol.AppendVarInt(nil, n)); err != nil {
				errs <- err
				return
			}
			unacked.Add(1)
		}
	}()

	// World → client.
	go func() {
		// Advancements are stateful: the tree frame (join-time, static) is
		// held until the progress snapshot pairs with it in one reset packet;
		// its requirement index then serves every incremental grant.
		var advTree *attach.AdvTree
		var advReqs map[string][]string
		for {
			typ, payload, err := attach.ReadFrame(b.Get())
			if err != nil {
				errs <- fmt.Errorf("world: %w", err)
				return
			}
			switch typ {
			case attach.MsgChunk:
				// Queue the promise (ordered) and the decode job (raced);
				// blocking on full queues is the backpressure path.
				pc := &pacedChunk{done: make(chan struct{})}
				select {
				case renderQ <- pc:
				case <-stop:
					return
				}
				select {
				case jobs <- chunkJob{payload: payload, out: pc}:
				case <-stop:
					return
				}
			case attach.MsgPlayerInfo:
				var e attach.PlayerInfo
				if json.Unmarshal(payload, &e) == nil {
					p := render770.PlayerInfoAdd(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgPlayerGone:
				var e attach.PlayerGone
				if json.Unmarshal(payload, &e) == nil {
					p := render770.PlayerRemove(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgEntityAdd:
				var e attach.EntityAdd
				if json.Unmarshal(payload, &e) == nil {
					cc.entTypes[e.EID] = e.Type // for the version-specific metadata policies
					p := view.Add(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgEntityMove:
				var e attach.EntityMove
				if json.Unmarshal(payload, &e) == nil {
					p := view.Move(e) // relative, or absolute resync — render770 decides
					cc.send(p.ID, p.Body)
				}
			case attach.MsgEntityHead:
				var e attach.EntityHead
				if json.Unmarshal(payload, &e) == nil {
					p := render770.Head(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgEntityRemove:
				var e attach.EntityRemove
				if json.Unmarshal(payload, &e) == nil {
					for _, eid := range e.EIDs {
						delete(cc.entTypes, eid)
					}
					p := view.Remove(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgBlockSet:
				var e attach.BlockSet
				if json.Unmarshal(payload, &e) == nil {
					p := render770.BlockSet(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgChat:
				var e attach.Chat
				if json.Unmarshal(payload, &e) == nil {
					p := render770.Chat(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgStats:
				var e attach.Stats
				if json.Unmarshal(payload, &e) == nil {
					p := render770.AwardStats(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgAdvTree:
				var e attach.AdvTree
				if json.Unmarshal(payload, &e) == nil {
					if advReqs == nil {
						// join: hold the visible tree for the reset packet
						advTree = &e
						advReqs = render770.ReqIndex(e)
					} else {
						// the world revealed more nodes: extend the session
						// index and ship them (no reset)
						advTree.Nodes = append(advTree.Nodes, e.Nodes...)
						for id, crits := range render770.ReqIndex(e) {
							advReqs[id] = crits
						}
						if len(e.Nodes) > 0 {
							p := render770.AdvancementsAdd(e)
							cc.send(p.ID, p.Body)
						}
					}
				}
			case attach.MsgAdvProgress:
				var e attach.AdvProgress
				if json.Unmarshal(payload, &e) == nil {
					if e.Reset && advTree != nil {
						p := render770.AdvancementsInit(*advTree, e)
						cc.send(p.ID, p.Body)
					} else if advReqs != nil {
						p := render770.AdvancementsUpdate(advReqs, e)
						cc.send(p.ID, p.Body)
					}
				}
			case attach.MsgBossBar:
				var e attach.BossBar
				if json.Unmarshal(payload, &e) == nil {
					p := render770.BossBar(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgHealth:
				var e attach.Health
				if json.Unmarshal(payload, &e) == nil {
					p := render770.Health(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgXP:
				var e attach.XP
				if json.Unmarshal(payload, &e) == nil {
					p := render770.XP(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgEffect:
				var e attach.Effect
				if json.Unmarshal(payload, &e) == nil {
					p := render770.Effect(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgHurt:
				var e attach.Hurt
				if json.Unmarshal(payload, &e) == nil {
					p := render770.Hurt(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgDeath:
				var e attach.Death
				if json.Unmarshal(payload, &e) == nil {
					p := render770.Death(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgEquipment:
				var e attach.Equipment
				if json.Unmarshal(payload, &e) == nil {
					p := render770.Equipment(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgEntityMeta:
				var e attach.EntityMeta
				if json.Unmarshal(payload, &e) == nil {
					// Version-specific metadata policy. Every rule is keyed on the
					// entity's canonical type + the client version, and every helper
					// self-no-ops at versions it doesn't concern — so this one block
					// serves all pinned gateways.
					etype := cc.entTypes[e.EID]
					// Drop metadata for a substituted entity (pre-774 clients): its
					// stand-in (e.g. a Ghast for a Happy Ghast) has a different
					// schema, so the original's type-specific entity-data would
					// mis-render or, at a mismatched type, disconnect the client.
					if protocol.IsSubstituted(clientProto, etype) {
						break
					}
					p := render770.EntityMeta(e)
					// Cube mobs (slime/magma) need their SIZE index shifted 16→18 on
					// 26.2; done here, not in the translator, because the shift is
					// type-specific and set_entity_data carries no type.
					if etype == typeSlime || etype == typeMagmaCube {
						p.Body = protocol.ShiftCubeMobMeta(clientProto, p.Body)
					}
					// The copper golem's index-16 oxidation state ships as an INT
					// placeholder; restore its WEATHERING_COPPER_STATE value-type on
					// clients that have the serializer (774+), else it type-mismatches.
					if etype == typeCopperGolem {
						p.Body = protocol.FixCopperGolemMeta(clientProto, p.Body)
					}
					// The painting variant's serializer id renumbered in 26.x
					// (COMPOUND_TAG removed, sound-variant serializers added).
					if etype == typePainting {
						p.Body = protocol.FixPaintingMeta(clientProto, p.Body)
					}
					// Item frames: 26.x's inserted DIRECTION shifts the
					// item/rotation entry indices by one.
					if etype == typeItemFrame || etype == typeGlowFrame {
						p.Body = protocol.FixItemFrameMeta(clientProto, p.Body)
					}
					cc.send(p.ID, p.Body)
				}
			case attach.MsgWindowOpen:
				var e attach.WindowOpen
				if json.Unmarshal(payload, &e) == nil {
					p := render770.WindowOpen(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgWindowItems:
				var e attach.WindowItems
				if json.Unmarshal(payload, &e) == nil {
					p := render770.WindowItems(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgWindowSlot:
				var e attach.WindowSlot
				if json.Unmarshal(payload, &e) == nil {
					p := render770.WindowSlot(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgWindowData:
				var e attach.WindowData
				if json.Unmarshal(payload, &e) == nil {
					p := render770.WindowData(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgHeldSync:
				var e attach.HeldSync
				if json.Unmarshal(payload, &e) == nil {
					p := render770.HeldSync(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgCollect:
				var e attach.Collect
				if json.Unmarshal(payload, &e) == nil {
					p := render770.Collect(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgSound:
				var e attach.Sound
				if json.Unmarshal(payload, &e) == nil {
					p := render770.Sound(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgParticles:
				var e attach.Particles
				if json.Unmarshal(payload, &e) == nil {
					p := render770.Particles(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgWorldFX:
				var e attach.WorldFX
				if json.Unmarshal(payload, &e) == nil {
					p := render770.WorldFX(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgGameEvent:
				var e attach.GameEvent
				if json.Unmarshal(payload, &e) == nil {
					p := render770.GameEvent(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgAbilities:
				var e attach.Abilities
				if json.Unmarshal(payload, &e) == nil {
					p := render770.Abilities(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgPassengers:
				var e attach.Passengers
				if json.Unmarshal(payload, &e) == nil {
					p := render770.Passengers(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgVehicleMove:
				var e attach.VehicleMove
				if json.Unmarshal(payload, &e) == nil {
					p := render770.VehicleMove(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgVelocity:
				var e attach.Velocity
				if json.Unmarshal(payload, &e) == nil {
					p := render770.Velocity(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgTrades:
				var e attach.Trades
				if json.Unmarshal(payload, &e) == nil {
					p := render770.Trades(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgCursorItem:
				var e attach.CursorItem
				if json.Unmarshal(payload, &e) == nil {
					p := render770.CursorItem(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgDifficulty:
				var e attach.Difficulty
				if json.Unmarshal(payload, &e) == nil {
					p := render770.Difficulty(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgCommandTree:
				var e attach.CommandTree
				if json.Unmarshal(payload, &e) == nil {
					p := render770.CommandTree(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgDimension:
				var e attach.Dimension
				if json.Unmarshal(payload, &e) == nil {
					curDim.Store(e.Dim)
					view.Reset() // the client discards its entity world on respawn
					p := render770.Respawn(e)
					cc.send(p.ID, p.Body)
					cc.send(playClientGameEvent, []byte{13, 0, 0, 0, 0})
				}
			case attach.MsgRehome:
				// The player was migrated to a neighbour shard. Swap our world
				// backend to the destination pod WITHOUT dropping the client, then
				// force a client entity/chunk reload so the old shard's world is
				// discarded and the new shard streams in. The client socket lives.
				var rh attach.Rehome
				if json.Unmarshal(payload, &rh) != nil {
					continue
				}
				nw, wel, err := dialResume(cfg, rh.DestSID, rh.Token, name, uuidStr, roles)
				if err != nil {
					errs <- fmt.Errorf("rehome: %w", err)
					return
				}
				b.Swap(nw) // the next ReadFrame(b.Get()) reads from the destination pod
				pos = wel.Spawn
				ccx, ccz = int32(math.Floor(pos.X))>>4, int32(math.Floor(pos.Z))>>4
				curDim.Store(0)
				// SILENT swap — no Respawn, no "waiting for chunks" game event: the
				// client keeps its loaded chunks (the overlap already streamed the
				// far side, and the destination re-serves the same window, replacing
				// chunks in place), so there is no "Loading terrain" screen and the
				// held hotbar slot survives. Entities are reconciled by hand instead
				// of the Respawn's implicit wipe: destroy exactly what this viewer
				// has rendered (the resume join re-adds the destination's roster —
				// same session-stable eids, so crossers and shadows stay continuous).
				if eids := view.Tracked(); len(eids) > 0 {
					p := view.Remove(attach.EntityRemove{EIDs: eids})
					cc.send(p.ID, p.Body)
				}
				view.Reset()
				cc.send(playClientCenterChunk, protocol.AppendVarInt(protocol.AppendVarInt(nil, ccx), ccz))
				// Pin the client at the real crossing position (absolute x/y/z,
				// camera kept) so it never sits at a stale height mid-swap.
				cc.send(playClientSyncPosition, syncPositionKeepView(pos))
				b.Write(attach.MsgWant, attach.Want{CX: ccx, CZ: ccz, Radius: viewDist.Load(), Dim: 0})
			case attach.MsgTeleport:
				var e attach.Teleport
				if json.Unmarshal(payload, &e) == nil {
					pos = e.Pos
					ccx, ccz = int32(math.Floor(pos.X))>>4, int32(math.Floor(pos.Z))>>4
					cc.send(playClientCenterChunk, protocol.AppendVarInt(protocol.AppendVarInt(nil, ccx), ccz))
					cc.send(playClientSyncPosition, syncPositionBody(e.Pos))
					b.Write(attach.MsgWant, attach.Want{CX: ccx, CZ: ccz, Radius: viewDist.Load(), Dim: curDim.Load()})
				}
			case attach.MsgTime:
				var t attach.Time
				if json.Unmarshal(payload, &t) == nil {
					p := render770.Time(t)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgEntityStatus:
				var e attach.EntityStatus
				if json.Unmarshal(payload, &e) == nil {
					p := render770.EntityStatus(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgSwing:
				var e attach.Swing
				if json.Unmarshal(payload, &e) == nil {
					p := render770.Swing(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgObjective:
				var e attach.Objective
				if json.Unmarshal(payload, &e) == nil {
					p := render770.Objective(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgDisplaySlot:
				var e attach.DisplaySlot
				if json.Unmarshal(payload, &e) == nil {
					p := render770.DisplaySlot(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgScore:
				var e attach.Score
				if json.Unmarshal(payload, &e) == nil {
					p := render770.Score(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgTeam:
				var e attach.Team
				if json.Unmarshal(payload, &e) == nil {
					// Composed at the client's real version: 26.2 reordered the
					// team parameters (see render770.PlayerTeam).
					p := render770.PlayerTeam(e, clientProto)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgRecipeSettings:
				var e attach.RecipeSettings
				if json.Unmarshal(payload, &e) == nil {
					p := render770.RecipeBookSettings(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgSignText:
				var e attach.SignText
				if json.Unmarshal(payload, &e) == nil {
					p := render770.SignData(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgMapData:
				var e attach.MapData
				if json.Unmarshal(payload, &e) == nil {
					p := render770.MapItemData(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgSignEditor:
				var e attach.SignEditor
				if json.Unmarshal(payload, &e) == nil {
					p := render770.SignEditor(e)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgRecipeBook:
				var e attach.RecipeBook
				if json.Unmarshal(payload, &e) == nil {
					// Rendered at the client's real version (recipe_book_add has
					// no body rewriter); cc.send only renumbers the packet id.
					p := render770.RecipeBook(e, clientProto)
					cc.send(p.ID, p.Body)
				}
			case attach.MsgResync:
				// Force-resend the current chunk window (world side of /refresh).
				b.Write(attach.MsgWant, attach.Want{CX: ccx, CZ: ccz, Radius: viewDist.Load(), Dim: curDim.Load(), Force: true})
			case attach.MsgPing:
				var buf [8]byte
				copy(buf[:], payload)
				fr := make([]byte, 0, 16)
				fr = binary.BigEndian.AppendUint32(fr, 9)
				fr = append(fr, attach.MsgPong)
				fr = append(fr, buf[:]...)
				b.Get().Write(fr) // through the CURRENT backend — after a swap, pongs must reach the NEW pod
			case attach.MsgBye:
				var bye attach.Bye
				json.Unmarshal(payload, &bye)
				errs <- fmt.Errorf("world closed session: %s", bye.Reason)
				return
			}
		}
	}()

	// Client → world (runs in this goroutine) + keepalive ticker.
	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		var n int64
		for range t.C {
			n++
			if cc.send(playClientKeepAlive, protocol.AppendI64(nil, n)) != nil {
				return
			}
		}
	}()

	go func() {
		for {
			pkt, err := protocol.ReadCompressed(br)
			if err != nil {
				if errors.Is(err, io.EOF) {
					errs <- nil // clean disconnect
				} else {
					errs <- fmt.Errorf("client: %w", err)
				}
				return
			}
			// Back-translate the client's version to canonical 770 so the
			// switch below (and the render770 parsers) speak one id space.
			// Identity for a 770 client.
			sbID, sbData, sbDrop := cc.tr.Serverbound(protocol.StatePlay, pkt.ID, pkt.Data)
			if sbDrop {
				continue
			}
			pkt.ID, pkt.Data = sbID, sbData
			switch pkt.ID {
			case render770.SIDChunkBatchReceived:
				// Gateway-local flow control: adopt the client's desired
				// chunks-per-tick and release one in-flight batch. Never
				// forwarded to the world.
				if v, ok := render770.ParseChunkBatchReceived(pkt.Data); ok {
					desired.Store(math.Float32bits(v))
					if unacked.Load() > 0 {
						unacked.Add(-1)
					}
					lastAck.Store(time.Now().UnixNano())
				}
			case playServerBlockDig:
				r := pkt.Body()
				status, _ := protocol.ReadVarInt(r)
				var posb [8]byte
				if _, err := io.ReadFull(r, posb[:]); err != nil {
					continue
				}
				face, _ := r.ReadByte()
				seq, _ := protocol.ReadVarInt(r)
				x, y, z := protocol.ReadPosition(posb[:])
				b.Write(attach.MsgDig, attach.Dig{Status: status, X: x, Y: y, Z: z, Face: int32(face)})
				cc.send(playClientAckDig, protocol.AppendVarInt(nil, seq))
			case playServerBlockPlace:
				r := pkt.Body()
				hand, _ := protocol.ReadVarInt(r)
				var posb [8]byte
				if _, err := io.ReadFull(r, posb[:]); err != nil {
					continue
				}
				face, _ := protocol.ReadVarInt(r)
				var cur [12]byte
				if _, err := io.ReadFull(r, cur[:]); err != nil {
					continue
				}
				inside, _ := r.ReadByte()
				r.ReadByte() // world border hit
				seq, _ := protocol.ReadVarInt(r)
				x, y, z := protocol.ReadPosition(posb[:])
				b.Write(attach.MsgPlace, attach.Place{
					Hand: hand, X: x, Y: y, Z: z, Face: face,
					CX: f32(cur[0:]), CY: f32(cur[4:]), CZ: f32(cur[8:]), Inside: inside == 1,
				})
				cc.send(playClientAckDig, protocol.AppendVarInt(nil, seq))
			case playServerHeldItem:
				if len(pkt.Data) >= 2 {
					b.Write(attach.MsgHeldSlot, attach.HeldSlot{Slot: int16(binary.BigEndian.Uint16(pkt.Data))})
				}
			case playServerChatCommand:
				if cmd, err := protocol.ReadString(pkt.Body()); err == nil && cmd != "" {
					b.Write(attach.MsgCommand, attach.Command{Cmd: cmd})
				}
			case playServerChatMessage:
				if text, err := protocol.ReadString(pkt.Body()); err == nil && text != "" {
					b.Write(attach.MsgChat, attach.Chat{Text: text})
				}
			case render770.SIDUseItem:
				b.Write(attach.MsgUseItem, attach.UseItem{})
			case render770.SIDUseEntity:
				if e, ok := render770.ParseUseEntity(pkt.Data); ok {
					b.Write(attach.MsgUseEntity, e)
				}
			case render770.SIDVehicleMove:
				if e, ok := render770.ParseVehicleMove(pkt.Data); ok {
					b.Write(attach.MsgVehicleMove, e)
				}
			case render770.SIDSelTrade:
				if e, ok := render770.ParseSelTrade(pkt.Data); ok {
					b.Write(attach.MsgSelTrade, e)
				}
			case render770.SIDPlayerInput:
				if e, ok := render770.ParseInput(pkt.Data); ok {
					b.Write(attach.MsgInput, e)
				}
			case render770.SIDWindowClick:
				if e, ok := render770.ParseWindowClick(pkt.Data); ok {
					b.Write(attach.MsgWindowClick, e)
				}
			case render770.SIDCraftRequest:
				if e, ok := render770.ParseCraft(pkt.Data); ok {
					b.Write(attach.MsgCraft, e)
				}
			case render770.SIDCloseWindow:
				b.Write(attach.MsgWindowClose, attach.WindowClose{})
			case render770.SIDNameItem:
				if e, ok := render770.ParseNameItem(pkt.Data); ok {
					b.Write(attach.MsgNameItem, e)
				}
			case render770.SIDEnchantItem:
				if e, ok := render770.ParseEnchant(pkt.Data); ok {
					b.Write(attach.MsgEnchant, e)
				}
			case render770.SIDSetBeacon:
				if e, ok := render770.ParseSetBeacon(pkt.Data); ok {
					b.Write(attach.MsgSetBeacon, e)
				}
			case render770.SIDEntityAction:
				if e, ok := render770.ParsePlayerAction(pkt.Data); ok {
					b.Write(attach.MsgPlayerAction, e)
				}
			case render770.SIDClientCommand:
				if e, ok := render770.ParseRespawnReq(pkt.Data); ok {
					b.Write(attach.MsgRespawnReq, e)
				} else if e, ok := render770.ParseStatsReq(pkt.Data); ok {
					b.Write(attach.MsgStatsReq, e)
				}
			case render770.SIDRecipeSettings:
				if e, ok := render770.ParseRecipeSettingChange(pkt.Data); ok {
					b.Write(attach.MsgRecipeSettingChange, e)
				}
			case render770.SIDRecipeSeen:
				if e, ok := render770.ParseRecipeSeen(pkt.Data); ok {
					b.Write(attach.MsgRecipeSeen, e)
				}
			case render770.SIDSignUpdate:
				if e, ok := render770.ParseSignUpdate(pkt.Data); ok {
					b.Write(attach.MsgSignUpdate, e)
				}
			case render770.SIDCreativeSlot:
				if e, ok := render770.ParseCreativeSlot(pkt.Data, clientProto); ok {
					b.Write(attach.MsgCreativeSlot, e)
				}
			case playServerClientInfo:
				// Mid-game video-settings change: honor the new render distance
				// (vanilla re-scales the chunk window immediately).
				if v, ok := clientViewDist(pkt.Data); ok {
					if nv := effectiveView(v, cfg.viewCap()); nv != viewDist.Load() {
						viewDist.Store(nv)
						b.Write(attach.MsgWant, attach.Want{CX: ccx, CZ: ccz, Radius: nv, Dim: curDim.Load()})
					}
				}
			case playServerPosition, playServerPositionLook, playServerLook:
				d := pkt.Data
				switch pkt.ID {
				case playServerPosition:
					if len(d) < 25 {
						continue
					}
					pos.X, pos.Y, pos.Z = f64(d), f64(d[8:]), f64(d[16:])
					onGround = d[24]&1 != 0
				case playServerPositionLook:
					if len(d) < 33 {
						continue
					}
					pos.X, pos.Y, pos.Z = f64(d), f64(d[8:]), f64(d[16:])
					pos.Yaw, pos.Pitch = f32(d[24:]), f32(d[28:])
					onGround = d[32]&1 != 0
				case playServerLook:
					if len(d) < 9 {
						continue
					}
					pos.Yaw, pos.Pitch = f32(d), f32(d[4:])
					onGround = d[8]&1 != 0
				}
				b.Write(attach.MsgMove, attach.Move{Pos: pos, OnGround: onGround})
				if ncx, ncz := int32(math.Floor(pos.X))>>4, int32(math.Floor(pos.Z))>>4; ncx != ccx || ncz != ccz {
					ccx, ccz = ncx, ncz
					cc.send(playClientCenterChunk, protocol.AppendVarInt(protocol.AppendVarInt(nil, ccx), ccz))
					b.Write(attach.MsgWant, attach.Want{CX: ccx, CZ: ccz, Radius: viewDist.Load(), Dim: curDim.Load()})
				}
			}
		}
	}()

	err := <-errs
	b.Write(attach.MsgBye, attach.Bye{Reason: "client gone"})
	log.Printf("session %q done: %v", name, err)
	return err
}

func f64(b []byte) float64 { return math.Float64frombits(binary.BigEndian.Uint64(b[:8])) }

func f32(b []byte) float32 { return math.Float32frombits(binary.BigEndian.Uint32(b[:4])) }
