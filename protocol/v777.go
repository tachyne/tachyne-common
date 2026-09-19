package protocol

import "bytes"

// 776→777 (26.3) body rewriters. Facts from the 26.3 server's own packet
// report and its protocol classes: the login/respawn spawn info carries the
// previous game mode as an optional varint (0 = none, else id+1) instead of
// a nullable byte; entity moves put an "on ground | step count" varint right
// after the entity id (a plain move is step count 0) and drop the trailing
// on-ground byte; entity position sync carries a position PATH (type 0 =
// linear + Vec3) and no delta movement; level particles lead with the
// particle, split the max speed into three and end with a randomization
// type; the arm swing left the animate packet for its own swing_animation
// packet (entity, hand, type, duration), renumbering the actions that
// stayed; the serverbound swing became a payload-free punch, sign updates
// moved the text-side flag behind the lines, and teleport confirms echo the
// accepted position. Keys are the 26.2-space packet ids (the step's lower).
const (
	login776               = 49
	respawn776             = 82
	moveEntityPos776       = 53
	moveEntityPosRot776    = 54
	entityPositionSync776  = 35
	levelParticles776      = 47
	animate776             = 2
	signUpdate776          = 61 // serverbound
	acceptTeleportation776 = 0  // serverbound
	swing776               = 63 // serverbound: 26.3's punch maps here (protomap_777_gen.go)

	// SwingAnimation777 is the clientbound swing_animation id at 26.3.
	SwingAnimation777 = 123
)

func init() {
	stepBody[777] = bodyRewriters{
		cbUp: map[State]map[int32]bodyFn{
			StateConfiguration: {cfgKnownPacksID: rewriteKnownPacksVersion("26.3")},
			StatePlay: {
				login776:              rewriteLoginPrevGameMode777,
				respawn776:            rewriteRespawnPrevGameMode777,
				moveEntityPos776:      rewriteMoveEntity777(false),
				moveEntityPosRot776:   rewriteMoveEntity777(true),
				entityPositionSync776: rewriteEntityPositionSync777,
				levelParticles776:     rewriteLevelParticles777,
				animate776:            rewriteAnimateActions777,
			},
		},
		sbDown: map[State]map[int32]bodyFn{
			StatePlay: {
				signUpdate776:          rewriteSignUpdate777,
				acceptTeleportation776: rewriteAcceptTeleportation777,
				swing776:               rewritePunchToSwing777,
			},
		},
	}
}

// prevGameMode777 turns the nullable previous-game-mode byte (-1 = none)
// into GameType.OPTIONAL_STREAM_CODEC's varint (0 = none, else id+1).
func prevGameMode777(b byte) []byte {
	if b == 0xff {
		return []byte{0}
	}
	return AppendVarInt(nil, int32(b)+1)
}

// skipSpawnInfoToPrev copies the CommonPlayerSpawnInfo up to and excluding
// the previous-game-mode byte: dimension type varint, dimension string,
// seed i64, game mode byte. Returns the output so far and the reader.
func skipSpawnInfoToPrev(r *bytes.Reader, out []byte) ([]byte, bool) {
	if !copyVarInt(r, &out) { // dimension type
		return out, false
	}
	dim, err := ReadString(r)
	if err != nil {
		return out, false
	}
	out = AppendString(out, dim)
	var seed [8]byte
	if _, err := r.Read(seed[:]); err != nil {
		return out, false
	}
	out = append(out, seed[:]...)
	gm, err := r.ReadByte()
	if err != nil {
		return out, false
	}
	return append(out, gm), true
}

// finishOptionalPrev reads the previous-game-mode byte, writes its 26.3
// form and copies the rest of the packet verbatim.
func finishOptionalPrev(r *bytes.Reader, out []byte, orig []byte) []byte {
	prev, err := r.ReadByte()
	if err != nil {
		return orig
	}
	out = append(out, prevGameMode777(prev)...)
	rest := make([]byte, r.Len())
	r.Read(rest)
	return append(out, rest...)
}

// rewriteLoginPrevGameMode777: login = i32 player id, bool hardcore, string
// list, three varints, three bools, then the spawn info.
func rewriteLoginPrevGameMode777(_ State, body []byte) []byte {
	r := bytes.NewReader(body)
	var out []byte
	var head [5]byte
	if _, err := r.Read(head[:]); err != nil {
		return body
	}
	out = append(out, head[:]...)
	n, err := ReadVarInt(r)
	if err != nil || n < 0 || n > 64 {
		return body
	}
	out = AppendVarInt(out, n)
	for i := int32(0); i < n; i++ {
		s, err := ReadString(r)
		if err != nil {
			return body
		}
		out = AppendString(out, s)
	}
	for i := 0; i < 3; i++ {
		if !copyVarInt(r, &out) {
			return body
		}
	}
	var flags [3]byte
	if _, err := r.Read(flags[:]); err != nil {
		return body
	}
	out = append(out, flags[:]...)
	out, ok := skipSpawnInfoToPrev(r, out)
	if !ok {
		return body
	}
	return finishOptionalPrev(r, out, body)
}

// rewriteRespawnPrevGameMode777: respawn = spawn info, then the keep byte.
func rewriteRespawnPrevGameMode777(_ State, body []byte) []byte {
	r := bytes.NewReader(body)
	out, ok := skipSpawnInfoToPrev(r, nil)
	if !ok {
		return body
	}
	return finishOptionalPrev(r, out, body)
}

// rewriteMoveEntity777 moves the trailing on-ground byte to a properties
// varint after the entity id (on-ground bit, step count 0): move_entity_pos
// is id, three i16, bool; move_entity_pos_rot adds yaw and pitch bytes
// before the bool.
func rewriteMoveEntity777(rot bool) bodyFn {
	tail := 6
	if rot {
		tail = 8
	}
	return func(_ State, body []byte) []byte {
		r := bytes.NewReader(body)
		var out []byte
		if !copyVarInt(r, &out) || r.Len() != tail+1 {
			return body
		}
		rest := body[len(body)-r.Len():]
		out = AppendVarInt(out, int32(rest[tail]&1))
		return append(out, rest[:tail]...)
	}
}

// rewriteEntityPositionSync777: id, position (3 f64), delta movement (3
// f64), yaw f32, pitch f32, on-ground → id, path type 0 (linear), position,
// yaw, pitch, on-ground.
func rewriteEntityPositionSync777(_ State, body []byte) []byte {
	r := bytes.NewReader(body)
	var out []byte
	if !copyVarInt(r, &out) || r.Len() != 24+24+8+1 {
		return body
	}
	rest := body[len(body)-r.Len():]
	out = append(out, 0) // PositionPath.Type.LINEAR
	out = append(out, rest[:24]...)
	return append(out, rest[48:]...)
}

// rewriteLevelParticles777: bool, bool, 3 f64, 3 f32 offsets, f32 speed,
// i32 count, particle varint + payload → particle + payload, bool, bool,
// position, offsets, speed ×3, count, randomization type 0.
func rewriteLevelParticles777(_ State, body []byte) []byte {
	const prefix = 2 + 24 + 12 + 4 + 4
	if len(body) < prefix+1 {
		return body
	}
	out := make([]byte, 0, len(body)+10)
	out = append(out, body[prefix:]...)        // particle id + payload
	out = append(out, body[:2+24+12]...)       // flags, position, offsets
	speed := body[2+24+12 : 2+24+12+4]         //
	out = append(out, speed...)                // x max speed
	out = append(out, speed...)                // y
	out = append(out, speed...)                // z
	out = append(out, body[2+24+16:prefix]...) // count
	return AppendVarInt(out, 0)                // RandomizationType.DEFAULT
}

// rewriteAnimateActions777 renumbers the animate actions that stayed in
// the packet (wake up 2→0, critical hit 4→1, magic critical hit 5→2); the
// swings (0 main hand, 3 off hand) are retargeted to swing_animation by
// swingAnimation777 before the step runs.
func rewriteAnimateActions777(_ State, body []byte) []byte {
	if len(body) < 2 {
		return body
	}
	out := append([]byte(nil), body...)
	switch out[len(out)-1] {
	case 2:
		out[len(out)-1] = 0
	case 4:
		out[len(out)-1] = 1
	case 5:
		out[len(out)-1] = 2
	}
	return out
}

// swingAnimation777 is the animate swing as 26.3's swing_animation packet:
// entity id, hand (0 main, 1 off), SwingAnimation.DEFAULT (type WHACK = 1,
// duration 6). ok is false when the animate is not a swing.
func swingAnimation777(body []byte) ([]byte, bool) {
	if len(body) < 2 {
		return nil, false
	}
	var hand int32
	switch body[len(body)-1] {
	case 0:
		hand = 0
	case 3:
		hand = 1
	default:
		return nil, false
	}
	out := append([]byte(nil), body[:len(body)-1]...)
	out = AppendVarInt(out, hand)
	out = AppendVarInt(out, 1) // SwingAnimationType.WHACK
	return AppendVarInt(out, 6), true
}

// rewriteSignUpdate777: pos i64, four strings, slot varint (0 back, 1
// front) → pos, is-front bool, four strings.
func rewriteSignUpdate777(_ State, body []byte) []byte {
	r := bytes.NewReader(body)
	var pos [8]byte
	if _, err := r.Read(pos[:]); err != nil {
		return body
	}
	var lines []string
	for i := 0; i < 4; i++ {
		s, err := ReadString(r)
		if err != nil {
			return body
		}
		lines = append(lines, s)
	}
	slot, err := ReadVarInt(r)
	if err != nil {
		return body
	}
	out := append([]byte(nil), pos[:]...)
	out = AppendBool(out, slot == 1)
	for _, s := range lines {
		out = AppendString(out, s)
	}
	return out
}

// rewriteAcceptTeleportation777 keeps only the teleport id; 26.3 echoes the
// accepted position and rotation after it.
func rewriteAcceptTeleportation777(_ State, body []byte) []byte {
	r := bytes.NewReader(body)
	var out []byte
	if !copyVarInt(r, &out) {
		return body
	}
	return out
}

// rewritePunchToSwing777 gives 26.3's payload-free punch the main-hand byte
// the canonical swing carries.
func rewritePunchToSwing777(_ State, body []byte) []byte {
	return []byte{0}
}
