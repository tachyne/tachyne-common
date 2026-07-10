// Package-level config-phase composition: the registry-data and tag packets
// a gateway (or the legacy server path) sends a client during Configuration.
// Moved from the engine (pre-rename) — the Minecraft protocol lives ONLY in gateways
// (tachyne doctrine); worlds speak the domain attach protocol.
package protocol

import "sync"

// ConfigRegistryPackets returns the Registry Data packet payloads for a
// client at protocol version v, in send order: every synced registry (26.x
// clients get the 26.2-added entries appended and dimension_type inlined),
// then for 775+ the extra 26.x registries.
func ConfigRegistryPackets(v int32) [][]byte {
	return ConfigRegistryPacketsFor(v, 0)
}

// ConfigRegistryPacketsFor is ConfigRegistryPackets with the session's
// overworld height in blocks (0 = vanilla 384). Gateways pass the world's
// real height (attach Welcome) so a TALL earth world declares its true
// ceiling; dimension_type is inlined for EVERY client version, so the
// declared height always wins over the client's built-in registry.
func ConfigRegistryPacketsFor(v int32, overworldHeight int32) [][]byte {
	inlineOK := v < 775
	var out [][]byte
	for _, reg := range SyncedRegistries {
		entries := reg.Entries
		if !inlineOK {
			if ex := extra26xEntries[reg.ID]; len(ex) > 0 {
				entries = append(append([]string(nil), reg.Entries...), ex...)
			}
		}
		data := AppendString(nil, reg.ID)
		data = AppendVarInt(data, int32(len(entries)))
		for _, entry := range entries {
			data = AppendString(data, entry)
			inline := inlineOK || reg.ID == "minecraft:dimension_type"
			if nbt, hasData := registryEntryDataFor(reg.ID, entry, v, overworldHeight); hasData && inline {
				data = AppendBool(data, true)
				data = append(data, nbt...)
			} else {
				data = AppendBool(data, false) // resolve via known pack
			}
		}
		out = append(out, data)
	}
	if v >= 775 {
		for _, reg := range extra26xRegistries {
			data := AppendString(nil, reg.id)
			data = AppendVarInt(data, int32(len(reg.entries)))
			for _, e := range reg.entries {
				data = AppendString(data, e)
				data = AppendBool(data, false)
			}
			out = append(out, data)
		}
	}
	return out
}

// UpdateTagsPacket returns the Update Tags payload for protocol version v.
func UpdateTagsPacket(v int32) []byte {
	if v >= 775 {
		return tags26x(v)
	}
	return tagsLegacy()
}

// BrandPayload is the config custom_payload on channel minecraft:brand.
func BrandPayload() []byte {
	b := AppendString(nil, "minecraft:brand")
	return AppendString(b, "tachyne")
}

// FeatureFlags is config update_enabled_features — the vanilla feature set.
func FeatureFlags() []byte {
	b := AppendVarInt(nil, 1)
	return AppendString(b, "minecraft:vanilla")
}

// extra26xEntries lists entries 26.x appended to existing 1.21.5 registries (from
// the 26.2 data). Appended (has_data=false) for 26.x clients so their registry has
// every element the client's item components reference. enchantment is omitted (we
// don't send that registry); the others' new entries resolve via the known pack.
var extra26xEntries = map[string][]string{
	"minecraft:painting_variant": {"minecraft:dennis"},
	"minecraft:worldgen/biome":   {"minecraft:sulfur_caves"},
	"minecraft:damage_type":      {"minecraft:spear", "minecraft:sulfur_cube_hot"},
	"minecraft:jukebox_song":     {"minecraft:bounce", "minecraft:lava_chicken", "minecraft:tears"},
	// 26.x added the spear enchant. APPENDED, so the 42 1.21.5 enchantment
	// network ids (what enchanted-item components carry) are identical on
	// every version.
	"minecraft:enchantment": {"minecraft:lunge"},
}

// extra26xRegistries are the registries 26.x added (timeline, world_clock, mob
// variant registries) that our 1.21.5 SyncedRegistries lacks. Declared
// has_data=false; the ENTRY ORDER here defines their network ids, so the tag
// resolver (tags26x) indexes against this same list.
var extra26xRegistries = []struct {
	id      string
	entries []string
}{
	{"minecraft:world_clock", []string{"minecraft:overworld", "minecraft:the_end"}},
	{"minecraft:timeline", []string{"minecraft:day", "minecraft:early_game", "minecraft:moon", "minecraft:villager_schedule"}},
	// New mob sound/appearance variant registries (26.x). The client validates
	// these as "must be non-empty", so we declare their entries (has_data=false).
	{"minecraft:cat_sound_variant", []string{"minecraft:classic", "minecraft:royal"}},
	{"minecraft:chicken_sound_variant", []string{"minecraft:classic", "minecraft:picky"}},
	{"minecraft:cow_sound_variant", []string{"minecraft:classic", "minecraft:moody"}},
	{"minecraft:pig_sound_variant", []string{"minecraft:big", "minecraft:classic", "minecraft:mini"}},
	{"minecraft:zombie_nautilus_variant", []string{"minecraft:temperate", "minecraft:warm"}},
}

// tags26xSkip lists registries whose tags must NOT be sent: they are
// datapack-only and absent at the client's config-time tag resolution, so
// sending them throws "Missing registry".
var tags26xSkip = map[string]bool{
	"minecraft:worldgen/configured_feature":          true,
	"minecraft:worldgen/flat_level_generator_preset": true,
	"minecraft:worldgen/structure":                   true,
	"minecraft:worldgen/world_preset":                true,
	"minecraft:villager_trade":                       true,
	"minecraft:dialog":                               true,
}

// fluid26xID: the fluid registry is static and tiny; vanilla registration order.
var fluid26xID = map[string]int32{
	"minecraft:empty": 0, "minecraft:flowing_water": 1, "minecraft:water": 2,
	"minecraft:flowing_lava": 3, "minecraft:lava": 4,
}

// dynamic26xIndex maps entry names to network ids for the DYNAMIC registries —
// their ids are simply the order this server declares them in registry_data:
// SyncedRegistries entries (+ the 26.x-added entries appended, exactly as
// sendRegistries does) plus the extra 26.x registries. enchantment is never
// declared, so it is absent here (its tags stay empty).
func dynamic26xIndex() map[string]map[string]int32 {
	idx := map[string]map[string]int32{}
	for _, reg := range SyncedRegistries {
		m := map[string]int32{}
		entries := reg.Entries
		if ex := extra26xEntries[reg.ID]; len(ex) > 0 {
			entries = append(append([]string(nil), reg.Entries...), ex...)
		}
		for i, e := range entries {
			m[e] = int32(i)
		}
		idx[reg.ID] = m
	}
	for _, reg := range extra26xRegistries {
		m := map[string]int32{}
		for i, e := range reg.entries {
			m[e] = int32(i)
		}
		idx[reg.id] = m
	}
	return idx
}

// legacyFluidTag gives the water/lava fluid tags their REAL contents on every
// version (vanilla registration order: flowing_water 1, water 2, flowing_lava
// 3, lava 4 — a static registry, stable across 770-26.2). The client's swim
// physics run off #minecraft:water — empty means players sink like stones.
func legacyFluidTag(registry, tag string) []int32 {
	if registry != "minecraft:fluid" {
		return nil
	}
	switch tag {
	case "minecraft:water":
		return []int32{2, 1}
	case "minecraft:lava":
		return []int32{4, 3}
	}
	return nil
}

var (
	tags26xMu    sync.Mutex
	tags26xByVer = map[int32][]byte{}

	tagsLegacyOnce sync.Once
	tagsLegacyBody []byte
)

// tagsLegacy builds the Update Tags packet for pre-26.x clients (770-774):
// every vanilla 1.21.5 tag name — plus any newer names 26.2 added within those
// same registries, for the in-between 1.21.6-1.21.11 clients — all with EMPTY
// contents. Presence is what the enchantment registry's freeze validates;
// clients ignore tag names they don't know, so the union is safe. Registries
// 1.21.5 clients don't have (timeline, sound variants, …) are never declared.
func tagsLegacy() []byte {
	tagsLegacyOnce.Do(func() {
		names := map[string][]string{}
		var order []string
		seen := map[string]map[string]bool{}
		add := func(registry, name string) {
			if seen[registry] == nil {
				seen[registry] = map[string]bool{}
				order = append(order, registry)
			}
			if !seen[registry][name] {
				seen[registry][name] = true
				names[registry] = append(names[registry], name)
			}
		}
		for _, reg := range tags1215Data {
			for _, t := range reg.tags {
				add(reg.registry, t.name)
			}
		}
		for _, reg := range tags26xData {
			if seen[reg.registry] == nil || tags26xSkip[reg.registry] {
				continue // registry a 1.21.5-era client can't resolve — skip
			}
			for _, t := range reg.tags {
				add(reg.registry, t.name)
			}
		}
		b := AppendVarInt(nil, int32(len(order)))
		for _, registry := range order {
			b = AppendString(b, registry)
			b = AppendVarInt(b, int32(len(names[registry])))
			for _, n := range names[registry] {
				b = AppendString(b, n)
				ids := legacyFluidTag(registry, n) // fluids carry real ids; all else empty
				b = AppendVarInt(b, int32(len(ids)))
				for _, id := range ids {
					b = AppendVarInt(b, id)
				}
			}
		}
		tagsLegacyBody = b
	})
	return tagsLegacyBody
}

// tags26x builds the Update Tags packet for a 26.x client. 26.2 requires every
// tag its item-component init references to be PRESENT; beyond that, REAL
// contents restore the client-side mechanics driven by tags — mining speed and
// correct-tool (mineable/*, needs_*_tool), fire resistance (damage_type
// is_fire), feeding, and the rest. Entry ids are per-registry: static
// registries use the generated 26.2 maps (ViaVersion order); dynamic
// registries use the order we declare in registry_data; registries with no
// safe id source keep empty tags. 26.1 (775) gets everything empty — its
// registry ids may differ from 26.2's and a bad id is a decode error.
func tags26x(version int32) []byte {
	tags26xMu.Lock()
	defer tags26xMu.Unlock()
	if b, ok := tags26xByVer[version]; ok {
		return b
	}
	full := version >= 776
	dyn := dynamic26xIndex()
	resolverFor := func(registry string) map[string]int32 {
		switch registry {
		case "minecraft:block":
			return block26xID
		case "minecraft:item":
			return item26xID
		case "minecraft:entity_type":
			return entity26xID
		case "minecraft:fluid":
			return fluid26xID
		default:
			return dyn[registry] // nil for undeclared registries → empty tags
		}
	}

	var sent []tagReg26x
	for _, reg := range tags26xData {
		if !tags26xSkip[reg.registry] {
			sent = append(sent, reg)
		}
	}
	b := AppendVarInt(nil, int32(len(sent)))
	for _, reg := range sent {
		b = AppendString(b, reg.registry)
		b = AppendVarInt(b, int32(len(reg.tags)))
		resolver := resolverFor(reg.registry)
		for _, t := range reg.tags {
			b = AppendString(b, t.name)
			var ids []int32
			if full && resolver != nil {
				for _, e := range t.entries {
					if id, ok := resolver[e]; ok {
						ids = append(ids, id) // unknown names dropped, never guessed
					}
				}
			} else if fl := legacyFluidTag(reg.registry, t.name); fl != nil {
				ids = fl // swim physics need #minecraft:water even on 26.1
			}
			b = AppendVarInt(b, int32(len(ids)))
			for _, id := range ids {
				b = AppendVarInt(b, id)
			}
		}
	}
	tags26xByVer[version] = b
	return b
}
