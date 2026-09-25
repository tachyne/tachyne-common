package protocol

import "strings"

// DamageTypeID is a damage type's network id: its place in the damage_type
// registry the config phase sent (the synced entries, then the ones 26.x
// appended). ok false for a name the registry lacks.
func DamageTypeID(name string) (int32, bool) {
	if !strings.Contains(name, ":") {
		name = "minecraft:" + name
	}
	for _, reg := range SyncedRegistries {
		if reg.ID != "minecraft:damage_type" {
			continue
		}
		for i, e := range reg.Entries {
			if e == name {
				return int32(i), true
			}
		}
		for i, e := range extra26xEntries[reg.ID] {
			if e == name {
				return int32(len(reg.Entries) + i), true
			}
		}
	}
	return 0, false
}
