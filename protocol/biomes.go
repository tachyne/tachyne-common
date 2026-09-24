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

// biomesAdded are biomes a client version has beyond the shared list and the
// 26.x extras, appended to that version's biome registry in this order. An
// older client is shown the stand-in: a 26.2 client does not have dappled
// forest, and a biome its built-in pack lacks cannot be declared to it.
var biomesAdded = []struct {
	since         int32
	name, standIn string
}{
	{777, "minecraft:dappled_forest", "minecraft:forest"},
}

// biomeEntriesFor is the biome registry a client version is sent: the shared
// list, the 26.x extras, then the biomes added in versions up to its own.
func biomeEntriesFor(v int32, shared []string) []string {
	if v < 775 {
		return shared
	}
	out := append(append([]string(nil), shared...), extra26xEntries["minecraft:worldgen/biome"]...)
	for _, b := range biomesAdded {
		if v >= b.since {
			out = append(out, b.name)
		}
	}
	return out
}

// BiomeIDFor is the network id of a biome on a client version: its index in
// the registry that version is sent (biomeEntriesFor), or its stand-in's on a
// version without it, or plains for an unknown name.
func BiomeIDFor(v int32, name string) int32 {
	if id, ok := biomeIndex[name]; ok {
		return id
	}
	for _, b := range biomesAdded {
		if b.name != name {
			continue
		}
		if v < b.since {
			return BiomeIDFor(v, b.standIn)
		}
		for i, e := range biomeEntriesFor(v, sharedBiomes()) {
			if e == name {
				return int32(i)
			}
		}
	}
	// A biome 26.x appended to the shared list (extra26xEntries): its place
	// after the shared ones. It fell through to plains, so every Java client
	// was told a sulfur cave was plains.
	for _, e := range extra26xEntries["minecraft:worldgen/biome"] {
		if e != name {
			continue
		}
		if v < 775 {
			return BiomeIDFor(v, "minecraft:dripstone_caves") // no such biome before 26.x
		}
		for i, n := range biomeEntriesFor(v, sharedBiomes()) {
			if n == name {
				return int32(i)
			}
		}
	}
	return BiomePlainsID
}

// sharedBiomes is the biome registry every version is sent first.
func sharedBiomes() []string {
	for _, reg := range SyncedRegistries {
		if reg.ID == "minecraft:worldgen/biome" {
			return reg.Entries
		}
	}
	return nil
}
