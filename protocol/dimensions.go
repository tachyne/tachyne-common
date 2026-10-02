package protocol

// The dimensions a world runs, as a table (vanilla's LevelStem list: a
// level key and its dimension type). The engine supplies the table at login
// (attach Welcome) and on reconfiguration; an empty table is the three
// dimensions every world has run so far. Everything that names a dimension
// on the wire — the login packet's level list and spawn info, respawn, a
// GlobalPos, a chunk's sky light, the dimension_type and world_clock
// registries — reads it.

// Dimension is one dimension: ID is the engine's number for it (what attach
// Want, Dimension, DeathPos and chunk headers carry), Key the level's
// registry key, Type its dimension_type entry. TypeNBT is the network NBT
// of a type this server does not ship (nil = a built-in type), sent inline
// in the dimension_type registry. Clock is the 26.x world_clock entry the
// dimension's time runs on ("" = none of its own).
type Dimension struct {
	ID       int32
	Key      string
	Type     string
	TypeNBT  []byte
	SkyLight bool
	Clock    string
}

// Dimensions is a world's dimension table.
type Dimensions []Dimension

// DefaultDimensions is the overworld, the Nether and the End with the ids
// the engine has always used.
var DefaultDimensions = Dimensions{
	{ID: 0, Key: "minecraft:overworld", Type: "minecraft:overworld", SkyLight: true, Clock: "minecraft:overworld"},
	{ID: 1, Key: "minecraft:the_nether", Type: "minecraft:the_nether"},
	{ID: 2, Key: "minecraft:the_end", Type: "minecraft:the_end", Clock: "minecraft:the_end"},
}

// Or returns ds, or the default table when ds is empty.
func (ds Dimensions) Or() Dimensions {
	if len(ds) == 0 {
		return DefaultDimensions
	}
	return ds
}

// ByID finds a dimension; an unknown id is the table's first (the
// overworld), which is where a stray id has always landed.
func (ds Dimensions) ByID(id int32) Dimension {
	ds = ds.Or()
	for _, d := range ds {
		if d.ID == id {
			return d
		}
	}
	return ds[0]
}

// Key is a dimension's level key.
func (ds Dimensions) Key(id int32) string { return ds.ByID(id).Key }

// SkyLight reports whether a dimension has sky light.
func (ds Dimensions) SkyLight(id int32) bool { return ds.ByID(id).SkyLight }

// dimensionTypeBase is the dimension_type registry as this server declares
// it before any table additions.
func dimensionTypeBase() []string {
	for _, reg := range SyncedRegistries {
		if reg.ID == "minecraft:dimension_type" {
			return reg.Entries
		}
	}
	return nil
}

// TypeEntries is the declared dimension_type registry: the built-in entries,
// then every table type they lack, in table order.
func (ds Dimensions) TypeEntries() []string {
	out := append([]string(nil), dimensionTypeBase()...)
	have := map[string]bool{}
	for _, e := range out {
		have[e] = true
	}
	for _, d := range ds.Or() {
		if d.Type != "" && !have[d.Type] {
			have[d.Type] = true
			out = append(out, d.Type)
		}
	}
	return out
}

// TypeID is a dimension's dimension_type network id (its index in
// TypeEntries); the overworld's for an unknown type.
func (ds Dimensions) TypeID(id int32) int32 {
	t := ds.ByID(id).Type
	for i, e := range ds.TypeEntries() {
		if e == t {
			return int32(i)
		}
	}
	return DimensionOverworldID
}

// typeNBT is the table's own data for a dimension type, if any.
func (ds Dimensions) typeNBT(typ string) ([]byte, bool) {
	for _, d := range ds.Or() {
		if d.Type == typ && d.TypeNBT != nil {
			return d.TypeNBT, true
		}
	}
	return nil, false
}

// clocks is the world_clock registry with the table's clocks appended.
func (ds Dimensions) clocks(base []string) []string {
	out := append([]string(nil), base...)
	have := map[string]bool{}
	for _, e := range out {
		have[e] = true
	}
	for _, d := range ds.Or() {
		if d.Clock != "" && !have[d.Clock] {
			have[d.Clock] = true
			out = append(out, d.Clock)
		}
	}
	return out
}
