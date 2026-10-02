package attach

import "encoding/json"

// Configuration frames: what a world adds to the configuration phase (its
// dimension table, a data pack's registry entries and tags), and the
// reconfiguration round trip — vanilla ServerGamePacketListenerImpl.
// switchToConfig, the new ServerConfigurationPacketListenerImpl's tasks,
// then PlayerList.placeNewPlayer once the client finishes.

// FeatureReconfigure is the Hello feature a gateway advertises when it
// handles MsgStartConfiguration (the Java gateway). A session without it is
// never sent the frame; send it a MsgUpdateTags instead, if it renders that.
const FeatureReconfigure = "reconfigure"

// DimensionInfo is one dimension the world runs (vanilla LevelStem): the
// engine's id for it (what Want, Dimension, DeathPos and chunk headers
// carry), the level key, its dimension_type entry, and that type's data in
// the data-pack JSON form when it is not one of the built-in types (the
// gateway sends it inline). SkyLight says whether its chunks carry sky
// light; Clock is the 26.x world_clock its time runs on ("" = none).
type DimensionInfo struct {
	ID       int32           `json:"id"`
	Key      string          `json:"key"`
	Type     string          `json:"type"`
	TypeData json.RawMessage `json:"type_data,omitempty"`
	SkyLight bool            `json:"sky_light,omitempty"`
	Clock    string          `json:"clock,omitempty"`
}

// RegistryEntries is a data pack's entries in one synced registry, each
// with its element in the data-pack JSON form (no data = the client's own
// pack has it). A name the registry has replaces that entry; a new one is
// appended.
type RegistryEntries struct {
	Registry string          `json:"registry"`
	Entries  []RegistryEntry `json:"entries"`
}

// RegistryEntry is one registry element.
type RegistryEntry struct {
	Name string          `json:"name"`
	Data json.RawMessage `json:"data,omitempty"`
}

// TagSet is a data pack's tags in one registry, each a flat list of member
// names (tag references already expanded, as the network form requires).
// A tag the built-in set has is replaced; a new one is added; an empty list
// is a tag that no longer loads. The registry may be its key
// ("minecraft:block") or its data-pack folder ("block", "worldgen/biome").
// The engine's tagSet (registry → tag id → member ids) is exactly this, so
// pack.tags.changedTags() maps one entry per registry.
type TagSet struct {
	Registry string `json:"registry"`
	Tags     []Tag  `json:"tags"`
}

// Tag is one tag: its name and members.
type Tag struct {
	Name    string   `json:"name"`
	Entries []string `json:"entries"`
}

// ConfigData is what the world adds to configuration. Empty fields keep the
// built-in data (and the three default dimensions).
type ConfigData struct {
	Dimensions []DimensionInfo   `json:"dimensions,omitempty"`
	Registries []RegistryEntries `json:"registries,omitempty"`
	Tags       []TagSet          `json:"tags,omitempty"`
}

// MsgStartConfiguration (w→gw): send the player back to configuration
// (ServerGamePacketListenerImpl.switchToConfig): the gateway sends
// start_configuration and stops sending play packets; on the client's
// configuration_acknowledged it runs the configuration phase again with
// this data, and on finish_configuration it answers MsgConfigured. The
// world has taken the player out of the level (removePlayerFromWorld) and
// sends nothing for it until it answers that with MsgRejoin.
const MsgStartConfiguration = 0xa7

// StartConfiguration carries the configuration data for the new phase.
type StartConfiguration struct {
	ConfigData
}

// MsgConfigured (gw→w): the client finished configuration
// (ServerConfigurationPacketListenerImpl.handleConfigurationFinished). The
// world places the player again (PlayerList.placeNewPlayer): it answers
// with MsgRejoin and then sends everything a join sends — the client has
// discarded its level, so every chunk, entity and screen goes again.
const MsgConfigured = 0xa8

// Configured carries the view distance the client reported during the
// phase (0 = it sent none; keep the old one).
type Configured struct {
	View int32 `json:"view,omitempty"`
}

// MsgRejoin (w→gw): the player is in the level again; the gateway sends the
// login packet from this Welcome, then resumes play. Frames before it are
// dropped by the gateway (the client is between phases).
const MsgRejoin = 0xa9

// Rejoin is the fresh join state: the player's entity id, position,
// dimension (Welcome.Dim), game mode and the rest. Its Config is ignored —
// the configuration it rejoins after was the MsgStartConfiguration's.
type Rejoin struct {
	Welcome Welcome `json:"welcome"`
}

// MsgUpdateTags (w→gw): the tags changed in play (a /reload,
// ClientboundUpdateTagsPacket): the gateway sends the full set again, with
// these replacing or adding tags. Registries cannot change in play, so the
// members resolve against the registries the last configuration declared
// (Welcome.Config or the last MsgStartConfiguration).
const MsgUpdateTags = 0xaa

// UpdateTags carries the world's tags (the whole data-pack set: tags it no
// longer has go back to the built-in ones).
type UpdateTags struct {
	Tags []TagSet `json:"tags,omitempty"`
}
