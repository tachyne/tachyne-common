package protocol

// Explode26x is ClientboundExplodePacket's body as 26.x lays it out (26.1
// added the radius, block count and block-debris particles; 26.3 the
// play-sound flag). Canonical 770's explode is a different, smaller packet
// and no step rewrites it, so the gateway builds this at the client's
// version and lets the chain renumber only the id.
//
// The explosion particle is explosion_emitter for a large blast and
// explosion for a small one (ServerExplosion.isSmall), the sound
// entity.generic.explode, and the debris Level's default list — poof (0.5,
// 1) and smoke (1, 1), weight 1 each.
func Explode26x(version int32, x, y, z float64, radius float32, blocks int32, knock *[3]float64, large, playSound bool) []byte {
	b := AppendF64(nil, x)
	b = AppendF64(b, y)
	b = AppendF64(b, z)
	b = AppendF32(b, radius)
	b = AppendI32(b, blocks)
	b = AppendBool(b, knock != nil)
	if knock != nil {
		b = AppendF64(b, knock[0])
		b = AppendF64(b, knock[1])
		b = AppendF64(b, knock[2])
	}
	particle := int32(canonParticleExplosion)
	if large {
		particle = canonParticleExplosionEmitter
	}
	b = AppendVarInt(b, remapParticleID(version, particle))
	// Holder<SoundEvent>, direct: 0, then the event's id and no fixed range.
	b = AppendVarInt(b, 0)
	b = AppendString(b, "minecraft:entity.generic.explode")
	b = AppendBool(b, false)
	b = AppendVarInt(b, 2) // WeightedList<ExplosionParticleInfo>
	for _, p := range [2]struct {
		id             int32
		scaling, speed float32
	}{{canonParticlePoof, 0.5, 1}, {canonParticleSmoke, 1, 1}} {
		b = AppendVarInt(b, remapParticleID(version, p.id))
		b = AppendF32(b, p.scaling)
		b = AppendF32(b, p.speed)
		b = AppendVarInt(b, 1) // weight
	}
	if version >= 777 {
		b = AppendBool(b, playSound)
	}
	return b
}

// Canonical (770) particle ids the explosion uses.
const (
	canonParticleExplosionEmitter = 21
	canonParticleExplosion        = 22
	canonParticlePoof             = 56
	canonParticleSmoke            = 59
)

// CanonExplode is explode's canonical 770 packet id; the chain renumbers it.
const CanonExplode = 0x20
