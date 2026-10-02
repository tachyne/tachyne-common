package protocol

import (
	"bytes"
	"testing"
)

// JSONToNBT, byte by byte: sorted compound keys; an int, a string, a list
// whose int and double widen to doubles, a boolean byte, and a mixed list as
// wrapped compounds ({"": value}, ListTag's heterogeneous form).
func TestJSONToNBT(t *testing.T) {
	got, ok := JSONToNBT([]byte(`{"e":[1,"s"],"d":true,"c":[1,2.5],"b":"x","a":1}`))
	if !ok {
		t.Fatal("not converted")
	}
	w := []byte{nbtCompound}
	w = append(w, nbtInt, 0, 1, 'a')
	w = AppendI32(w, 1)
	w = append(w, nbtString, 0, 1, 'b', 0, 1, 'x')
	w = append(w, nbtList, 0, 1, 'c', nbtDouble)
	w = AppendI32(w, 2)
	w = AppendF64(w, 1)
	w = AppendF64(w, 2.5)
	w = append(w, nbtByte, 0, 1, 'd', 1)
	w = append(w, nbtList, 0, 1, 'e', nbtCompound)
	w = AppendI32(w, 2)
	w = append(w, nbtInt, 0, 0)
	w = AppendI32(w, 1)
	w = append(w, nbtEnd)
	w = append(w, nbtString, 0, 0, 0, 1, 's', nbtEnd)
	w = append(w, nbtEnd)
	if !bytes.Equal(got, w) {
		t.Fatalf("nbt\n got %x\nwant %x", got, w)
	}
	if _, ok := JSONToNBT([]byte(`null`)); ok {
		t.Fatal("null converted")
	}
	if b, ok := JSONToNBT([]byte(`"hi"`)); !ok || !bytes.Equal(b, []byte{nbtString, 0, 2, 'h', 'i'}) {
		t.Fatalf("string root %x", b)
	}
}

// regEntry is one parsed registry_data entry.
type regEntry struct {
	name string
	has  bool
	nbt  []byte
}

// parseRegistries reads registry_data bodies back: id, count, then name,
// has-data and the data.
func parseRegistries(t *testing.T, pkts [][]byte) map[string][]regEntry {
	t.Helper()
	out := map[string][]regEntry{}
	for _, p := range pkts {
		r := bytes.NewReader(p)
		id, err := ReadString(r)
		if err != nil {
			t.Fatal(err)
		}
		n, err := ReadVarInt(r)
		if err != nil {
			t.Fatal(err)
		}
		for i := int32(0); i < n; i++ {
			name, err := ReadString(r)
			if err != nil {
				t.Fatal(err)
			}
			has, _ := r.ReadByte()
			e := regEntry{name: name, has: has == 1}
			if e.has {
				start := len(p) - r.Len()
				if err := SkipNetworkNBT(r); err != nil {
					t.Fatal(err)
				}
				e.nbt = p[start : len(p)-r.Len()]
			}
			out[id] = append(out[id], e)
		}
		if r.Len() != 0 {
			t.Fatalf("%s: %d trailing bytes", id, r.Len())
		}
	}
	return out
}

func moonDims() Dimensions {
	return append(append(Dimensions(nil), DefaultDimensions...),
		Dimension{ID: 3, Key: "tachyne:moon", Type: "tachyne:moon", TypeNBT: []byte{nbtCompound, nbtEnd}, SkyLight: true, Clock: "tachyne:moon"})
}

// A fourth dimension: its type is appended to dimension_type with its own
// data inline, its clock to world_clock; the default table reproduces the
// built-in registries exactly.
func TestFourthDimensionRegistries(t *testing.T) {
	for _, v := range []int32{776, 777} {
		base := ConfigRegistryPacketsFor(v, 0)
		def := ConfigRegistryPacketsWith(v, 0, ConfigExtras{Dims: DefaultDimensions})
		if len(base) != len(def) {
			t.Fatalf("%d: default table changed the packet count", v)
		}
		for i := range base {
			if !bytes.Equal(base[i], def[i]) {
				t.Fatalf("%d: default table changed registry packet %d", v, i)
			}
		}
		regs := parseRegistries(t, ConfigRegistryPacketsWith(v, 0, ConfigExtras{Dims: moonDims()}))
		dt := regs["minecraft:dimension_type"]
		if len(dt) != 5 || dt[4].name != "tachyne:moon" || !dt[4].has || !bytes.Equal(dt[4].nbt, []byte{nbtCompound, nbtEnd}) {
			t.Fatalf("%d: dimension_type %+v", v, dt)
		}
		for i, want := range []string{"minecraft:overworld", "minecraft:overworld_caves", "minecraft:the_end", "minecraft:the_nether"} {
			if dt[i].name != want {
				t.Fatalf("%d: built-in type %d moved: %q", v, i, dt[i].name)
			}
		}
		wc := regs["minecraft:world_clock"]
		if len(wc) != 3 || wc[2].name != "tachyne:moon" {
			t.Fatalf("%d: world_clock %+v", v, wc)
		}
	}
	d := moonDims()
	if d.TypeID(3) != 4 || d.Key(3) != "tachyne:moon" || !d.SkyLight(3) {
		t.Fatalf("moon: type %d key %q", d.TypeID(3), d.Key(3))
	}
	if d.TypeID(1) != DimensionNetherID || d.TypeID(2) != DimensionEndID || d.TypeID(0) != DimensionOverworldID {
		t.Fatal("built-in dimension type ids moved")
	}
	var none Dimensions
	if none.Key(1) != "minecraft:the_nether" || none.Key(7) != "minecraft:overworld" || none.SkyLight(1) {
		t.Fatal("default table lookups")
	}
}

// A data pack's registry entries: a new name is appended with its data, an
// existing one keeps its place and takes the new data.
func TestRegistryExtrasAppendAndReplace(t *testing.T) {
	base := parseRegistries(t, ConfigRegistryPacketsFor(777, 0))["minecraft:painting_variant"]
	if len(base) == 0 {
		t.Fatal("no painting_variant registry")
	}
	data := []byte{nbtCompound, nbtEnd}
	x := ConfigExtras{Registries: []RegistryExtra{{Registry: "minecraft:painting_variant", Entries: []RegistryEntry{
		{Name: "tachyne:sunset", NBT: data}, {Name: base[0].name, NBT: data},
	}}, {Registry: "tachyne:things", Entries: []RegistryEntry{{Name: "tachyne:a"}}}}}
	regs := parseRegistries(t, ConfigRegistryPacketsWith(777, 0, x))
	pv := regs["minecraft:painting_variant"]
	if len(pv) != len(base)+1 || pv[len(pv)-1].name != "tachyne:sunset" || !pv[len(pv)-1].has {
		t.Fatalf("appended entry %+v", pv[len(pv)-1])
	}
	if pv[0].name != base[0].name || !pv[0].has || !bytes.Equal(pv[0].nbt, data) {
		t.Fatalf("replaced entry %+v", pv[0])
	}
	if th := regs["tachyne:things"]; len(th) != 1 || th[0].name != "tachyne:a" || th[0].has {
		t.Fatalf("new registry %+v", th)
	}
}

// parseTags reads an update_tags body: registry → tag → ids.
func parseTags(t *testing.T, b []byte) map[string]map[string][]int32 {
	t.Helper()
	r := bytes.NewReader(b)
	out := map[string]map[string][]int32{}
	n, err := ReadVarInt(r)
	if err != nil {
		t.Fatal(err)
	}
	for i := int32(0); i < n; i++ {
		reg, _ := ReadString(r)
		m := map[string][]int32{}
		c, _ := ReadVarInt(r)
		for j := int32(0); j < c; j++ {
			name, _ := ReadString(r)
			k, _ := ReadVarInt(r)
			ids := []int32{}
			for l := int32(0); l < k; l++ {
				id, _ := ReadVarInt(r)
				ids = append(ids, id)
			}
			m[name] = ids
		}
		out[reg] = m
	}
	if r.Len() != 0 {
		t.Fatalf("%d trailing bytes", r.Len())
	}
	return out
}

// A data pack's tags: a tag naming the pack's own painting resolves to the
// id the registry appended it at; a new block tag resolves through the
// client version's block ids; the built-in tags are untouched; with no
// extras the body is the built-in one.
func TestTagExtras(t *testing.T) {
	if !bytes.Equal(UpdateTagsPacketWith(777, ConfigExtras{}), UpdateTagsPacket(777)) {
		t.Fatal("no extras changed the tags")
	}
	n := int32(len(dynamic26xIndexFor(777)["minecraft:painting_variant"]))
	x := ConfigExtras{
		Registries: []RegistryExtra{{Registry: "minecraft:painting_variant", Entries: []RegistryEntry{{Name: "tachyne:sunset", NBT: []byte{nbtCompound, nbtEnd}}}}},
		Tags: []TagExtra{
			{Registry: "minecraft:painting_variant", Name: "minecraft:placeable", Entries: []string{"tachyne:sunset"}},
			{Registry: "minecraft:block", Name: "tachyne:shiny", Entries: []string{"minecraft:stone", "minecraft:nope"}},
		},
	}
	got := parseTags(t, UpdateTagsPacketWith(777, x))
	if ids := got["minecraft:painting_variant"]["minecraft:placeable"]; len(ids) != 1 || ids[0] != n {
		t.Fatalf("placeable %v, want [%d]", ids, n)
	}
	if ids := got["minecraft:block"]["tachyne:shiny"]; len(ids) != 1 || ids[0] != block263ID["minecraft:stone"] {
		t.Fatalf("shiny %v", ids)
	}
	base := parseTags(t, UpdateTagsPacket(777))
	for name, ids := range base["minecraft:block"] {
		if g := got["minecraft:block"][name]; len(g) != len(ids) {
			t.Fatalf("built-in block tag %s changed", name)
		}
	}
	if len(got["minecraft:block"]) != len(base["minecraft:block"])+1 {
		t.Fatal("the new block tag was not added once")
	}
}
