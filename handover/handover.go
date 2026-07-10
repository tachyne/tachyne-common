// Package handover carries the direct pod-to-pod (world↔world) transport for the
// sharded world: the peer-protocol frames and entity-state snapshots a pod sends
// a neighbour to migrate a player/mob/vehicle across a seam, plus the shadow push
// that makes an approaching player visible to a neighbour BEFORE the crossing.
// See tachyne-world/docs/SHARDING-BUILD.md (§4 handover, §4a awareness, §5).
//
// These are plain JSON value types. The peer link frames them with
// attach.WriteFrame/ReadFrame (u32be len | u8 type | payload); the type bytes
// below are LOCAL to the peer protocol and independent of the attach
// (gateway↔world) frame ids. Codec rule inherited from attach: ONE json tag per
// field, never a shared tag on a multi-field declaration, and no custom
// marshalers (encoding/json silently drops colliders otherwise).
package handover

// Peer-protocol frame type bytes (world↔world; independent of attach ids).
const (
	MsgPeerHello  = 0x01 // dialer→acceptor: PeerHello (identify + topo assert)
	MsgMigrate    = 0x02 // owner→neighbour: MigrateEntity
	MsgAck        = 0x03 // neighbour→owner: Ack
	MsgShadow     = 0x04 // owner→neighbour: Shadow (awareness pose push)
	MsgShadowGone = 0x05 // owner→neighbour: ShadowGone (left awareness range)
)

// PeerHello identifies a dialing pod to its neighbour, authenticates the link
// (shared token, like the attach protocol), and asserts topology agreement — a
// mixed-topology mesh must fail loudly at connect, not corrupt a seam later.
type PeerHello struct {
	SID   int32  `json:"sid"`
	Token string `json:"token"`
	Topo  string `json:"topo"` // shard.Map.TopoHash()
}

// Kind discriminates the payload of a MigrateEntity.
type Kind uint8

const (
	KindPlayer  Kind = 1
	KindMob     Kind = 2
	KindVehicle Kind = 3
)

// MigrateEntity transfers ownership of one entity to the neighbour. Exactly one
// of Player/Mob/Vehicle is set, matching Kind. The neighbour applies it and
// replies Ack{MigID, OK}; the owner releases the entity ONLY on OK
// (make-before-break). MigID is the idempotency key — a reconnect-driven resend
// must not double-apply. For a player, the gateway then reconnects to the
// neighbour with attach Hello{Purpose:"resume", ResumeToken==MigID} to re-bind
// the client to the migrated state.
type MigrateEntity struct {
	Kind    Kind          `json:"kind"`
	MigID   string        `json:"mig_id"`
	Player  *PlayerState  `json:"player,omitempty"`
	Mob     *MobState     `json:"mob,omitempty"`
	Vehicle *VehicleState `json:"vehicle,omitempty"`
}

// Ack confirms (OK) or rejects receipt of a MigrateEntity.
type Ack struct {
	MigID string `json:"mig_id"`
	OK    bool   `json:"ok"`
	Err   string `json:"err,omitempty"`
}

// Shadow is the awareness push: the owner streams an approaching entity's render
// pose to a neighbour so the neighbour's mobs aggro it and its players see it
// before the crossing. Read-only on the neighbour — the owner stays authoritative
// for the entity's state; cross-border hits route back to the owner.
//
// It carries BOTH players and mobs: Kind discriminates, and for a mob EType (the
// canonical entity-type id) and Baby let the neighbour spawn the right creature.
// Zero Kind decodes as KindPlayer for forward-compatibility with an older sender.
type Shadow struct {
	EID       int32    `json:"eid"`
	Kind      Kind     `json:"kind"`  // KindPlayer or KindMob (0 == player)
	EType     int32    `json:"etype"` // canonical entity type (mobs only)
	Name      string   `json:"name"`
	UUID      [16]byte `json:"uuid"`
	Dim       int32    `json:"dim"`
	X         float64  `json:"x"`
	Y         float64  `json:"y"`
	Z         float64  `json:"z"`
	Yaw       float32  `json:"yaw"`
	Pitch     float32  `json:"pitch"`
	HeadYaw   float32  `json:"head_yaw"`
	OnGround  bool     `json:"on_ground"`
	Sprinting bool     `json:"sprinting"`
	Sneaking  bool     `json:"sneaking"`
	Baby      bool     `json:"baby"`     // mob baby flag (render scale)
	Gamemode  int32    `json:"gamemode"` // player only: the neighbour's mobs hunt survival shadows, never creative/spectator
}

// ShadowGone tells a neighbour to drop a shadow (the player left the awareness
// band without crossing, or crossed and is now real there).
type ShadowGone struct {
	EID int32 `json:"eid"`
}
