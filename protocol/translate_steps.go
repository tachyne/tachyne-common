package protocol

import (
	"bytes"
	"io"
)

// Hand-written body rewriters for translation steps whose field layouts or content
// differ beyond a plain ID remap. Registered into stepBody (see translate_chain.go),
// keyed by the step's UPPER protocol version and the canonical-side packet ID.

// cfgKnownPacksID is the clientbound select_known_packs packet ID in the
// Configuration state (stable across the 770-onward chain).
const cfgKnownPacksID = 0x0e

// rewriteKnownPacksVersion returns a body rewriter for the Configuration Known
// Packs packet that replaces each advertised pack's VERSION string with ver. The
// server advertises minecraft:core@1.21.5 (its canonical version); a newer client
// has no 1.21.5 pack, so it would refuse our has_data=false registry entries and
// fail with a protocol error. Re-advertising the CLIENT's own version makes the
// pack match, so the client sources registry content from its built-in copy.
// Composes naturally: each step bumps the string to its upper version (1.21.5 →
// 1.21.6 → 1.21.8 → …), leaving it at the client's version by the end of the chain.
func rewriteKnownPacksVersion(ver string) bodyFn {
	return func(_ State, body []byte) []byte {
		r := bytes.NewReader(body)
		count, err := ReadVarInt(r)
		if err != nil {
			return body
		}
		out := AppendVarInt(nil, count)
		for i := int32(0); i < count; i++ {
			ns, e1 := ReadString(r)
			name, e2 := ReadString(r)
			_, e3 := ReadString(r) // old version — discarded
			if e1 != nil || e2 != nil || e3 != nil {
				return body // malformed; leave untouched
			}
			out = AppendString(out, ns)
			out = AppendString(out, name)
			out = AppendString(out, ver)
		}
		return out
	}
}

// Canonical (770) serverbound Interact Entity packet ID, and the 26.x serverbound
// IDs of the two packets that replaced it.
const (
	canonUseEntity = 0x18 // 770 use_entity (mouse enum: 0 interact / 1 attack / 2 interact-at)
	x26AttackID    = 0x01 // 26.x Attack (left-click): body = target VarInt only
	x26InteractID  = 0x1a // 26.x Interact (right-click): target + hand + hit-loc + secondary
)

// rewrite26xEntity converts the 26.x split entity-interaction packets back to the
// canonical single use_entity packet. 26.x (proto ≥775) dropped use_entity's "mouse"
// enum and split it into Attack (left-click) and Interact (right-click), so a 770
// core never saw the attack (only right-click's Interact reached it, and only by
// luck). Map Attack→mouse=1 (attack) and Interact→mouse=0 (interact). Returns
// handled=false for any other packet, leaving the normal chain to translate it.
func rewrite26xEntity(id int32, body []byte) (int32, []byte, bool) {
	switch id {
	case x26AttackID: // left-click → use_entity mouse=1
		r := bytes.NewReader(body)
		target, err := ReadVarInt(r)
		if err != nil {
			return id, body, false
		}
		out := AppendVarInt(nil, target)
		out = AppendVarInt(out, 1)   // mouse = attack
		out = AppendBool(out, false) // sneaking (unused by our handler)
		return canonUseEntity, out, true
	case x26InteractID: // right-click → use_entity mouse=0 (interact; a no-op for us)
		r := bytes.NewReader(body)
		target, e1 := ReadVarInt(r)
		hand, e2 := ReadVarInt(r)
		if e1 != nil || e2 != nil {
			return id, body, false
		}
		var loc [6]byte // hit-location lpVec3 (3×i16) — discarded
		io.ReadFull(r, loc[:])
		secondary, _ := r.ReadByte()
		out := AppendVarInt(nil, target)
		out = AppendVarInt(out, 0) // mouse = interact
		out = AppendVarInt(out, hand)
		out = AppendBool(out, secondary != 0)
		return canonUseEntity, out, true
	}
	return id, body, false
}

// spawnEntityID is the clientbound spawn_entity packet ID (canonical 770; unchanged
// through 773, so the same key works for the 772→773 step body rewriter).
const spawnEntityID = 0x01

// rewriteSpawnEntity772to773 reorders the spawn_entity body for 1.21.9+: velocity
// moved from the trailing position (after objectData) to right after z, and its
// type changed vec3i16 (3×i16) → lpVec3. We always send velocity 0, which lpVec3
// encodes as a single 0x00 byte, so we drop the 6 zero bytes and emit one.
//
//	772: id, uuid, type, x,y,z, pitch,yaw,headPitch, objectData, velocity(6)
//	773: id, uuid, type, x,y,z, velocity(lpVec3), pitch,yaw,headPitch, objectData
func rewriteSpawnEntity772to773(_ State, body []byte) []byte {
	r := bytes.NewReader(body)
	eid, e1 := ReadVarInt(r)
	var uuid [16]byte
	if _, err := io.ReadFull(r, uuid[:]); err != nil {
		return body
	}
	typ, e2 := ReadVarInt(r)
	var xyz [24]byte // x,y,z as f64
	if _, err := io.ReadFull(r, xyz[:]); err != nil {
		return body
	}
	var ang [3]byte // pitch, yaw, headPitch (i8)
	if _, err := io.ReadFull(r, ang[:]); err != nil {
		return body
	}
	objData, e3 := ReadVarInt(r)
	var vel [6]byte // vec3i16 velocity — we always send 0
	if _, err := io.ReadFull(r, vel[:]); err != nil {
		return body
	}
	if e1 != nil || e2 != nil || e3 != nil {
		return body
	}
	out := AppendVarInt(nil, eid)
	out = append(out, uuid[:]...)
	out = AppendVarInt(out, typ)
	out = append(out, xyz[:]...)
	out = append(out, 0x00) // lpVec3 zero velocity
	out = append(out, ang[:]...)
	out = AppendVarInt(out, objData)
	return out
}

func init() {
	// 770→771 and 771→772 are pure ID remaps on the wire EXCEPT the Known Packs
	// content negotiation: the advertised core version must name the client's
	// version so it loads its own registries. Each step bumps the string by one.
	stepBody[771] = bodyRewriters{
		cbUp: map[State]map[int32]bodyFn{
			StateConfiguration: {cfgKnownPacksID: rewriteKnownPacksVersion("1.21.6")},
		},
	}
	stepBody[772] = bodyRewriters{
		cbUp: map[State]map[int32]bodyFn{
			StateConfiguration: {cfgKnownPacksID: rewriteKnownPacksVersion("1.21.8")},
		},
	}
	// 772→773: spawn_entity velocity moved + re-encoded (the one wire change among
	// packets we send); player_info's game_profile rename is wire-identical for our
	// 0-property entries. Proto 773 covers BOTH 1.21.9 and 1.21.10 but the handshake
	// only gives the number, so we advertise the newer (1.21.10); a 1.21.9 client
	// would fail the Known Packs match (acceptable — testers use the latest).
	stepBody[773] = bodyRewriters{
		cbUp: map[State]map[int32]bodyFn{
			StateConfiguration: {cfgKnownPacksID: rewriteKnownPacksVersion("1.21.10")},
			StatePlay:          {spawnEntityID: rewriteSpawnEntity772to773},
		},
	}
	// 773→774: wire-identical; only the Known Packs version differs (1.21.11).
	stepBody[774] = bodyRewriters{
		cbUp: map[State]map[int32]bodyFn{
			StateConfiguration: {cfgKnownPacksID: rewriteKnownPacksVersion("1.21.11")},
		},
	}
	// 774→775 (26.1) and 775→776 (26.2): Known Packs version. The chunk fluid-count
	// (775+) and Join trailing boolean (776) are handled at canonical IDs in
	// remapClientboundIDs; block/item/entity ID shifts via the translation tables.
	stepBody[775] = bodyRewriters{
		cbUp: map[State]map[int32]bodyFn{
			StateConfiguration: {cfgKnownPacksID: rewriteKnownPacksVersion("26.1")},
			// 26.1 changed the advancement DisplayInfo icon from a full Slot to
			// an ItemStackTemplate (field reorder) — see advremap.go. Keyed by
			// the packet's 774-space id (renumbered at the 773 step).
			StatePlay: {advancementsID774: rewriteAdvIconTemplate},
		},
	}
	stepBody[776] = bodyRewriters{
		cbUp: map[State]map[int32]bodyFn{
			StateConfiguration: {cfgKnownPacksID: rewriteKnownPacksVersion("26.2")},
			StateLogin:         {loginFinishedID: appendLoginSessionID},
		},
	}
}

// loginFinishedID is the clientbound Login Success / login_finished packet ID
// (login state, stable across the chain).
const loginFinishedID = 0x02

// appendLoginSessionID adds the trailing Session ID (UUID) that 26.2 added to
// login_finished. login_finished is UUID + Name + ProfileProperties[]; 26.2 appends
// a Session ID UUID after it. Offline mode accepts any value, so we reuse the
// player's UUID (the leading 16 bytes) as a valid non-zero session ID.
func appendLoginSessionID(_ State, body []byte) []byte {
	if len(body) < 16 {
		return body
	}
	return append(append([]byte(nil), body...), body[:16]...)
}
