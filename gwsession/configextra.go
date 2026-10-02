package gwsession

import (
	"log"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// configExtras turns the world's configuration data into what the protocol
// composition takes: data-pack JSON elements become network NBT. An element
// that is not valid JSON is sent without data (the client's own pack, or
// nothing) and logged.
func configExtras(d *attach.ConfigData) protocol.ConfigExtras {
	var x protocol.ConfigExtras
	if d == nil {
		return x
	}
	x.Dims = dimsOf(d.Dimensions)
	x.Registries = registryExtras(d.Registries)
	x.Tags = tagExtras(d.Tags)
	return x
}

// dimsOf is the dimension table (nil = the default three).
func dimsOf(in []attach.DimensionInfo) protocol.Dimensions {
	var out protocol.Dimensions
	for _, di := range in {
		d := protocol.Dimension{ID: di.ID, Key: di.Key, Type: di.Type, SkyLight: di.SkyLight, Clock: di.Clock}
		if d.Type == "" {
			d.Type = d.Key
		}
		if len(di.TypeData) > 0 {
			if nbt, ok := protocol.JSONToNBT(di.TypeData); ok {
				d.TypeNBT = nbt
			} else {
				log.Printf("configuration: dimension type %s: data is not JSON", d.Type)
			}
		}
		out = append(out, d)
	}
	return out
}

func registryExtras(in []attach.RegistryEntries) []protocol.RegistryExtra {
	var out []protocol.RegistryExtra
	for _, r := range in {
		rx := protocol.RegistryExtra{Registry: r.Registry}
		for _, e := range r.Entries {
			pe := protocol.RegistryEntry{Name: e.Name}
			if len(e.Data) > 0 {
				if nbt, ok := protocol.JSONToNBT(e.Data); ok {
					pe.NBT = nbt
				} else {
					log.Printf("configuration: %s %s: data is not JSON", r.Registry, e.Name)
				}
			}
			rx.Entries = append(rx.Entries, pe)
		}
		out = append(out, rx)
	}
	return out
}

func tagExtras(in []attach.TagSet) []protocol.TagExtra {
	var out []protocol.TagExtra
	for _, set := range in {
		for _, t := range set.Tags {
			out = append(out, protocol.TagExtra{Registry: set.Registry, Name: t.Name, Entries: t.Entries})
		}
	}
	return out
}

// welcomeDims is a Welcome's dimension table (nil = the default three).
func welcomeDims(w attach.Welcome) protocol.Dimensions {
	if w.Config == nil {
		return nil
	}
	return dimsOf(w.Config.Dimensions)
}
