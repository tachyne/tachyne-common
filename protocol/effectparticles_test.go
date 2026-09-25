package protocol

import (
	"bytes"
	"testing"
)

// A living entity's effect swirls (DATA_EFFECT_PARTICLES, index 10): the
// serializer is 18 canonically and 17 from 773 (COMPOUND_TAG's removal), and
// each particle id moves to the client's registry — entity_effect keeps
// its ARGB payload, item_slime (Oozing) has none.
func TestEffectParticlesMeta(t *testing.T) {
	body := AppendVarInt(nil, 42)
	body = append(body, 10)
	body = AppendVarInt(body, metaTypeParticles)
	body = AppendVarInt(body, 2)
	body = AppendVarInt(body, particleEntityEffect770)
	body = append(body, 0xff, 0x33, 0x99, 0xcc)
	body = AppendVarInt(body, particleItemSlime770)
	body = append(body, 11)
	body = AppendVarInt(body, metaTypeBoolean)
	body = append(body, 0, 0xff)

	for _, tc := range []struct {
		version       int32
		effect, slime byte
	}{{776, 28, 59}, {777, 28, 62}} {
		want := AppendVarInt(nil, 42)
		want = append(want, 10, 17, 2, tc.effect, 0xff, 0x33, 0x99, 0xcc, tc.slime, 11, 8, 0, 0xff)
		if got := remapEntityMeta(tc.version, body); !bytes.Equal(got, want) {
			t.Errorf("v%d: %x, want %x", tc.version, got, want)
		}
	}
	// The ageable index shift walks it without bailing (and leaves 10/11 be).
	if got := ShiftAgeableMobMeta(776, body); !bytes.Equal(got, body) {
		t.Errorf("the ageable shift changed a swirl-only frame: %x", got)
	}
	// An unknown particle bails rather than guessing its payload.
	bad := append(AppendVarInt(nil, 42), 10)
	bad = AppendVarInt(bad, metaTypeParticles)
	bad = append(bad, 1, 7, 0xff)
	if got := remapEntityMeta(777, bad); !bytes.Equal(got, bad) {
		t.Errorf("an unknown particle was rewritten: %x", got)
	}
}
