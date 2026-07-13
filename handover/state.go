package handover

// invSlots is the survival inventory size (hotbar 0-8 + main 9-35), mirroring
// the engine's invSize. A slot/armor/offhand row is [item, count, dmg, ench] —
// the engine's existing pack (invstore.packStack), so the mapping is a copy.
const invSlots = 36

// PlayerState is the full handover snapshot of a player. It mirrors the engine's
// LIVE `tracked` state (not the on-disk subset) — health/food/effects/fire are
// hub-live-only and would reset on a crossing if omitted. See SHARDING-BUILD.md
// §5 for the field-selection rules.
//
// v1 losses (documented, "at most as lossy as a relog"): item name/potion are
// not carried (the row pack drops them, as relog does today); fine-grained
// tick cooldowns (attack/bow/shield/grace) reset on the destination pod's clock;
// anti-cheat counters reset. Effects and fire are carried because they store
// remaining time directly, not an absolute per-pod tick.
type PlayerState struct {
	EID   int32    `json:"eid"` // session-stable player-lane eid (shard.MintEID(_, PlayerSID))
	Name  string   `json:"name"`
	UUID  [16]byte `json:"uuid"`
	Dim   int32    `json:"dim"`
	X     float64  `json:"x"`
	Y     float64  `json:"y"`
	Z     float64  `json:"z"`
	Yaw   float32  `json:"yaw"`
	Pitch float32  `json:"pitch"`

	OnGround  bool    `json:"on_ground"`
	Sprinting bool    `json:"sprinting"`
	Airborne  bool    `json:"airborne"` // fall-damage calc
	PeakY     float64 `json:"peak_y"`   // fall-damage peak height

	Gamemode int32 `json:"gamemode"`

	Health     float32 `json:"health"`
	Absorption float32 `json:"absorption"`
	Food       int32   `json:"food"`
	Saturation float32 `json:"saturation"`
	Exhaustion float32 `json:"exhaustion"`
	Air        int32   `json:"air"`
	FireSecs   int32   `json:"fire_secs"`

	XPLevel  int32 `json:"xp_level"`
	XPPoints int32 `json:"xp_points"`

	Effects []EffectState `json:"effects"`

	// Inventory — the engine's stack pack, savedInv-compatible:
	// [item, count, dmg, enchPack, mapID, 6×bannerLayer, trimPack, bookID].
	// Old shorter rows zero-fill on decode (JSON arrays).
	Slots   [invSlots][13]int32 `json:"slots"`
	Armor   [4][13]int32        `json:"armor"`
	Offhand [13]int32           `json:"offhand"`

	BedSpawn *[3]int32 `json:"bed_spawn,omitempty"` // nil = no bed spawn set
}

// EffectState is one active status effect. Left is SECONDS remaining (already
// relative — no per-pod tick conversion needed), Amp the 0-based amplifier.
type EffectState struct {
	ID   int32 `json:"id"`
	Amp  int32 `json:"amp"`
	Left int32 `json:"left"`
}

// VehicleState is a boat/minecart snapshot. Vehicles are memory-only in the
// engine (no disk store), so the transport carries the whole thing. Rider is a
// player eid (0 = empty) and is REMAPPED to the destination pod's eid space.
type VehicleState struct {
	EID   int32    `json:"eid"`
	Dim   int32    `json:"dim"`
	UUID  [16]byte `json:"uuid"`
	EType int32    `json:"etype"` // minecart ordinal or boat-per-wood ordinal
	X     float64  `json:"x"`
	Y     float64  `json:"y"`
	Z     float64  `json:"z"`
	Yaw   float32  `json:"yaw"`
	Rider int32    `json:"rider"` // player eid, 0 = empty (remapped on dest)
}

// MobState is a mob snapshot. v1 CORE fields only — enough to migrate the common
// mob shape; species-specific state, pathfinding, villager offers, and the
// `behavior` interface (re-resolved from EType on the destination, never
// marshaled) are fleshed out in PR3 with their own round-trip test. Cross-entity
// eid refs (Owner/Rider/Riders) are remapped on the destination pod.
type MobState struct {
	EID     int32    `json:"eid"`
	EType   int32    `json:"etype"`
	UUID    [16]byte `json:"uuid"`
	Dim     int32    `json:"dim"`
	X       float64  `json:"x"`
	Y       float64  `json:"y"`
	Z       float64  `json:"z"`
	Yaw     float32  `json:"yaw"`
	HeadYaw float32  `json:"head_yaw"`
	VX      float64  `json:"vx"`
	VY      float64  `json:"vy"`
	VZ      float64  `json:"vz"`

	Health  int32 `json:"health"`
	Hostile bool  `json:"hostile"`

	Baby    bool    `json:"baby"`
	Tamed   bool    `json:"tamed"`
	Sitting bool    `json:"sitting"`
	Owner   int32   `json:"owner"` // player eid (remapped)
	Saddled bool    `json:"saddled"`
	Rider   int32   `json:"rider"`            // player eid (remapped)
	Riders  []int32 `json:"riders,omitempty"` // player eids (remapped)
	Harness int32   `json:"harness"`
}
