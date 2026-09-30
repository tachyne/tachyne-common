package protocol

import (
	"bytes"
	"testing"
)

// The canonical level_particles prefix: two flags, position, offsets, speed,
// count — 46 bytes, the particle after it.
func particlesPrefix() []byte {
	b := AppendBool(nil, false)
	b = AppendBool(b, false)
	b = AppendF64(b, 1)
	b = AppendF64(b, 2)
	b = AppendF64(b, 3)
	b = AppendF32(b, 0.25)
	b = AppendF32(b, 0.5)
	b = AppendF32(b, 0.75)
	b = AppendF32(b, 0.1)
	return AppendI32(b, 9)
}

// Each option type's bytes after the id, written from the types'
// STREAM_CODECs (identical on 1.21.5, 26.2 and 26.3 unless noted): the
// options follow the (remapped) id unchanged, block states remapped.
func TestLevelParticleOptions(t *testing.T) {
	rgb := AppendI32(nil, 0x3366cc)
	dust := AppendF32(append([]byte(nil), rgb...), 1.5)                                         // INT colour, FLOAT scale
	dct := AppendF32(AppendI32(append([]byte(nil), rgb...), 0x112233), 2)                       // INT from, INT to, FLOAT scale
	trail := AppendVarInt(AppendI32(AppendF64(AppendF64(AppendF64(nil, 4), 5), 6), 0xff00), 40) // Vec3, INT, VAR_INT
	vibBlock := AppendVarInt(AppendPosition(AppendVarInt(nil, 0), 7, 64, -8), 20)               // block source, VAR_INT ticks
	vibEntity := AppendVarInt(AppendF32(AppendVarInt(AppendVarInt(nil, 1), 300), 0.5), 12)      // entity source: VAR_INT id, FLOAT offset
	for _, v := range []int32{770, 776, 777} {
		for _, tc := range []struct {
			name string
			pid  int32
			opts []byte
		}{
			{"dust", ParticleDust770, dust},
			{"dust_color_transition", ParticleDustColorTransition770, dct},
			{"entity_effect", ParticleEntityEffect770, AppendI32(nil, -0x10000000)},
			{"tinted_leaves", ParticleTintedLeaves770, rgb},
			{"sculk_charge", ParticleSculkCharge770, AppendF32(nil, 3.14)},
			{"shriek", ParticleShriek770, AppendVarInt(nil, 150)},
			{"trail", ParticleTrail770, trail},
			{"vibration block", ParticleVibration770, vibBlock},
			{"vibration entity", ParticleVibration770, vibEntity},
		} {
			body := append(AppendVarInt(particlesPrefix(), tc.pid), tc.opts...)
			got, drop := remapLevelParticles(v, body)
			want := append(AppendVarInt(particlesPrefix(), remapParticleID(v, tc.pid)), tc.opts...)
			if drop || !bytes.Equal(got, want) {
				t.Errorf("v%d %s: drop=%v\n got %x\nwant %x", v, tc.name, drop, got, want)
			}
		}
		// A block particle's state is a canonical block-state id.
		const stone = 1
		for _, pid := range []int32{ParticleBlock770, ParticleBlockMarker770, ParticleFallingDust770, ParticleDustPillar770, ParticleBlockCrumble770} {
			body := AppendVarInt(AppendVarInt(particlesPrefix(), pid), stone)
			got, drop := remapLevelParticles(v, body)
			want := AppendVarInt(AppendVarInt(particlesPrefix(), remapParticleID(v, pid)), RemapID(RegBlockState, v, stone))
			if drop || !bytes.Equal(got, want) {
				t.Errorf("v%d block particle %d: %x, want %x", v, pid, got, want)
			}
		}
	}
	// A truncated option set drops the packet rather than send it short.
	if _, drop := remapLevelParticles(777, append(AppendVarInt(particlesPrefix(), ParticleDust770), rgb...)); !drop {
		t.Error("a dust particle without its scale went out")
	}
}

// dragon_breath, effect, instant_effect and flash took options in 26.x
// (PowerParticleOption FLOAT; SpellParticleOption INT colour, FLOAT power;
// ColorParticleOption INT). A 26.x client gets the canonical tail when the
// engine sent one, else the codecs' defaults; a 770 client gets neither.
func TestLevelParticleOptions26x(t *testing.T) {
	one := AppendF32(nil, 1)
	for _, tc := range []struct {
		name      string
		pid       int32
		tail, def []byte
	}{
		{"dragon_breath", ParticleDragonBreath770, AppendF32(nil, 0.5), one},
		{"flash", ParticleFlash770, AppendI32(nil, 0x7f00ff00), AppendI32(nil, -1)},
		{"effect", ParticleEffect770, AppendF32(AppendI32(nil, 0xff0000), 2), append(AppendI32(nil, -1), one...)},
		{"instant_effect", ParticleInstantEffect770, AppendF32(AppendI32(nil, 0x00ff00), 0.25), append(AppendI32(nil, -1), one...)},
	} {
		bare := AppendVarInt(particlesPrefix(), tc.pid)
		withTail := append(append([]byte(nil), bare...), tc.tail...)
		for _, v := range []int32{776, 777} {
			id := AppendVarInt(particlesPrefix(), remapParticleID(v, tc.pid))
			if got, drop := remapLevelParticles(v, bare); drop || !bytes.Equal(got, append(append([]byte(nil), id...), tc.def...)) {
				t.Errorf("v%d %s without options: %x", v, tc.name, got)
			}
			if got, drop := remapLevelParticles(v, withTail); drop || !bytes.Equal(got, append(append([]byte(nil), id...), tc.tail...)) {
				t.Errorf("v%d %s with options: %x", v, tc.name, got)
			}
		}
		if got, drop := remapLevelParticles(770, withTail); drop || !bytes.Equal(got, bare) {
			t.Errorf("770 %s kept the 26.x options: %x", tc.name, got)
		}
	}
}

// An item particle: a canonical Slot (count, item, components), which 26.x
// reads as an ItemStackTemplate (item, count, components); an item the
// client lacks cannot be sent (a template is never empty).
func TestLevelParticleItem(t *testing.T) {
	item := CanonicalItem("diamond")
	slot := append(AppendVarInt(AppendVarInt(nil, 3), item), 0, 0) // count 3, no components
	body := append(AppendVarInt(particlesPrefix(), ParticleItem770), slot...)
	for _, v := range []int32{776, 777} {
		want := AppendVarInt(particlesPrefix(), remapParticleID(v, ParticleItem770))
		want = AppendVarInt(want, RemapID(RegItem, v, item))
		want = append(AppendVarInt(want, 3), 0, 0)
		if got, drop := remapLevelParticles(v, body); drop || !bytes.Equal(got, want) {
			t.Errorf("v%d item particle:\n got %x\nwant %x", v, got, want)
		}
	}
	empty := AppendVarInt(AppendVarInt(particlesPrefix(), ParticleItem770), 0)
	if _, drop := remapLevelParticles(777, empty); !drop {
		t.Error("an empty item particle went out")
	}
}

// Through the whole 26.3 chain: options remapped, then the 777 step moves
// the particle (with its options) to the front. Re-parsed field by field
// from 26.3's STREAM_CODEC: particle, bool, bool, 3 DOUBLE, 3 FLOAT offsets,
// 3 FLOAT max speeds, VAR_INT count, VAR_INT randomization.
func TestLevelParticlesChain777(t *testing.T) {
	opts := AppendF32(AppendI32(nil, 0xabcdef), 2)
	body := append(AppendVarInt(particlesPrefix(), ParticleDust770), opts...)
	id, out, drop := TranslatorFor(777).Clientbound(StatePlay, canonWorldParticles, body)
	if drop {
		t.Fatal("dropped")
	}
	r := bytes.NewReader(out)
	if pid, _ := ReadVarInt(r); pid != 21 { // 26.3's dust
		t.Fatalf("particle id %d, want 21 (packet 0x%x)", pid, id)
	}
	got := make([]byte, 8)
	r.Read(got)
	if !bytes.Equal(got, opts) {
		t.Fatalf("dust options %x, want %x", got, opts)
	}
	rest := make([]byte, 2+24+12+12)
	r.Read(rest)
	if !bytes.Equal(rest[:2+24+12], particlesPrefix()[:2+24+12]) {
		t.Errorf("flags/position/offsets moved: %x", rest)
	}
	if count, _ := ReadVarInt(r); count != 9 {
		t.Errorf("count %d", count)
	}
	if rnd, _ := ReadVarInt(r); rnd != 0 || r.Len() != 0 {
		t.Errorf("randomization %d, %d bytes left", rnd, r.Len())
	}
}

// An area-effect cloud's DATA_PARTICLE (index 10): the PARTICLE serializer,
// 17 canonically and 16 from 773 (COMPOUND_TAG's removal) — 16 on 26.2 and
// 26.3 in EntityDataSerializers' registration order — with the particle in
// the client's form. The dragon's breath gains its 26.x power (1).
func TestCloudParticleMeta(t *testing.T) {
	meta := func(typ int32, particle []byte) []byte {
		b := append(AppendVarInt(nil, 42), 10)
		b = AppendVarInt(b, typ)
		b = append(b, particle...)
		return append(b, 0xff)
	}
	potion := append(AppendVarInt(nil, ParticleEntityEffect770), AppendI32(nil, -0xcc3399)...)
	breath := AppendVarInt(nil, ParticleDragonBreath770)
	for _, v := range []int32{776, 777} {
		want := meta(16, append(AppendVarInt(nil, remapParticleID(v, ParticleEntityEffect770)), AppendI32(nil, -0xcc3399)...))
		if got := remapEntityMeta(v, meta(metaTypeParticle, potion)); !bytes.Equal(got, want) {
			t.Errorf("v%d potion cloud: %x, want %x", v, got, want)
		}
		want = meta(16, append(AppendVarInt(nil, remapParticleID(v, ParticleDragonBreath770)), AppendF32(nil, 1)...))
		if got := remapEntityMeta(v, meta(metaTypeParticle, breath)); !bytes.Equal(got, want) {
			t.Errorf("v%d breath cloud: %x, want %x", v, got, want)
		}
	}
	// Canonical 770 keeps its ids and serializer.
	if got := remapEntityMeta(770, meta(metaTypeParticle, breath)); !bytes.Equal(got, meta(metaTypeParticle, breath)) {
		t.Errorf("770 breath cloud: %x", got)
	}
	// The type-specific walkers carry it as it is.
	if got := ShiftAgeableMobMeta(776, meta(metaTypeParticle, potion)); !bytes.Equal(got, meta(metaTypeParticle, potion)) {
		t.Errorf("the walker changed a cloud particle: %x", got)
	}
}

// A cushion's DATA_COLOR (index 8) arrives as an INT placeholder; a 26.3
// client reads DYE_COLOR, serializer 43 (the last of 26.3's
// registrations), and the chain carries it.
func TestCushionColorMeta(t *testing.T) {
	body := append(AppendVarInt(nil, 77), 8)
	body = AppendVarInt(body, metaTypeVarInt)
	body = AppendVarInt(body, 14) // red
	body = append(body, 0xff)
	fixed := FixCushionMeta(777, body)
	want := append(AppendVarInt(nil, 77), 8, 43, 14, 0xff)
	if !bytes.Equal(fixed, want) {
		t.Fatalf("fixed %x, want %x", fixed, want)
	}
	if got := remapEntityMeta(777, fixed); !bytes.Equal(got, want) {
		t.Errorf("through the chain %x, want %x", got, want)
	}
	if got := FixCushionMeta(776, body); !bytes.Equal(got, body) {
		t.Errorf("26.2 (no cushion) was rewritten: %x", got)
	}
}

// 26.3's breaking level events go to 26.3 clients only; a 26.2 client
// draws its own cracks and has neither event.
func TestDestroyProgressEvents(t *testing.T) {
	for _, ev := range []int32{LevelEventDestroyProgress, LevelEventDestroyProgressAndSound} {
		body := AppendBool(AppendI32(AppendPosition(AppendI32(nil, ev), 1, 70, -3), 5), false) // face EAST
		if _, _, drop := TranslatorFor(776).Clientbound(StatePlay, canonWorldEvent, body); !drop {
			t.Errorf("event %d reached a 26.2 client", ev)
		}
		_, out, drop := TranslatorFor(777).Clientbound(StatePlay, canonWorldEvent, body)
		if drop || !bytes.Equal(out, body) {
			t.Errorf("event %d on 26.3: drop=%v %x", ev, drop, out)
		}
	}
	// The global flag rides through untouched.
	global := AppendBool(AppendI32(AppendPosition(AppendI32(nil, 1023), 1, 70, -3), 0), true)
	if _, out, drop := TranslatorFor(776).Clientbound(StatePlay, canonWorldEvent, global); drop || !bytes.Equal(out, global) {
		t.Errorf("a global event changed: %x", out)
	}
}
