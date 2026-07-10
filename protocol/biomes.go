package protocol

// biomeIndex maps a biome identifier to its network ID (its index in the biome
// registry we send during Configuration).
var biomeIndex = func() map[string]int32 {
	m := make(map[string]int32)
	for _, reg := range SyncedRegistries {
		if reg.ID == "minecraft:worldgen/biome" {
			for i, e := range reg.Entries {
				m[e] = int32(i)
			}
		}
	}
	return m
}()

// BiomeID returns the network ID of a biome by identifier, falling back to
// plains if the name is unknown.
func BiomeID(name string) int32 {
	if id, ok := biomeIndex[name]; ok {
		return id
	}
	return BiomePlainsID
}
