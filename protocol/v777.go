package protocol

import (
	"bytes"
	"io"
)

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
	updateAdvancements776  = 130
	levelChunkWithLight776 = 45
	lightUpdate776         = 48
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
				login776:               rewriteLoginPrevGameMode777,
				respawn776:             rewriteRespawnPrevGameMode777,
				moveEntityPos776:       rewriteMoveEntity777(false),
				moveEntityPosRot776:    rewriteMoveEntity777(true),
				entityPositionSync776:  rewriteEntityPositionSync777,
				levelParticles776:      rewriteLevelParticles777,
				animate776:             rewriteAnimateActions777,
				updateAdvancements776:  rewriteAdvancementsPositioned777,
				levelChunkWithLight776: rewriteChunkLightBitSets777,
				lightUpdate776:         rewriteLightUpdateBitSets777,
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
// position, offsets, speed ×3, count, randomization type 0. 26.3 reads the
// count as a VarInt (26.2 still an int): copying the four bytes left the
// client three over, and it dropped the connection on the first particle.
func rewriteLevelParticles777(_ State, body []byte) []byte {
	const prefix = 2 + 24 + 12 + 4 + 4
	if len(body) < prefix+1 {
		return body
	}
	out := make([]byte, 0, len(body)+10)
	out = append(out, body[prefix:]...)  // particle id + payload
	out = append(out, body[:2+24+12]...) // flags, position, offsets
	speed := body[2+24+12 : 2+24+12+4]   //
	out = append(out, speed...)          // x max speed
	out = append(out, speed...)          // y
	out = append(out, speed...)          // z
	c := body[2+24+16 : prefix]
	count := int32(uint32(c[0])<<24 | uint32(c[1])<<16 | uint32(c[2])<<8 | uint32(c[3]))
	out = AppendVarInt(out, count) // count: VarInt in 26.3
	return AppendVarInt(out, 0)    // RandomizationType.DEFAULT
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

// rewriteAdvancementsPositioned777: 26.3 moved an advancement's tree
// position out of its DisplayInfo (…flags, [background], f32 x, f32 y) into
// a PositionedAdvancement wrapper (holder, f32 x, f32 y) around each added
// entry. Per entry the two floats are cut from the display and appended
// after the entry; an entry without a display gets (0, 0). Icons are the
// 26.1+ ItemStackTemplate with no components (what the renderer composes);
// anything richer bails out untouched, like the other walkers.
func rewriteAdvancementsPositioned777(_ State, body []byte) []byte {
	r := bytes.NewReader(body)
	out := make([]byte, 0, len(body)+64)
	pos := func() int { return len(body) - r.Len() }
	flushed := 0
	if _, err := r.ReadByte(); err != nil { // reset
		return body
	}
	n, err := ReadVarInt(r)
	if err != nil || n < 0 {
		return body
	}
	skipString := func() bool {
		l, err := ReadVarInt(r)
		if err != nil || l < 0 || int(l) > r.Len() {
			return false
		}
		return skipN(r, int64(l)) == nil
	}
	var zero [8]byte
	for i := int32(0); i < n; i++ {
		xy := zero[:]
		if !skipString() { // id
			return body
		}
		hasParent, err := r.ReadByte()
		if err != nil {
			return body
		}
		if hasParent != 0 && !skipString() {
			return body
		}
		hasDisplay, err := r.ReadByte()
		if err != nil {
			return body
		}
		if hasDisplay != 0 {
			if SkipNetworkNBT(r) != nil || SkipNetworkNBT(r) != nil { // title, description
				return body
			}
			item, e1 := ReadVarInt(r) // ItemStackTemplate: item, count, components
			count, e2 := ReadVarInt(r)
			addC, e3 := ReadVarInt(r)
			remC, e4 := ReadVarInt(r)
			if e1 != nil || e2 != nil || e3 != nil || e4 != nil || item < 0 || count < 0 || addC != 0 || remC != 0 {
				return body
			}
			if _, err := ReadVarInt(r); err != nil { // frame
				return body
			}
			var flags [4]byte
			if _, err := io.ReadFull(r, flags[:]); err != nil {
				return body
			}
			if flags[3]&1 != 0 && !skipString() { // background
				return body
			}
			// Cut x, y out of the display: flush up to here, skip them.
			out = append(out, body[flushed:pos()]...)
			if r.Len() < 8 {
				return body
			}
			xy = body[pos() : pos()+8]
			if skipN(r, 8) != nil {
				return body
			}
			flushed = pos()
		}
		ng, err := ReadVarInt(r) // requirements groups
		if err != nil || ng < 0 {
			return body
		}
		for g := int32(0); g < ng; g++ {
			nc, err := ReadVarInt(r)
			if err != nil || nc < 0 {
				return body
			}
			for c := int32(0); c < nc; c++ {
				if !skipString() {
					return body
				}
			}
		}
		if _, err := r.ReadByte(); err != nil { // telemetry
			return body
		}
		out = append(out, body[flushed:pos()]...)
		out = append(out, xy...) // PositionedAdvancement x, y
		flushed = pos()
	}
	out = append(out, body[flushed:]...) // removed + progress + showAdvancements: unchanged
	return out
}

// 26.3's light data codes its four masks with ByteBufCodecs.BIT_SET, which
// reads a BYTE array (BitSet.valueOf(byte[])); 26.2 read a long array. The
// bits are the same (both little-endian), so each mask becomes the longs'
// little-endian bytes. A 26.3 client had stopped 8n-n bytes short of every
// chunk and reported the rest as "larger than I expected".

// rewriteBitSets777 converts count masks at r's position into out.
func rewriteBitSets777(r *bytes.Reader, out *[]byte, count int) bool {
	for i := 0; i < count; i++ {
		n, err := ReadVarInt(r)
		if err != nil || n < 0 || int(n)*8 > r.Len() {
			return false
		}
		*out = AppendVarInt(*out, n*8)
		var l [8]byte
		for j := int32(0); j < n; j++ {
			if _, err := io.ReadFull(r, l[:]); err != nil {
				return false
			}
			for k := 7; k >= 0; k-- { // big-endian i64 on the wire → little-endian bytes
				*out = append(*out, l[k])
			}
		}
	}
	return true
}

// rewriteLightUpdateBitSets777: i32 x, i32 z, four masks, two lists.
func rewriteLightUpdateBitSets777(_ State, body []byte) []byte {
	if len(body) < 8 {
		return body
	}
	r := bytes.NewReader(body[8:])
	out := append([]byte(nil), body[:8]...)
	if !rewriteBitSets777(r, &out, 4) {
		return body
	}
	rest := make([]byte, r.Len())
	r.Read(rest)
	return append(out, rest...)
}

// rewriteChunkLightBitSets777: i32 x, i32 z, heightmaps (varint n × (varint
// type, varint longs, longs)), buffer (varint length, bytes), block entities
// (varint n × (byte, i16, varint type, NBT)), then the light data as above.
func rewriteChunkLightBitSets777(_ State, body []byte) []byte {
	r := bytes.NewReader(body)
	pos := func() int { return len(body) - r.Len() }
	if skipN(r, 8) != nil {
		return body
	}
	n, err := ReadVarInt(r) // heightmaps
	if err != nil || n < 0 {
		return body
	}
	for i := int32(0); i < n; i++ {
		if _, err := ReadVarInt(r); err != nil { // type
			return body
		}
		l, err := ReadVarInt(r)
		if err != nil || l < 0 || skipN(r, int64(l)*8) != nil {
			return body
		}
	}
	l, err := ReadVarInt(r) // section buffer
	if err != nil || l < 0 || skipN(r, int64(l)) != nil {
		return body
	}
	n, err = ReadVarInt(r) // block entities
	if err != nil || n < 0 {
		return body
	}
	for i := int32(0); i < n; i++ {
		if skipN(r, 3) != nil { // packed xz, y
			return body
		}
		if _, err := ReadVarInt(r); err != nil { // type
			return body
		}
		if SkipNetworkNBT(r) != nil { // tag (TAG_End when absent)
			return body
		}
	}
	out := append([]byte(nil), body[:pos()]...)
	if !rewriteBitSets777(r, &out, 4) {
		return body
	}
	rest := make([]byte, r.Len())
	r.Read(rest)
	return append(out, rest...)
}
