package render770

// Strict re-parser tests for update_advancements, mirroring the vanilla
// client read paths: 1.21.5 ClientboundUpdateAdvancementsPacket +
// Advancement.read + DisplayInfo.fromNetwork + AdvancementProgress.fromNetwork
// at 770, and the 26.2 codecs (icon as ItemStackTemplate) after the chain
// lifts the packet to 776. Every byte must be consumed.

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"testing"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

func rdString(t *testing.T, r *bytes.Reader) string {
	t.Helper()
	s, err := protocol.ReadString(r)
	if err != nil {
		t.Fatalf("string: %v", err)
	}
	return s
}

func rdVarInt(t *testing.T, r *bytes.Reader) int32 {
	t.Helper()
	v, err := protocol.ReadVarInt(r)
	if err != nil {
		t.Fatalf("varint: %v", err)
	}
	return v
}

func rdBool(t *testing.T, r *bytes.Reader) bool {
	t.Helper()
	b, err := r.ReadByte()
	if err != nil {
		t.Fatalf("bool: %v", err)
	}
	return b != 0
}

type parsedDisplay struct {
	item, count int32
	frame       int32
	background  string
	hasBG       bool
	x, y        float32
}

// rdDisplay consumes a DisplayInfo. template selects the ≥775 icon order
// (item, count, patch) vs the 770 Slot order (count, item, patch).
func rdDisplay(t *testing.T, r *bytes.Reader, template bool) parsedDisplay {
	t.Helper()
	if err := protocol.SkipNetworkNBT(r); err != nil { // title
		t.Fatalf("title NBT: %v", err)
	}
	if err := protocol.SkipNetworkNBT(r); err != nil { // description
		t.Fatalf("desc NBT: %v", err)
	}
	var d parsedDisplay
	if template {
		d.item = rdVarInt(t, r)
		d.count = rdVarInt(t, r)
	} else {
		d.count = rdVarInt(t, r)
		if d.count <= 0 {
			t.Fatalf("empty icon slot")
		}
		d.item = rdVarInt(t, r)
	}
	if a, rm := rdVarInt(t, r), rdVarInt(t, r); a != 0 || rm != 0 {
		t.Fatalf("icon carries components: +%d -%d", a, rm)
	}
	d.frame = rdVarInt(t, r)
	var flags [4]byte
	if _, err := io.ReadFull(r, flags[:]); err != nil {
		t.Fatalf("flags: %v", err)
	}
	if flags[3]&1 != 0 {
		d.hasBG = true
		d.background = rdString(t, r)
	}
	var xy [8]byte
	if _, err := io.ReadFull(r, xy[:]); err != nil {
		t.Fatalf("xy: %v", err)
	}
	d.x = math.Float32frombits(binary.BigEndian.Uint32(xy[0:]))
	d.y = math.Float32frombits(binary.BigEndian.Uint32(xy[4:]))
	return d
}

func testTree() attach.AdvTree {
	return attach.AdvTree{Nodes: []attach.AdvNode{
		{
			ID: "minecraft:story/root", Reqs: [][]string{{"crafting_table"}},
			HasDisplay: true, Title: "advancements.story.root.title",
			Desc:       "advancements.story.root.description",
			Icon:       attach.ItemStack{ID: 294, Count: 1},
			Background: "gui/advancements/backgrounds/stone",
			ShowToast:  false, Announce: false,
		},
		{
			ID: "minecraft:story/mine_stone", Parent: "minecraft:story/root",
			Reqs:       [][]string{{"get_stone"}, {"also_this", "or_this"}},
			HasDisplay: true, Title: "advancements.story.mine_stone.title",
			Desc:      "advancements.story.mine_stone.description",
			Icon:      attach.ItemStack{ID: 913, Count: 1},
			ShowToast: true, Announce: true, X: 1, Y: 1.75,
		},
		{ID: "minecraft:invisible/helper", Parent: "minecraft:story/root",
			Reqs: [][]string{{"c"}}},
	}}
}

func testProgress() attach.AdvProgress {
	return attach.AdvProgress{Reset: true, Entries: []attach.AdvProgressEntry{
		{ID: "minecraft:story/mine_stone",
			Done: map[string]int64{"get_stone": 1752200000000}},
	}}
}

// reparseUpdateAdvancements strictly consumes a whole update_advancements
// body in the given icon layout and returns what it saw.
func reparseUpdateAdvancements(t *testing.T, body []byte, template bool) (nodes map[string]parsedDisplay, reqs map[string][][]string, progress map[string]map[string]int64) {
	t.Helper()
	r := bytes.NewReader(body)
	if !rdBool(t, r) {
		t.Fatalf("reset flag not set")
	}
	nodes = map[string]parsedDisplay{}
	reqs = map[string][][]string{}
	n := rdVarInt(t, r)
	for i := int32(0); i < n; i++ {
		id := rdString(t, r)
		if rdBool(t, r) {
			rdString(t, r) // parent
		}
		if rdBool(t, r) {
			nodes[id] = rdDisplay(t, r, template)
		}
		ng := rdVarInt(t, r)
		var groups [][]string
		for g := int32(0); g < ng; g++ {
			nc := rdVarInt(t, r)
			var group []string
			for c := int32(0); c < nc; c++ {
				group = append(group, rdString(t, r))
			}
			groups = append(groups, group)
		}
		reqs[id] = groups
		if rdBool(t, r) {
			t.Fatalf("%s: sends_telemetry_event should be false", id)
		}
	}
	if removed := rdVarInt(t, r); removed != 0 {
		t.Fatalf("removed = %d", removed)
	}
	progress = map[string]map[string]int64{}
	np := rdVarInt(t, r)
	for i := int32(0); i < np; i++ {
		id := rdString(t, r)
		nc := rdVarInt(t, r)
		m := map[string]int64{}
		for c := int32(0); c < nc; c++ {
			name := rdString(t, r)
			if rdBool(t, r) {
				var ms [8]byte
				if _, err := io.ReadFull(r, ms[:]); err != nil {
					t.Fatalf("instant: %v", err)
				}
				m[name] = int64(binary.BigEndian.Uint64(ms[:]))
			} else {
				m[name] = -1 // present, unobtained
			}
		}
		progress[id] = m
	}
	if !rdBool(t, r) {
		t.Fatalf("showAdvancements false")
	}
	if r.Len() != 0 {
		t.Fatalf("%d trailing bytes", r.Len())
	}
	return
}

func TestAdvancementsInitReparse770(t *testing.T) {
	pkt := AdvancementsInit(testTree(), testProgress())
	if pkt.ID != IDUpdateAdvancements {
		t.Fatalf("packet id 0x%x", pkt.ID)
	}
	nodes, reqs, progress := reparseUpdateAdvancements(t, pkt.Body, false)
	root := nodes["minecraft:story/root"]
	if root.item != 294 || root.count != 1 || !root.hasBG ||
		root.background != "gui/advancements/backgrounds/stone" {
		t.Fatalf("root display: %+v", root)
	}
	ms := nodes["minecraft:story/mine_stone"]
	if ms.item != 913 || ms.x != 1 || ms.y != 1.75 || ms.hasBG {
		t.Fatalf("mine_stone display: %+v", ms)
	}
	if _, ok := nodes["minecraft:invisible/helper"]; ok {
		t.Fatalf("helper node grew a display")
	}
	if g := reqs["minecraft:story/mine_stone"]; len(g) != 2 || g[0][0] != "get_stone" || len(g[1]) != 2 {
		t.Fatalf("requirements: %+v", g)
	}
	p := progress["minecraft:story/mine_stone"]
	if p["get_stone"] != 1752200000000 {
		t.Fatalf("obtained millis: %d", p["get_stone"])
	}
	// unobtained criteria of the same advancement ride along as nulls
	if v, ok := p["also_this"]; !ok || v != -1 {
		t.Fatalf("unobtained criterion missing: %+v", p)
	}
}

// TestAdvancementsChainTo776 lifts the packet through the full 770→776 chain:
// id renumbered to the 26.2 update_advancements, icons re-ordered to
// ItemStackTemplate and item ids remapped to the 776 registry.
func TestAdvancementsChainTo776(t *testing.T) {
	tr := protocol.TranslatorFor(776)
	if tr == nil {
		t.Fatal("no 776 translator")
	}
	pkt := AdvancementsInit(testTree(), testProgress())
	id, body, drop := tr.Clientbound(protocol.StatePlay, pkt.ID, pkt.Body)
	if drop {
		t.Fatal("packet dropped")
	}
	if id != 0x82 { // the 26.2 update_advancements id
		t.Fatalf("776 id = 0x%x, want 0x82", id)
	}
	nodes, _, progress := reparseUpdateAdvancements(t, body, true)
	want294 := protocol.RemapID(protocol.RegItem, 776, 294)
	want913 := protocol.RemapID(protocol.RegItem, 776, 913)
	if nodes["minecraft:story/root"].item != want294 {
		t.Fatalf("root icon %d, want %d", nodes["minecraft:story/root"].item, want294)
	}
	if nodes["minecraft:story/mine_stone"].item != want913 {
		t.Fatalf("mine_stone icon %d, want %d", nodes["minecraft:story/mine_stone"].item, want913)
	}
	if progress["minecraft:story/mine_stone"]["get_stone"] != 1752200000000 {
		t.Fatal("progress corrupted by the chain")
	}
}
