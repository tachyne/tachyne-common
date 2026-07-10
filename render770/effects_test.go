package render770

import (
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
	want := protocol.AppendBool(nil, true)
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

func TestWorldFXAndBlockSet(t *testing.T) {
	want := protocol.AppendI32(nil, 2001)
	want = protocol.AppendPosition(want, 5, 64, -9)
	want = protocol.AppendI32(want, 86)
	want = protocol.AppendBool(want, false)
	eq(t, "worldfx", WorldFX(attach.WorldFX{Event: 2001, X: 5, Y: 64, Z: -9, Data: 86}), IDWorldEvent, want)

	wantB := protocol.AppendPosition(nil, 5, 64, -9)
	wantB = protocol.AppendVarInt(wantB, 86)
	eq(t, "blockset", BlockSet(attach.BlockSet{X: 5, Y: 64, Z: -9, State: 86}), IDBlockUpdate, wantB)
}
