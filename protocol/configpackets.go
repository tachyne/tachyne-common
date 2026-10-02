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
	return ConfigRegistryPacketsWith(v, overworldHeight, ConfigExtras{})
}

// regEntryOut is one registry_data entry: its name and, when has is set,
// its inline data.
type regEntryOut struct {
	name string
	nbt  []byte
	has  bool
}

// regOut is one registry_data packet before encoding.
type regOut struct {
	id      string
	entries []regEntryOut
}

// configRegistries composes the registry_data packets at version v (see
// ConfigRegistryPackets), with the world's dimension table deciding the
// dimension_type and world_clock entries.
func configRegistries(v int32, overworldHeight int32, dims Dimensions) []regOut {
	inlineOK := v < 775
	var out []regOut
	for _, reg := range SyncedRegistries {
		entries := reg.Entries
		if reg.ID == "minecraft:worldgen/biome" {
			entries = biomeEntriesFor(v, reg.Entries)
		} else if reg.ID == "minecraft:dimension_type" {
			entries = dims.TypeEntries()
		} else if !inlineOK {
			if ex := extra26xEntries[reg.ID]; len(ex) > 0 {
				entries = append(append([]string(nil), reg.Entries...), ex...)
			}
		}
		r := regOut{id: reg.ID}
		for _, entry := range entries {
			inline := inlineOK || reg.ID == "minecraft:dimension_type"
			nbt, hasData := registryEntryDataFor(reg.ID, entry, v, overworldHeight)
			if reg.ID == "minecraft:dimension_type" {
				if own, ok := dims.typeNBT(entry); ok {
					nbt, hasData = own, true
				}
			}
			if hasData && inline {
				r.entries = append(r.entries, regEntryOut{name: entry, nbt: nbt, has: true})
			} else {
				r.entries = append(r.entries, regEntryOut{name: entry}) // resolve via known pack
			}
		}
		out = append(out, r)
	}
	if v >= 775 {
		for _, reg := range extra26xRegistries {
			entries := reg.entries
			if reg.id == "minecraft:world_clock" {
				entries = dims.clocks(reg.entries)
			}
			r := regOut{id: reg.id}
			for _, e := range entries {
				r.entries = append(r.entries, regEntryOut{name: e})
			}
			out = append(out, r)
		}
	}
	if v >= 777 {
		for _, reg := range extra263Registries {
			r := regOut{id: reg.id}
			for _, e := range reg.entries {
				r.entries = append(r.entries, regEntryOut{name: e})
			}
			out = append(out, r)
		}
	}
	return out
}

// encodeRegistries writes each registry_data body: the registry id, the
// entry count, then per entry its name and optional inline data.
func encodeRegistries(rs []regOut) [][]byte {
	out := make([][]byte, 0, len(rs))
	for _, r := range rs {
		data := AppendString(nil, r.id)
		data = AppendVarInt(data, int32(len(r.entries)))
		for _, e := range r.entries {
			data = AppendString(data, e.name)
			data = AppendBool(data, e.has)
			if e.has {
				data = append(data, e.nbt...)
			}
		}
		out = append(out, data)
	}
	return out
}

// extra263Registries are the registries 26.3 made synced on top of 26.2's
// (RegistryDataLoader.SYNCHRONIZED_REGISTRIES): the decorated pot patterns,
// the block transformers (axe/hoe/shovel) and the worldgen block-state
// providers. Declared has_data=false (the client's own pack resolves them);
// entry order = network id, from the 26.3 jar's data folders.
var extra263Registries = []struct {
	id      string
	entries []string
}{
	{"minecraft:decorated_pot_pattern", []string{"minecraft:angler", "minecraft:archer", "minecraft:arms_up", "minecraft:blade", "minecraft:brewer", "minecraft:burn", "minecraft:danger", "minecraft:explorer", "minecraft:flow", "minecraft:friend", "minecraft:guster", "minecraft:heart", "minecraft:heartbreak", "minecraft:howl", "minecraft:miner", "minecraft:mourner", "minecraft:plenty", "minecraft:prize", "minecraft:scrape", "minecraft:sheaf", "minecraft:shelter", "minecraft:skull", "minecraft:snort"}},
	{"minecraft:block_transformer", []string{"minecraft:axe", "minecraft:hoe", "minecraft:shovel"}},
	{"minecraft:worldgen/block_state_provider", []string{"minecraft:cave_vines_body", "minecraft:cave_vines_head", "minecraft:flower_flower_forest", "minecraft:flower_meadow", "minecraft:flower_plain", "minecraft:mangrove_propagule", "minecraft:podzol_beneath_tree", "minecraft:soil_beneath_tree"}},
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
	// Dialogs (1.21.6+): the core pack's three, in the order the server's
	// registry holds them (loaded sorted by id). Declaring the registry is
	// what lets show_dialog name one and the pause_screen_additions /
	// quick_actions tags be sent.
	{"minecraft:dialog", dialogEntries},
}

// dialogEntries is the dialog registry this server declares.
var dialogEntries = []string{"minecraft:custom_options", "minecraft:quick_actions", "minecraft:server_links"}

// DialogRegistryID is a dialog's network id in the declared registry (a
// holder reference sends it +1).
func DialogRegistryID(name string) (int32, bool) {
	for i, e := range dialogEntries {
		if e == name {
			return int32(i), true
		}
	}
	return 0, false
}

// tags26xSkip lists registries whose tags must NOT be sent: they are
// datapack-only and absent at the client's config-time tag resolution, so
// sending them throws "Missing registry".
var tags26xSkip = map[string]bool{
	"minecraft:worldgen/configured_feature":          true,
	"minecraft:worldgen/feature":                     true, // 26.3's name for it (a 26.3 client crashed on it at configuration, 2026-09-19)
	"minecraft:worldgen/flat_level_generator_preset": true,
	"minecraft:worldgen/structure":                   true,
	"minecraft:worldgen/world_preset":                true,
	"minecraft:villager_trade":                       true,
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
func dynamic26xIndex() map[string]map[string]int32 { return dynamic26xIndexFor(776) }

// dynamic26xIndexFor is dynamic26xIndex for one client version: a version's
// own additions (26.3's dappled forest) have ids only on it.
func dynamic26xIndexFor(v int32) map[string]map[string]int32 {
	idx := map[string]map[string]int32{}
	for _, reg := range SyncedRegistries {
		m := map[string]int32{}
		entries := reg.Entries
		if reg.ID == "minecraft:worldgen/biome" {
			entries = biomeEntriesFor(v, reg.Entries)
		} else if ex := extra26xEntries[reg.ID]; len(ex) > 0 {
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
	for _, reg := range extra263Registries { // harmless for 26.2 clients: no 26.2 tag names them
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
// legacyContentRegistries are the registries whose LEGACY (770-774) tags
// carry real memberships: client-side mechanics read them. The loom computes
// its selectable pattern list from the banner_pattern tags. Ids resolve
// against the declared dynamic-registry order (SyncedRegistries), which is
// identical on every version we serve.
var legacyContentRegistries = map[string]bool{"minecraft:banner_pattern": true}

func tagsLegacy() []byte {
	tagsLegacyOnce.Do(func() {
		names := map[string][]string{}
		contents := map[string]map[string][]string{}
		var order []string
		seen := map[string]map[string]bool{}
		add := func(registry, name string, entries []string) {
			if seen[registry] == nil {
				seen[registry] = map[string]bool{}
				contents[registry] = map[string][]string{}
				order = append(order, registry)
			}
			if !seen[registry][name] {
				seen[registry][name] = true
				names[registry] = append(names[registry], name)
				contents[registry][name] = entries
			}
		}
		for _, reg := range tags1215Data {
			for _, t := range reg.tags {
				add(reg.registry, t.name, t.entries)
			}
		}
		for _, reg := range tags26xData {
			if seen[reg.registry] == nil || tags26xSkip[reg.registry] {
				continue // registry a 1.21.5-era client can't resolve — skip
			}
			for _, t := range reg.tags {
				add(reg.registry, t.name, nil)
			}
		}
		dyn := dynamic26xIndex()
		b := AppendVarInt(nil, int32(len(order)))
		for _, registry := range order {
			b = AppendString(b, registry)
			b = AppendVarInt(b, int32(len(names[registry])))
			for _, n := range names[registry] {
				b = AppendString(b, n)
				ids := legacyFluidTag(registry, n) // fluids carry real ids
				if ids == nil && legacyContentRegistries[registry] {
					for _, e := range contents[registry][n] {
						if id, ok := dyn[registry][e]; ok {
							ids = append(ids, id) // unknown names dropped, never guessed
						}
					}
				}
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
	b := buildTags26x(version, dynamic26xIndexFor(version), nil)
	tags26xByVer[version] = b
	return b
}

// buildTags26x composes a 26.x update_tags body against the dynamic
// registries' declared order dyn, with a world's extra tags merged in (a
// tag of an existing name replaced, a new one appended to its registry, a
// registry the set lacks added at the end).
func buildTags26x(version int32, dyn map[string]map[string]int32, extra []TagExtra) []byte {
	full := version >= 776
	// The tag set and the static-registry ids are the client's own version's:
	// 26.3 (777) inserted blocks, items and entities ahead of the 26.2 ids and
	// references tags 26.2 never had.
	data, blockID, itemID, entityID := tags26xData, block26xID, item26xID, entity26xID
	if version >= 777 {
		data, blockID, itemID, entityID = tags263Data, block263ID, item263ID, entity263ID
	}
	resolverFor := func(registry string) map[string]int32 {
		switch registry {
		case "minecraft:block":
			return blockID
		case "minecraft:item":
			return itemID
		case "minecraft:entity_type":
			return entityID
		case "minecraft:fluid":
			return fluid26xID
		default:
			return dyn[registry] // nil for undeclared registries → empty tags
		}
	}

	var sent []tagReg26x
	for _, reg := range data {
		if !tags26xSkip[reg.registry] {
			sent = append(sent, reg)
		}
	}
	for _, t := range extra {
		if tags26xSkip[t.Registry] {
			continue
		}
		i := -1
		for j := range sent {
			if sent[j].registry == t.Registry {
				i = j
				break
			}
		}
		if i < 0 {
			sent = append(sent, tagReg26x{registry: t.Registry})
			i = len(sent) - 1
		}
		// Copy before changing: sent shares the generated tables' slices.
		tags := append([]tag26x(nil), sent[i].tags...)
		replaced := false
		for k := range tags {
			if tags[k].name == t.Name {
				tags[k] = tag26x{name: t.Name, entries: t.Entries}
				replaced = true
				break
			}
		}
		if !replaced {
			tags = append(tags, tag26x{name: t.Name, entries: t.Entries})
		}
		sent[i].tags = tags
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
	return b
}
