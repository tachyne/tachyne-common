package protocol

// The client requires inline NBT data for three registries even when a known
// pack matches: dimension_type, worldgen/biome, and damage_type. Everything
// else resolves from the known pack (has_data=false). These builders provide
// the minimal vanilla-accurate content for the entries we actually reference.

// visualAttrs opens the 26.x dimension_type "attributes" compound and writes
// the visual entries. 26.1 restructured dimension_type: `effects` is gone and
// every look-and-light property moved into this map, so a 1.21.5-shaped entry
// leaves a 26.x client with EnvironmentAttributeMap.EMPTY and the defaults —
// which is why our nether was lit and fogged like the overworld. The keys are
// the attribute registry's ids; an RGB_COLOR is ExtraCodecs.STRING_RGB_VEC3_
// COLOR (a "#rrggbb" STRING, not an int) and a FLOAT is a plain float. Only
// syncable attributes reach a client, and every visual one is syncable.
func visualAttrs(b []byte, kv ...any) []byte {
	b = NBTCompound(b, "attributes")
	for i := 0; i+1 < len(kv); i += 2 {
		key := "minecraft:visual/" + kv[i].(string)
		switch v := kv[i+1].(type) {
		case string:
			b = NBTString(b, key, v)
		case float64:
			b = NBTFloat(b, key, float32(v))
		}
	}
	return NBTEnd(b)
}

func overworldNBT(v int32, height int32) []byte {
	if height <= 0 {
		height = 384 // vanilla-height world
	}
	b := NBTRoot()
	b = NBTBool(b, "has_skylight", true)
	b = NBTBool(b, "has_ceiling", false)
	b = NBTBool(b, "ultrawarm", false)
	b = NBTBool(b, "natural", true)
	b = NBTDouble(b, "coordinate_scale", 1.0)
	b = NBTBool(b, "bed_works", true)
	b = NBTBool(b, "respawn_anchor_works", false)
	b = NBTInt(b, "min_y", -64)
	b = NBTInt(b, "height", height)
	b = NBTInt(b, "logical_height", height)
	b = NBTString(b, "infiniburn", "#minecraft:infiniburn_overworld")
	b = NBTString(b, "effects", "minecraft:overworld")
	b = NBTFloat(b, "ambient_light", 0)
	b = NBTBool(b, "piglin_safe", false)
	b = NBTBool(b, "has_raids", true)
	b = NBTInt(b, "monster_spawn_block_light_limit", 0)
	b = NBTInt(b, "monster_spawn_light_level", 0)
	if v >= 775 {
		b = NBTBool(b, "has_ender_dragon_fight", false)
		// 26.x day/night: the sky follows a WORLD CLOCK referenced from the
		// dimension. default_clock is OPTIONAL and defaults to none — without
		// it the client's sun is frozen at the default position and every
		// set_time clock update is ignored (found live: HUD said 22:14, sun
		// stood at noon). timelines drive the 26.x celestial/ambience events;
		// both values mirror vanilla's dimension_type/overworld.json, and the
		// timeline tag ships in our Update Tags data.
		b = NBTString(b, "default_clock", "minecraft:overworld")
		b = NBTString(b, "timelines", "#minecraft:in_overworld")
	}
	return NBTEnd(b)
}

// netherNBT declares OUR nether dimension: vanilla look (the_nether effects,
// no skylight, ultrawarm) but the overworld's -64..384 height, so the chunk
// pipeline (24 sections + light arrays) is identical in both dimensions.
func netherNBT(v int32) []byte {
	b := NBTRoot()
	b = NBTBool(b, "has_skylight", false)
	b = NBTBool(b, "has_ceiling", false)
	b = NBTBool(b, "ultrawarm", true)
	b = NBTBool(b, "natural", false)
	b = NBTDouble(b, "coordinate_scale", 8.0)
	b = NBTBool(b, "bed_works", false)
	b = NBTBool(b, "respawn_anchor_works", true)
	b = NBTInt(b, "min_y", -64)
	b = NBTInt(b, "height", 384)
	b = NBTInt(b, "logical_height", 384)
	b = NBTString(b, "infiniburn", "#minecraft:infiniburn_nether")
	b = NBTString(b, "effects", "minecraft:the_nether")
	b = NBTFloat(b, "ambient_light", 0.1)
	b = NBTBool(b, "piglin_safe", true)
	b = NBTBool(b, "has_raids", false)
	b = NBTInt(b, "monster_spawn_block_light_limit", 15)
	b = NBTInt(b, "monster_spawn_light_level", 7)
	if v >= 775 {
		// 26.1+ dimension_type requires this field (ViaVersion adds exactly
		// this when translating 1.21.5 data forward).
		b = NBTBool(b, "has_ender_dragon_fight", false)
		// Vanilla nether: frozen sky (no clock), nether ambience timelines.
		b = NBTBool(b, "has_fixed_time", true)
		b = NBTString(b, "timelines", "#minecraft:in_nether")
		// Without these a 26.x client renders the nether with the OVERWORLD
		// skybox, the overworld's directional face shading and no ambient
		// tint — reported in game as "the lighting in the nether is not
		// correct". Values are vanilla the_nether.json (26.1 through 26.3,
		// identical).
		b = NBTString(b, "skybox", "none")
		b = NBTString(b, "cardinal_light", "nether")
		b = visualAttrs(b,
			"ambient_light_color", "#302821",
			"sky_light_color", "#7a7aff",
			"sky_light_factor", 0.0,
			"fog_start_distance", 10.0,
			"fog_end_distance", 96.0)
	}
	return NBTEnd(b)
}

func plainsNBT() []byte {
	b := NBTRoot()
	b = NBTBool(b, "has_precipitation", true)
	b = NBTFloat(b, "temperature", 0.8)
	b = NBTFloat(b, "downfall", 0.4)
	b = NBTCompound(b, "effects")
	b = NBTInt(b, "sky_color", 7907327)
	b = NBTInt(b, "water_color", 4159204)
	b = NBTInt(b, "water_fog_color", 329011)
	b = NBTInt(b, "fog_color", 12638463)
	b = NBTCompound(b, "mood_sound")
	b = NBTString(b, "sound", "minecraft:ambient.cave")
	b = NBTInt(b, "tick_delay", 6000)
	b = NBTInt(b, "block_search_extent", 8)
	b = NBTDouble(b, "offset", 2.0)
	b = NBTEnd(b)    // mood_sound
	b = NBTEnd(b)    // effects
	return NBTEnd(b) // root
}

// endNBT: our End dimension — vanilla End look (the_end effects, fixed dusk
// light) on the shared -64..384 canvas, same trick as the nether.
func endNBT(v int32) []byte {
	b := NBTRoot()
	b = NBTBool(b, "has_skylight", false)
	b = NBTBool(b, "has_ceiling", false)
	b = NBTBool(b, "ultrawarm", false)
	b = NBTBool(b, "natural", false)
	b = NBTDouble(b, "coordinate_scale", 1.0)
	b = NBTBool(b, "bed_works", false)
	b = NBTBool(b, "respawn_anchor_works", false)
	b = NBTInt(b, "min_y", -64)
	b = NBTInt(b, "height", 384)
	b = NBTInt(b, "logical_height", 384)
	b = NBTString(b, "infiniburn", "#minecraft:infiniburn_end")
	b = NBTString(b, "effects", "minecraft:the_end")
	if v >= 773 {
		b = NBTFloat(b, "ambient_light", 0.25) // 1.21.9+: the End gained real skylight
	} else {
		b = NBTFloat(b, "ambient_light", 0)
	}
	b = NBTBool(b, "piglin_safe", false)
	b = NBTBool(b, "has_raids", true)
	b = NBTInt(b, "monster_spawn_block_light_limit", 0)
	b = NBTInt(b, "monster_spawn_light_level", 7)
	if v >= 775 {
		// FALSE deliberately: true tells a 26.x client this dimension runs the
		// vanilla dragon-fight machinery and the client defers dragon handling
		// to fight state we don't send — the boss simply never rendered. With
		// false the dragon is an ordinary entity, exactly as in the overworld
		// (where the flag is false and a summoned dragon renders fine).
		b = NBTBool(b, "has_ender_dragon_fight", false)
		// Vanilla End: frozen sky, but the End clock still exists (drives
		// non-celestial events) and the End ambience timelines.
		b = NBTBool(b, "has_fixed_time", true)
		b = NBTString(b, "default_clock", "minecraft:the_end")
		b = NBTString(b, "timelines", "#minecraft:in_end")
		b = NBTString(b, "skybox", "end")
		b = visualAttrs(b,
			"ambient_light_color", "#3f473f",
			"sky_light_color", "#ac60cd",
			"sky_light_factor", 0.0,
			"sky_color", "#000000",
			"fog_color", "#181318")
	}
	return NBTEnd(b)
}

// theEndBiomeNBT: the End's black-sky ambience.
func theEndBiomeNBT() []byte {
	b := NBTRoot()
	b = NBTBool(b, "has_precipitation", false)
	b = NBTFloat(b, "temperature", 0.5)
	b = NBTFloat(b, "downfall", 0.5)
	b = NBTCompound(b, "effects")
	b = NBTInt(b, "sky_color", 0)
	b = NBTInt(b, "water_color", 4159204)
	b = NBTInt(b, "water_fog_color", 329011)
	b = NBTInt(b, "fog_color", 10518688)
	b = NBTCompound(b, "mood_sound")
	b = NBTString(b, "sound", "minecraft:ambient.cave")
	b = NBTInt(b, "tick_delay", 6000)
	b = NBTInt(b, "block_search_extent", 8)
	b = NBTDouble(b, "offset", 2.0)
	b = NBTEnd(b)    // mood_sound
	b = NBTEnd(b)    // effects
	return NBTEnd(b) // root
}

// netherWastesNBT: red fog, no rain — the nether's default biome ambience.
func netherWastesNBT() []byte {
	b := NBTRoot()
	b = NBTBool(b, "has_precipitation", false)
	b = NBTFloat(b, "temperature", 2.0)
	b = NBTFloat(b, "downfall", 0.0)
	b = NBTCompound(b, "effects")
	b = NBTInt(b, "sky_color", 7254527)
	b = NBTInt(b, "water_color", 4159204)
	b = NBTInt(b, "water_fog_color", 329011)
	b = NBTInt(b, "fog_color", 3344392)
	b = NBTCompound(b, "mood_sound")
	b = NBTString(b, "sound", "minecraft:ambient.nether_wastes.mood")
	b = NBTInt(b, "tick_delay", 6000)
	b = NBTInt(b, "block_search_extent", 8)
	b = NBTDouble(b, "offset", 2.0)
	b = NBTEnd(b)    // mood_sound
	b = NBTEnd(b)    // effects
	return NBTEnd(b) // root
}

func damageTypeNBT(d damageType) []byte {
	b := NBTRoot()
	b = NBTString(b, "message_id", d.MessageID)
	b = NBTString(b, "scaling", d.Scaling)
	b = NBTFloat(b, "exhaustion", d.Exhaustion)
	if d.Effects != "" {
		b = NBTString(b, "effects", d.Effects)
	}
	if d.DeathMessageType != "" {
		b = NBTString(b, "death_message_type", d.DeathMessageType)
	}
	return NBTEnd(b)
}

var damageTypeByName = func() map[string]damageType {
	m := make(map[string]damageType, len(damageTypes))
	for _, d := range damageTypes {
		m["minecraft:"+d.Name] = d
	}
	return m
}()

// RegistryEntryData returns the inline NBT for a registry entry and whether it
// must be sent (has_data=true). The three mandatory registries return data for
// the entries we reference; everything else resolves via the known pack.
func RegistryEntryData(registryID, entryName string) ([]byte, bool) {
	return RegistryEntryDataFor(registryID, entryName, Target)
}

// RegistryEntryDataFor is RegistryEntryData for a specific client protocol
// version: 26.x dimension_type entries carry the extra required fields
// (has_ender_dragon_fight) so the inline NBT validates there too. The
// dimension bounds are LOAD-BEARING for translated clients — without inline
// data the client falls back to its built-in nether/End (0..256) while our
// chunks assume -64..384, rendering every dimension 64 blocks up-shifted.
func RegistryEntryDataFor(registryID, entryName string, v int32) ([]byte, bool) {
	return registryEntryDataFor(registryID, entryName, v, 0)
}

// registryEntryDataFor additionally takes the overworld height in blocks
// (0 = vanilla 384) — a TALL world (earth mode at true vertical scale)
// declares its real ceiling so the client sizes chunk columns, heightmap
// bit-packing, and the build limit to match.
func registryEntryDataFor(registryID, entryName string, v, overworldHeight int32) ([]byte, bool) {
	switch registryID {
	case "minecraft:dimension_type":
		if entryName == "minecraft:overworld" {
			return overworldNBT(v, overworldHeight), true
		}
		if entryName == "minecraft:the_nether" {
			return netherNBT(v), true
		}
		if entryName == "minecraft:the_end" {
			return endNBT(v), true
		}
	case "minecraft:worldgen/biome":
		if entryName == "minecraft:plains" {
			return plainsNBT(), true
		}
		if entryName == "minecraft:nether_wastes" {
			return netherWastesNBT(), true
		}
		if entryName == "minecraft:the_end" {
			return theEndBiomeNBT(), true
		}
	case "minecraft:damage_type":
		if d, ok := damageTypeByName[entryName]; ok {
			return damageTypeNBT(d), true
		}
	}
	return nil, false
}
