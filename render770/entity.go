// Package render770 renders tachyne domain events (the attach frame types)
// into canonical Minecraft 1.21.5 (protocol 770) clientbound play packets.
// It is the single shared implementation: java gateways render attach frames
// through it (multi-version gateways then translate its output with the
// protocol translator chain), and the engine's legacy TCP path renders
// hub events through it until that path is deleted.
package render770

import (
	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// Canonical-770 clientbound play packet IDs for the families rendered here.
const (
	IDSpawnEntity   = 0x01
	IDEntitySync    = 0x1f // sync_entity_position — ABSOLUTE pos+vel+angles
	IDEntityMoveRot = 0x2f // entity_move_look — i16 relative deltas + angle bytes
	IDPlayerRemove  = 0x3e
	IDPlayerInfo    = 0x3f
	IDEntityDestroy = 0x46
	IDEntityHead    = 0x4c // entity_head_rotation
)

// Packet is one rendered clientbound packet: canonical 770 id + body,
// unframed (the caller owns framing/compression/translation).
type Packet struct {
	ID   int32
	Body []byte
}

// velUnit is the Spawn Entity velocity unit: 1/8000 block per tick.
const velUnit = 8000

// syncEvery is how many moves pass between absolute resyncs (2 s at 20 TPS).
const syncEvery = 40

// angleByte encodes degrees as Minecraft's 1/256-turn signed byte.
func angleByte(deg float32) byte { return byte(int32(deg * 256 / 360)) }

// EntityView renders one viewer's entity movement. Relative moves are deltas
// against what THIS viewer last saw, so a tracker lives per connection. A
// move renders as an absolute sync_entity_position on first sight, on a jump
// too big for the i16 delta, and every syncEvery-th move (self-healing any
// drift from packets the transport dropped); otherwise as a relative
// entity_move_look. Not safe for concurrent use: the owning writer goroutine
// renders everything for its connection.
type EntityView struct {
	pos  map[int32][3]float64
	sync map[int32]int
}

func NewEntityView() *EntityView {
	return &EntityView{pos: map[int32][3]float64{}, sync: map[int32]int{}}
}

// Reset forgets every entity — call on a dimension switch, where the client
// discards its entity world.
func (v *EntityView) Reset() {
	v.pos = map[int32][3]float64{}
	v.sync = map[int32]int{}
}

// Tracked returns the eids this viewer currently has rendered — the set to
// explicitly destroy on a SILENT world swap (a shard crossing without a Respawn,
// where the client keeps its chunks and entities must be reconciled by hand).
func (v *EntityView) Tracked() []int32 {
	out := make([]int32, 0, len(v.pos))
	for id := range v.pos {
		out = append(out, id)
	}
	return out
}

// Render renders one domain event; ok=false means no renderer exists for the
// value's type (an unconverted family, or not an event at all).
func (v *EntityView) Render(ev any) (Packet, bool) {
	switch e := ev.(type) {
	case attach.EntityAdd:
		return v.Add(e), true
	case attach.EntityMove:
		return v.Move(e), true
	case attach.EntityHead:
		return Head(e), true
	case attach.EntityRemove:
		return v.Remove(e), true
	case attach.PlayerInfo:
		return PlayerInfoAdd(e), true
	case attach.PlayerGone:
		return PlayerRemove(e), true
	case attach.Chat:
		return Chat(e), true
	case attach.BossBar:
		return BossBar(e), true
	case attach.Time:
		return Time(e), true
	case attach.Health:
		return Health(e), true
	case attach.XP:
		return XP(e), true
	case attach.Effect:
		return Effect(e), true
	case attach.Hurt:
		return Hurt(e), true
	case attach.Death:
		return Death(e), true
	case attach.Equipment:
		return Equipment(e), true
	case attach.EntityMeta:
		return EntityMeta(e), true
	case attach.WindowOpen:
		return WindowOpen(e), true
	case attach.WindowItems:
		return WindowItems(e), true
	case attach.WindowSlot:
		return WindowSlot(e), true
	case attach.WindowData:
		return WindowData(e), true
	case attach.HeldSync:
		return HeldSync(e), true
	case attach.Collect:
		return Collect(e), true
	case attach.Sound:
		return Sound(e), true
	case attach.Particles:
		return Particles(e), true
	case attach.WorldFX:
		return WorldFX(e), true
	case attach.BlockSet:
		return BlockSet(e), true
	case attach.GameEvent:
		return GameEvent(e), true
	case attach.Abilities:
		return Abilities(e), true
	case attach.Passengers:
		return Passengers(e), true
	case attach.VehicleMove:
		return VehicleMove(e), true
	case attach.Velocity:
		return Velocity(e), true
	case attach.Trades:
		return Trades(e), true
	case attach.CursorItem:
		return CursorItem(e), true
	case attach.Difficulty:
		return Difficulty(e), true
	case attach.CommandTree:
		return CommandTree(e), true
	case attach.EntityStatus:
		return EntityStatus(e), true
	case attach.Swing:
		return Swing(e), true
	}
	return Packet{}, false
}

// Add records the entity and renders Spawn Entity.
func (v *EntityView) Add(e attach.EntityAdd) Packet {
	v.pos[e.EID] = [3]float64{e.X, e.Y, e.Z}
	b := protocol.AppendVarInt(nil, e.EID)
	b = append(b, e.UUID[:]...)
	b = protocol.AppendVarInt(b, e.Type)
	b = protocol.AppendF64(b, e.X)
	b = protocol.AppendF64(b, e.Y)
	b = protocol.AppendF64(b, e.Z)
	b = protocol.AppendU8(b, angleByte(e.Pitch))
	b = protocol.AppendU8(b, angleByte(e.Yaw))
	b = protocol.AppendU8(b, angleByte(e.Yaw)) // head yaw
	b = protocol.AppendVarInt(b, e.Data)
	b = protocol.AppendI16(b, int16(e.VX*velUnit))
	b = protocol.AppendI16(b, int16(e.VY*velUnit))
	b = protocol.AppendI16(b, int16(e.VZ*velUnit))
	return Packet{IDSpawnEntity, b}
}

// Move renders a movement event: relative when this viewer has a recent
// baseline, absolute otherwise (see EntityView). NoSync movement never
// resyncs (except on first sight, where there is no baseline to be relative
// to): oversized deltas saturate the i16 and the baseline advances by exactly
// what was sent, so the entity converges over the next moves instead.
func (v *EntityView) Move(e attach.EntityMove) Packet {
	last, known := v.pos[e.EID]
	resync := !known
	if !e.NoSync {
		v.sync[e.EID]++
		resync = !known || v.sync[e.EID]%syncEvery == 0 ||
			abs(e.X-last[0]) >= 7.5 || abs(e.Y-last[1]) >= 7.5 || abs(e.Z-last[2]) >= 7.5
	}
	v.pos[e.EID] = [3]float64{e.X, e.Y, e.Z}
	if resync {
		b := protocol.AppendVarInt(nil, e.EID)
		b = protocol.AppendF64(b, e.X)
		b = protocol.AppendF64(b, e.Y)
		b = protocol.AppendF64(b, e.Z)
		b = protocol.AppendF64(b, 0) // velocity — 0: relative moves carry motion
		b = protocol.AppendF64(b, 0)
		b = protocol.AppendF64(b, 0)
		b = protocol.AppendF32(b, e.Yaw)
		b = protocol.AppendF32(b, e.Pitch)
		b = protocol.AppendBool(b, e.OnGround)
		return Packet{IDEntitySync, b}
	}
	dx := clampDelta(e.X - last[0])
	dy := clampDelta(e.Y - last[1])
	dz := clampDelta(e.Z - last[2])
	if e.NoSync { // advance by exactly what we send, so saturation converges
		v.pos[e.EID] = [3]float64{
			last[0] + float64(dx)/4096, last[1] + float64(dy)/4096, last[2] + float64(dz)/4096}
	}
	b := protocol.AppendVarInt(nil, e.EID)
	b = protocol.AppendI16(b, dx)
	b = protocol.AppendI16(b, dy)
	b = protocol.AppendI16(b, dz)
	b = protocol.AppendU8(b, angleByte(e.Yaw))
	b = protocol.AppendU8(b, angleByte(e.Pitch))
	b = protocol.AppendBool(b, e.OnGround)
	return Packet{IDEntityMoveRot, b}
}

// clampDelta converts a block delta to the i16 1/4096-block fixed point used
// by the relative-move packet, saturating rather than overflowing.
func clampDelta(d float64) int16 {
	v := d * 4096
	if v > 32767 {
		return 32767
	}
	if v < -32768 {
		return -32768
	}
	return int16(v)
}

// Remove forgets the entities and renders Remove Entities.
func (v *EntityView) Remove(e attach.EntityRemove) Packet {
	b := protocol.AppendVarInt(nil, int32(len(e.EIDs)))
	for _, id := range e.EIDs {
		delete(v.pos, id)
		delete(v.sync, id)
		b = protocol.AppendVarInt(b, id)
	}
	return Packet{IDEntityDestroy, b}
}

// Head renders entity_head_rotation.
func Head(e attach.EntityHead) Packet {
	b := protocol.AppendVarInt(nil, e.EID)
	return Packet{IDEntityHead, protocol.AppendU8(b, angleByte(e.Yaw))}
}

// playerInfoAddFlags: add player | update gamemode | update listed | update
// latency — the combination the vanilla client expects for a fresh entry.
const playerInfoAddFlags = 0x01 | 0x04 | 0x08 | 0x10

// PlayerInfoAdd renders a tab-list add (with skin properties when present).
func PlayerInfoAdd(e attach.PlayerInfo) Packet {
	b := protocol.AppendU8(nil, playerInfoAddFlags)
	b = protocol.AppendVarInt(b, 1) // one entry
	b = append(b, e.UUID[:]...)
	b = protocol.AppendString(b, e.Name)
	b = protocol.AppendVarInt(b, int32(len(e.Props)))
	for _, pr := range e.Props { // the textures blob = other clients render skins
		b = protocol.AppendString(b, pr.Name)
		b = protocol.AppendString(b, pr.Value)
		b = protocol.AppendBool(b, pr.Signature != "")
		if pr.Signature != "" {
			b = protocol.AppendString(b, pr.Signature)
		}
	}
	b = protocol.AppendVarInt(b, 1) // gamemode = creative
	b = protocol.AppendVarInt(b, 1) // listed = true
	b = protocol.AppendVarInt(b, 0) // latency = 0 ms
	return Packet{IDPlayerInfo, b}
}

// PlayerRemove renders a tab-list remove.
func PlayerRemove(e attach.PlayerGone) Packet {
	b := protocol.AppendVarInt(nil, 1)
	return Packet{IDPlayerRemove, append(b, e.UUID[:]...)}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
