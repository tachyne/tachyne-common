package protocol

// spawnernbt.go — the mob spawner block entity's update tag
// (SpawnerBlockEntity.getUpdateTag: BaseSpawner.save minus SpawnPotentials,
// which vanilla strips before sending).
//
// The one field the client truly needs is SpawnData.entity.id: that is what
// the renderer builds the little mob turning inside the cage from. The
// configuration shorts ride along because they are what vanilla sends and
// they cost nothing.

// SpawnerCfg is BaseSpawner's configuration. A zero field takes vanilla's
// own default, so a caller that only knows which mob need fill in nothing.
type SpawnerCfg struct {
	Delay             int16
	MinDelay          int16
	MaxDelay          int16
	SpawnCount        int16
	MaxNearbyEntities int16
	PlayerRange       int16
	SpawnRange        int16
}

// BaseSpawner's field defaults.
const (
	spawnerDefMinDelay    = 200
	spawnerDefMaxDelay    = 800
	spawnerDefCount       = 4
	spawnerDefMaxNearby   = 6
	spawnerDefPlayerRange = 16
	spawnerDefSpawnRange  = 4
)

func or16(v, def int16) int16 {
	if v == 0 {
		return def
	}
	return v
}

// AppendSpawnerNBT appends a spawner's update tag as network NBT (nameless
// root compound). An empty entity name leaves SpawnData out entirely, which
// is a spawner nothing has set yet — vanilla draws that one empty.
func AppendSpawnerNBT(b []byte, entity string, c SpawnerCfg) []byte {
	b = append(b, NBTRoot()...)
	b = NBTShort(b, "Delay", c.Delay)
	b = NBTShort(b, "MinSpawnDelay", or16(c.MinDelay, spawnerDefMinDelay))
	b = NBTShort(b, "MaxSpawnDelay", or16(c.MaxDelay, spawnerDefMaxDelay))
	b = NBTShort(b, "SpawnCount", or16(c.SpawnCount, spawnerDefCount))
	b = NBTShort(b, "MaxNearbyEntities", or16(c.MaxNearbyEntities, spawnerDefMaxNearby))
	b = NBTShort(b, "RequiredPlayerRange", or16(c.PlayerRange, spawnerDefPlayerRange))
	b = NBTShort(b, "SpawnRange", or16(c.SpawnRange, spawnerDefSpawnRange))
	if entity != "" {
		// SpawnData: { entity: { id: "minecraft:blaze" } }
		b = nbtName(append(b, nbtCompound), "SpawnData")
		b = nbtName(append(b, nbtCompound), "entity")
		b = NBTString(b, "id", entity)
		b = NBTEnd(b) // entity
		b = NBTEnd(b) // SpawnData
	}
	return NBTEnd(b)
}
