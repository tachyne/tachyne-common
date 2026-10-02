package protocol

// What a world adds to the configuration phase beyond the built-in data:
// its dimension table, and a data pack's registry entries and tags. Vanilla
// sends a data pack's synced registry entries (with their data — the client
// has no pack for them) and its tags in SynchronizeRegistriesTask, and the
// tags again in play after a /reload (ClientboundUpdateTagsPacket).

// RegistryExtra is a data pack's entries in one synced registry. An entry
// under a name the registry already has replaces that entry's data; a new
// name is appended (so the built-in network ids never move). A registry
// this server does not declare is sent as a registry of its own.
type RegistryExtra struct {
	Registry string
	Entries  []RegistryEntry
}

// RegistryEntry is one entry: its name and its network NBT (nil = the
// client resolves it from its own known pack).
type RegistryEntry struct {
	Name string
	NBT  []byte
}

// TagExtra is one tag a data pack defines: its registry, its name and its
// members by name (flattened — no #tag references). A tag the built-in set
// has is replaced; a new one is added.
type TagExtra struct {
	Registry string
	Name     string
	Entries  []string
}

// ConfigExtras is everything a world adds to the configuration phase.
type ConfigExtras struct {
	Dims       Dimensions
	Registries []RegistryExtra
	Tags       []TagExtra
}

// empty reports whether the extras change nothing.
func (x ConfigExtras) empty() bool {
	return len(x.Dims) == 0 && len(x.Registries) == 0 && len(x.Tags) == 0
}

// ConfigRegistryPacketsWith is ConfigRegistryPacketsFor with a world's
// extras: the dimension table's types and clocks, then the data pack's
// registry entries.
func ConfigRegistryPacketsWith(v int32, overworldHeight int32, x ConfigExtras) [][]byte {
	rs := configRegistries(v, overworldHeight, x.Dims.Or())
	for _, rx := range x.Registries {
		i := -1
		for j := range rs {
			if rs[j].id == rx.Registry {
				i = j
				break
			}
		}
		if i < 0 {
			rs = append(rs, regOut{id: rx.Registry})
			i = len(rs) - 1
		}
		for _, e := range rx.Entries {
			out := regEntryOut{name: e.Name, nbt: e.NBT, has: e.NBT != nil}
			replaced := false
			for k := range rs[i].entries {
				if rs[i].entries[k].name == e.Name {
					rs[i].entries[k] = out
					replaced = true
					break
				}
			}
			if !replaced {
				rs[i].entries = append(rs[i].entries, out)
			}
		}
	}
	return encodeRegistries(rs)
}

// dynamicIndexWith is dynamic26xIndexFor with the extras' entries: the
// dimension table's types and clocks, then the data pack's new names, each
// appended in the order ConfigRegistryPacketsWith appends them.
func dynamicIndexWith(v int32, x ConfigExtras) map[string]map[string]int32 {
	idx := dynamic26xIndexFor(v)
	add := func(registry, name string) {
		m := idx[registry]
		if m == nil {
			m = map[string]int32{}
			idx[registry] = m
		}
		if _, ok := m[name]; !ok {
			m[name] = int32(len(m))
		}
	}
	dims := x.Dims.Or()
	for _, e := range dims.TypeEntries() {
		add("minecraft:dimension_type", e)
	}
	for _, d := range dims {
		if d.Clock != "" {
			add("minecraft:world_clock", d.Clock)
		}
	}
	for _, rx := range x.Registries {
		for _, e := range rx.Entries {
			add(rx.Registry, e.Name)
		}
	}
	return idx
}

// UpdateTagsPacketWith is UpdateTagsPacket with a world's extras: its tags
// added or replacing the built-in ones, every name resolved against the
// registries as the extras declare them. It is the configuration phase's
// update_tags and, unchanged, the play-state one.
func UpdateTagsPacketWith(v int32, x ConfigExtras) []byte {
	if x.empty() || v < 776 {
		return UpdateTagsPacket(v)
	}
	return buildTags26x(v, dynamicIndexWith(v, x), x.Tags)
}
