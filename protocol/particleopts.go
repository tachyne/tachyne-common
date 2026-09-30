package protocol

import (
	"bytes"
	"io"
)

// Particle options across versions. A particle on the wire is its type id
// followed by that type's options (ParticleTypes.STREAM_CODEC): nothing for
// a simple type, else the type's own codec. Canonically the id is 770's and
// the options are 770's, with one addition: the four types that took options
// in 26.x (dragon_breath's PowerParticleOption, effect's and instant_effect's
// SpellParticleOption, flash's ColorParticleOption) may carry them after the
// 770 form, which a 770 client does not read and a 26.x one must.
//
// Canonical (770) ids of the types that carry options, from the 1.21.5
// particle_type registry.
const (
	ParticleBlock770               = 1   // BlockParticleOption: state id
	ParticleBlockMarker770         = 2   // BlockParticleOption
	ParticleDragonBreath770        = 7   // simple; PowerParticleOption (float power) on 26.x
	ParticleDust770                = 13  // DustParticleOptions: int RGB, float scale
	ParticleDustColorTransition770 = 14  // DustColorTransitionOptions: int from, int to, float scale
	ParticleEffect770              = 15  // simple; SpellParticleOption (int color, float power) on 26.x
	ParticleFallingDust770         = 28  // BlockParticleOption
	ParticleTintedLeaves770        = 35  // ColorParticleOption: int ARGB
	ParticleSculkCharge770         = 37  // SculkChargeParticleOptions: float roll
	ParticleFlash770               = 41  // simple; ColorParticleOption (int ARGB) on 26.x
	ParticleInstantEffect770       = 45  // simple; SpellParticleOption on 26.x
	ParticleItem770                = 46  // ItemParticleOption: ItemStack (an ItemStackTemplate on 26.x)
	ParticleVibration770           = 47  // VibrationParticleOption: PositionSource, VarInt ticks
	ParticleTrail770               = 48  // TrailParticleOption: Vec3 target, int RGB, VarInt duration
	ParticleShriek770              = 102 // ShriekParticleOption: VarInt delay
	ParticleDustPillar770          = 108 // BlockParticleOption
	ParticleBlockCrumble770        = 112 // BlockParticleOption

	// ParticleTypes the 1.21.5 registry holds; an id past it is unknown.
	particleCount770 = 114
)

// canonicalParticle, as copyParticle's version, copies canonical to
// canonical.
const canonicalParticle int32 = -1

// particleOptions26x reports whether a client reads 26.x's options for
// dragon_breath, effect, instant_effect and flash (773–775 are unrouted).
func particleOptions26x(version int32) bool { return version >= 776 }

// particleExtLen is the length of the 26.x options a canonical particle of
// this type may carry after its 770 form: 0 for every other type.
func particleExtLen(id int32) int {
	switch id {
	case ParticleDragonBreath770, ParticleFlash770:
		return 4
	case ParticleEffect770, ParticleInstantEffect770:
		return 8
	}
	return 0
}

// particleExtDefault is what those options are when the canonical form leaves
// them out: the codecs' own defaults (power 1, colour -1 = opaque white).
func particleExtDefault(id int32) []byte {
	one := AppendF32(nil, 1)
	switch id {
	case ParticleDragonBreath770:
		return one
	case ParticleFlash770:
		return AppendI32(nil, -1)
	case ParticleEffect770, ParticleInstantEffect770:
		return append(AppendI32(nil, -1), one...)
	}
	return nil
}

// copyParticle reads one canonical particle (id and options) off r and writes
// it in the client version's form: the id through the version's table, block
// states and items remapped, an item as a template on 26.x, the 26.x options
// added (from the canonical tail when ext and it is there, else the codec
// defaults) or dropped below 26.x. ext says the particle ends its packet,
// so a canonical 26.x tail can be told from what follows. False means r did
// not hold a particle this walker understands, or its item does not exist on
// the client (an item particle cannot be empty); the caller bails.
//
// version canonicalParticle copies the particle as it is (a walker that
// only moves entries about); an item particle then bails.
func copyParticle(r *bytes.Reader, out *[]byte, version int32, ext bool) bool {
	id, err := ReadVarInt(r)
	if err != nil || id < 0 || id >= particleCount770 {
		return false
	}
	canon := version == canonicalParticle
	if canon {
		*out = AppendVarInt(*out, id)
	} else {
		*out = AppendVarInt(*out, remapParticleID(version, id))
	}
	fixed := func(n int) bool {
		b := make([]byte, n)
		if _, err := io.ReadFull(r, b); err != nil {
			return false
		}
		*out = append(*out, b...)
		return true
	}
	varint := func(mapV func(int32) int32) bool {
		v, err := ReadVarInt(r)
		if err != nil {
			return false
		}
		*out = AppendVarInt(*out, mapV(v))
		return true
	}
	same := func(v int32) int32 { return v }
	switch id {
	case ParticleBlock770, ParticleBlockMarker770, ParticleFallingDust770, ParticleDustPillar770, ParticleBlockCrumble770:
		if canon {
			return varint(same)
		}
		return varint(func(s int32) int32 { return RemapID(RegBlockState, version, s) })
	case ParticleDust770:
		return fixed(8)
	case ParticleDustColorTransition770:
		return fixed(12)
	case ParticleEntityEffect770, ParticleTintedLeaves770:
		return fixed(4)
	case ParticleSculkCharge770:
		return fixed(4)
	case ParticleShriek770:
		return varint(same)
	case ParticleTrail770:
		return fixed(24+4) && varint(same)
	case ParticleVibration770:
		kind, err := ReadVarInt(r)
		if err != nil {
			return false
		}
		*out = AppendVarInt(*out, kind)
		switch kind {
		case 0: // minecraft:block — BlockPos
			if !fixed(8) {
				return false
			}
		case 1: // minecraft:entity — VarInt id, float y offset
			if !varint(same) || !fixed(4) {
				return false
			}
		default:
			return false
		}
		return varint(same) // arrival in ticks
	case ParticleItem770:
		return !canon && copyParticleItem(r, out, version)
	case ParticleDragonBreath770, ParticleEffect770, ParticleInstantEffect770, ParticleFlash770:
		n := particleExtLen(id)
		opts := particleExtDefault(id)
		tail := ext && r.Len() == n
		if tail {
			opts = make([]byte, n)
			if _, err := io.ReadFull(r, opts); err != nil {
				return false
			}
		}
		if (canon && tail) || (!canon && particleOptions26x(version)) {
			*out = append(*out, opts...)
		}
		return true
	}
	return true // a simple type
}

// copyParticleItem copies ItemParticleOption's stack: a canonical Slot, which
// a 26.x client reads as an ItemStackTemplate (item, count, components) that
// cannot be empty.
func copyParticleItem(r *bytes.Reader, out *[]byte, version int32) bool {
	remap := func(i int32) int32 { return RemapID(RegItem, version, i) }
	if !templateStacks(version) {
		start := len(*out)
		if !copyFullSlot(r, out, remap, version, false) {
			return false
		}
		return len(*out) > start && (*out)[start] != 0 // an empty stack is no particle
	}
	count, err := ReadVarInt(r)
	if err != nil || count <= 0 {
		return false
	}
	item, err := ReadVarInt(r)
	if err != nil {
		return false
	}
	var patch []byte
	if !copyComponentPatch(r, &patch, remap, version, false, 0) {
		return false
	}
	if item = remap(item); item == 0 {
		return false
	}
	*out = AppendVarInt(*out, item)
	*out = AppendVarInt(*out, count)
	*out = append(*out, patch...)
	return true
}

// remapLevelParticles rewrites level_particles' trailing particle for the
// client (canonical layout: two flags, position, offsets, speed, count, then
// the particle). Drop is true for a particle that cannot be written for this
// client — an item it lacks, or options this walker does not know — which
// would otherwise reach it unreadable.
func remapLevelParticles(version int32, body []byte) (out []byte, drop bool) {
	const prefix = 2 + 24 + 16 + 4
	if len(body) <= prefix {
		return body, false
	}
	r := bytes.NewReader(body[prefix:])
	peek, err := ReadVarInt(bytes.NewReader(body[prefix:]))
	if err != nil {
		return body, true
	}
	if peek >= particleCount770 {
		// Not a canonical id: carried as it is when nothing follows it, as
		// ever; with options behind it there is no way to read them.
		if r.Len() == varIntLen(peek) {
			return body, false
		}
		return body, true
	}
	out = append([]byte(nil), body[:prefix]...)
	if !copyParticle(r, &out, version, true) || r.Len() != 0 {
		return body, true
	}
	return out, false
}

// varIntLen is the encoded length of v.
func varIntLen(v int32) int { return len(AppendVarInt(nil, v)) }
