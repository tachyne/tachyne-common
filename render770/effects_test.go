package render770

import (
	"encoding/json"
	"testing"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// Oracles: the gomc hub's sound/particle/world-event builders at deletion
// time (stage 5).

func TestSoundMatchesOracle(t *testing.T) {
	want := protocol.AppendVarInt(nil, 0)
	want = protocol.AppendString(want, "minecraft:entity.player.hurt")
	want = protocol.AppendBool(want, false)
	want = protocol.AppendVarInt(want, 7)
	want = protocol.AppendI32(want, int32(10.5*8))
	want = protocol.AppendI32(want, int32(64.0*8))
	want = protocol.AppendI32(want, int32(-3.25*8))
	want = protocol.AppendF32(want, 1)
	want = protocol.AppendF32(want, 0.95)
	want = protocol.AppendI64(want, 0)
	eq(t, "sound", Sound(attach.Sound{Name: "minecraft:entity.player.hurt", Category: 7,
		X: 10.5, Y: 64, Z: -3.25, Volume: 1, Pitch: 0.95}), IDSoundEffect, want)
}

func TestParticlesMatchesOracle(t *testing.T) {
	// The engine's old builder sent overrideLimiter true on every burst;
	// the frame now says (Force), and sendParticles' ordinary case is false.
	want := protocol.AppendBool(nil, false)
	want = protocol.AppendBool(want, false)
	want = protocol.AppendF64(want, 1)
	want = protocol.AppendF64(want, 2)
	want = protocol.AppendF64(want, 3)
	want = protocol.AppendF32(want, 0.5)
	want = protocol.AppendF32(want, 0.5)
	want = protocol.AppendF32(want, 0.5)
	want = protocol.AppendF32(want, 0.1)
	want = protocol.AppendI32(want, 12)
	want = protocol.AppendVarInt(want, 21)
	eq(t, "particles", Particles(attach.Particles{PID: 21, X: 1, Y: 2, Z: 3,
		Spread: 0.5, Speed: 0.1, Count: 12}), IDParticles, want)
}

// ClientboundLevelParticlesPacket field by field: overrideLimiter,
// alwaysShow, x/y/z DOUBLE, xDist/yDist/zDist FLOAT, maxSpeed FLOAT, count
// INT, then the particle (ParticleTypes.STREAM_CODEC: id, options).
func particlesHead(force, always bool, dx, dy, dz float32, pid int32) []byte {
	b := protocol.AppendBool(nil, force)
	b = protocol.AppendBool(b, always)
	b = protocol.AppendF64(b, 1)
	b = protocol.AppendF64(b, 2)
	b = protocol.AppendF64(b, 3)
	b = protocol.AppendF32(b, dx)
	b = protocol.AppendF32(b, dy)
	b = protocol.AppendF32(b, dz)
	b = protocol.AppendF32(b, 0)
	b = protocol.AppendI32(b, 1)
	return protocol.AppendVarInt(b, pid)
}

func TestParticlesOffsetsAndFlags(t *testing.T) {
	eq(t, "three offsets", Particles(attach.Particles{PID: 21, X: 1, Y: 2, Z: 3, Spread: 9,
		DX: 0.25, DY: 0.5, DZ: 0.75, Count: 1, Force: true}), IDParticles,
		particlesHead(true, false, 0.25, 0.5, 0.75, 21))
	eq(t, "always show", Particles(attach.Particles{PID: 21, X: 1, Y: 2, Z: 3, Count: 1, AlwaysShow: true}),
		IDParticles, particlesHead(false, true, 0, 0, 0, 21))
}

func TestParticleOptions(t *testing.T) {
	p := func(pid int32, o *attach.ParticleOptions) Packet {
		return Particles(attach.Particles{PID: pid, X: 1, Y: 2, Z: 3, Count: 1, Options: o})
	}
	head := func(pid int32) []byte { return particlesHead(false, false, 0, 0, 0, pid) }
	// DustParticleOptions: INT colour, FLOAT scale.
	w := protocol.AppendF32(protocol.AppendI32(head(protocol.ParticleDust770), 0xff0000), 2)
	eq(t, "dust", p(protocol.ParticleDust770, &attach.ParticleOptions{Color: 0xff0000, Scale: 2}), IDParticles, w)
	// DustColorTransitionOptions: INT from, INT to, FLOAT scale.
	w = protocol.AppendF32(protocol.AppendI32(protocol.AppendI32(head(protocol.ParticleDustColorTransition770), 1), 2), 3)
	eq(t, "dust_color_transition", p(protocol.ParticleDustColorTransition770, &attach.ParticleOptions{Color: 1, ToColor: 2, Scale: 3}), IDParticles, w)
	// BlockParticleOption: a block-state id.
	w = protocol.AppendVarInt(head(protocol.ParticleFallingDust770), 300)
	eq(t, "falling_dust", p(protocol.ParticleFallingDust770, &attach.ParticleOptions{State: 300}), IDParticles, w)
	// ColorParticleOption: one ARGB INT.
	w = protocol.AppendI32(head(protocol.ParticleEntityEffect770), -1)
	eq(t, "entity_effect", p(protocol.ParticleEntityEffect770, &attach.ParticleOptions{Color: -1}), IDParticles, w)
	// SculkChargeParticleOptions: FLOAT roll; ShriekParticleOption: VAR_INT delay.
	eq(t, "sculk_charge", p(protocol.ParticleSculkCharge770, &attach.ParticleOptions{Roll: 1.5}), IDParticles,
		protocol.AppendF32(head(protocol.ParticleSculkCharge770), 1.5))
	eq(t, "shriek", p(protocol.ParticleShriek770, &attach.ParticleOptions{Delay: 200}), IDParticles,
		protocol.AppendVarInt(head(protocol.ParticleShriek770), 200))
	// TrailParticleOption: Vec3 target, INT colour, VAR_INT duration.
	w = protocol.AppendF64(protocol.AppendF64(protocol.AppendF64(head(protocol.ParticleTrail770), 4), 5), 6)
	w = protocol.AppendVarInt(protocol.AppendI32(w, 0x00ff00), 30)
	eq(t, "trail", p(protocol.ParticleTrail770, &attach.ParticleOptions{TX: 4, TY: 5, TZ: 6, Color: 0x00ff00, Ticks: 30}), IDParticles, w)
	// VibrationParticleOption: PositionSource (type 0 block + BlockPos, or
	// type 1 entity + VAR_INT id + FLOAT offset), VAR_INT ticks.
	w = protocol.AppendVarInt(protocol.AppendPosition(protocol.AppendVarInt(head(protocol.ParticleVibration770), 0), 4, 5, -7), 10)
	eq(t, "vibration block", p(protocol.ParticleVibration770, &attach.ParticleOptions{TX: 4.5, TY: 5.9, TZ: -6.5, Ticks: 10}), IDParticles, w)
	w = protocol.AppendVarInt(protocol.AppendF32(protocol.AppendVarInt(protocol.AppendVarInt(head(protocol.ParticleVibration770), 1), 99), 1.2), 10)
	eq(t, "vibration entity", p(protocol.ParticleVibration770, &attach.ParticleOptions{TargetEID: 99, TargetYOffset: 1.2, Ticks: 10}), IDParticles, w)
	// ItemParticleOption: an ItemStack (canonical Slot).
	st := attach.ItemStack{ID: 800, Count: 1}
	eq(t, "item", p(protocol.ParticleItem770, &attach.ParticleOptions{Item: &st}), IDParticles,
		AppendItemStack(head(protocol.ParticleItem770), st))
	// The 26.x-optioned types: nothing without Options, the tail with them.
	eq(t, "dragon_breath bare", p(protocol.ParticleDragonBreath770, nil), IDParticles, head(protocol.ParticleDragonBreath770))
	eq(t, "dragon_breath power", p(protocol.ParticleDragonBreath770, &attach.ParticleOptions{Power: 1}), IDParticles,
		protocol.AppendF32(head(protocol.ParticleDragonBreath770), 1))
	eq(t, "effect", p(protocol.ParticleEffect770, &attach.ParticleOptions{Color: 0xff00ff, Power: 0.5}), IDParticles,
		protocol.AppendF32(protocol.AppendI32(head(protocol.ParticleEffect770), 0xff00ff), 0.5))
	eq(t, "flash", p(protocol.ParticleFlash770, &attach.ParticleOptions{Color: -1}), IDParticles,
		protocol.AppendI32(head(protocol.ParticleFlash770), -1))
	// A type without options ignores them.
	eq(t, "simple", p(21, &attach.ParticleOptions{Color: 5}), IDParticles, head(21))
}

// A frame from an engine that predates the offsets and options still
// decodes and renders as before.
func TestParticlesOldFrame(t *testing.T) {
	var e attach.Particles
	if err := json.Unmarshal([]byte(`{"pid":21,"x":1,"y":2,"z":3,"spread":0.5,"speed":0,"count":1}`), &e); err != nil {
		t.Fatal(err)
	}
	eq(t, "old frame", Particles(e), IDParticles, particlesHead(false, false, 0.5, 0.5, 0.5, 21))
	var fx attach.WorldFX
	if err := json.Unmarshal([]byte(`{"event":1023,"x":1,"y":2,"z":3,"data":0}`), &fx); err != nil || fx.Global {
		t.Fatalf("old world event: %v %+v", err, fx)
	}
}

func TestWorldFXAndBlockSet(t *testing.T) {
	want := protocol.AppendI32(nil, 2001)
	want = protocol.AppendPosition(want, 5, 64, -9)
	want = protocol.AppendI32(want, 86)
	want = protocol.AppendBool(want, false)
	eq(t, "worldfx", WorldFX(attach.WorldFX{Event: 2001, X: 5, Y: 64, Z: -9, Data: 86}), IDWorldEvent, want)
	// ClientboundLevelEventPacket's last field is globalEvent (BOOLEAN).
	wantG := protocol.AppendI32(nil, 1028)
	wantG = protocol.AppendPosition(wantG, 5, 64, -9)
	wantG = protocol.AppendI32(wantG, 0)
	wantG = protocol.AppendBool(wantG, true)
	eq(t, "global worldfx", WorldFX(attach.WorldFX{Event: 1028, X: 5, Y: 64, Z: -9, Global: true}), IDWorldEvent, wantG)

	wantB := protocol.AppendPosition(nil, 5, 64, -9)
	wantB = protocol.AppendVarInt(wantB, 86)
	eq(t, "blockset", BlockSet(attach.BlockSet{X: 5, Y: 64, Z: -9, State: 86}), IDBlockUpdate, wantB)
}

func TestSoundEntityMatchesOracle(t *testing.T) {
	w := protocol.AppendVarInt(nil, 0)
	w = protocol.AppendString(w, "minecraft:entity.sheep.shear")
	w = protocol.AppendBool(w, false)
	w = protocol.AppendVarInt(w, 7) // players
	w = protocol.AppendVarInt(w, 42)
	w = protocol.AppendF32(w, 1)
	w = protocol.AppendF32(w, 1)
	w = protocol.AppendI64(w, 0)
	eq(t, "sound entity", SoundEntity(attach.Sound{Name: "minecraft:entity.sheep.shear", Category: 7, EID: 42, Volume: 1, Pitch: 1}), IDSoundEntity, w)
}
