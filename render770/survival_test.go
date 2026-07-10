package render770

import (
	"testing"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// Oracles: the gomc hub's survival builders at deletion time (stage 3).

func TestHealthMatchesOracle(t *testing.T) {
	want := protocol.AppendF32(nil, 17.5)
	want = protocol.AppendVarInt(want, 18)
	want = protocol.AppendF32(want, 3.2)
	eq(t, "health", Health(attach.Health{Health: 17.5, Food: 18, Saturation: 3.2}), IDUpdateHealth, want)
}

func TestXPMatchesOracle(t *testing.T) {
	want := protocol.AppendF32(nil, 0.4)
	want = protocol.AppendVarInt(want, 7)
	want = protocol.AppendVarInt(want, 112)
	eq(t, "xp", XP(attach.XP{Progress: 0.4, Level: 7, Total: 112}), IDSetExperience, want)
}

func TestEffectMatchesOracle(t *testing.T) {
	want := protocol.AppendVarInt(nil, 5)
	want = protocol.AppendVarInt(want, 10) // regen
	want = protocol.AppendVarInt(want, 1)
	want = protocol.AppendVarInt(want, 600)
	want = protocol.AppendU8(want, 0x02)
	eq(t, "effect add", Effect(attach.Effect{EID: 5, ID: 10, Amp: 1, Ticks: 600}), IDEntityEffect, want)

	wantRm := protocol.AppendVarInt(nil, 5)
	wantRm = protocol.AppendVarInt(wantRm, 10)
	eq(t, "effect remove", Effect(attach.Effect{EID: 5, ID: 10, Remove: true}), IDRemoveEffect, wantRm)
}

func TestHurtDeathMatchOracle(t *testing.T) {
	want := protocol.AppendVarInt(nil, 9)
	want = protocol.AppendF32(want, 123.5)
	eq(t, "hurt", Hurt(attach.Hurt{EID: 9, Yaw: 123.5}), IDHurtAnimation, want)

	wantD := protocol.AppendVarInt(nil, 9)
	wantD = append(wantD, oracleChatNBT("You died")...)
	eq(t, "death", Death(attach.Death{EID: 9, Message: "You died"}), IDDeathCombat, wantD)
}
