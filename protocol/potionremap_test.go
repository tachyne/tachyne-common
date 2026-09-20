package protocol

import (
	"bytes"
	"testing"
)

// potionSlot builds a potion stack carrying potion_contents in canonical (770)
// numbering. The effects are what makes the liquid the right colour: the
// client mixes their registry colours (PotionContents.getColor), so a Healing
// and a Poison stop looking alike.
func potionSlot(item int32, effects [][2]int32, color int32, hasColor, hidden bool) []byte {
	b := AppendVarInt(nil, 1)                    // count
	b = AppendVarInt(b, item)                    // the potion item
	b = AppendVarInt(b, 1)                       // 1 component to add
	b = AppendVarInt(b, 0)                       // 0 to remove
	b = AppendVarInt(b, componentPotionContents) // potion_contents
	b = append(b, 0)                             // no potion holder
	if hasColor {
		b = append(b, 1)
		b = AppendI32(b, color)
	} else {
		b = append(b, 0)
	}
	b = AppendVarInt(b, int32(len(effects)))
	for _, e := range effects {
		b = AppendVarInt(b, e[0]+1) // holder ref = registry id + 1
		b = AppendVarInt(b, 0)      // amplifier
		b = AppendVarInt(b, e[1])   // duration in ticks
		b = append(b, 0, 1, 1)      // ambient, showParticles, showIcon
		if hidden {
			// one nested Details (a weaker effect this one overwrote)
			b = append(b, 1)
			b = AppendVarInt(b, 0)
			b = AppendVarInt(b, 200)
			b = append(b, 0, 1, 1, 0)
		} else {
			b = append(b, 0)
		}
	}
	return append(b, 0) // no custom name
}

// The payload rides through byte-for-byte; only the component id takes the
// client version's number, in both directions.
func TestPotionContentsRenumbersAcrossVersions(t *testing.T) {
	const potion = 1271 // canonical minecraft:potion
	for _, tc := range []struct {
		version int32
		wantID  int32
	}{{770, 42}, {772, 42}, {774, 49}, {775, 51}, {776, 51}, {777, 53}} {
		for _, shape := range []struct {
			name     string
			effects  [][2]int32
			color    int32
			hasColor bool
			hidden   bool
		}{
			{"speed", [][2]int32{{0, 3600}}, 0, false, false},
			{"turtle master", [][2]int32{{1, 400}, {10, 400}}, 0, false, false},
			{"water bottle", nil, 0, false, false},
			{"custom colour", nil, 0x385DC6, true, false},
			{"hidden effect", [][2]int32{{18, 900}}, 0, false, true},
		} {
			body := potionSlot(potion, shape.effects, shape.color, shape.hasColor, shape.hidden)
			var out []byte
			if !copyFullSlot(bytes.NewReader(body), &out, func(i int32) int32 { return i }, tc.version, false) {
				t.Fatalf("v%d %s: the potion_contents case is missing", tc.version, shape.name)
			}
			r := bytes.NewReader(out)
			for i := 0; i < 4; i++ {
				ReadVarInt(r) // count, item, add count, remove count
			}
			cid, _ := ReadVarInt(r)
			if cid != tc.wantID {
				t.Errorf("v%d %s: component id %d, want %d", tc.version, shape.name, cid, tc.wantID)
			}
			payloadIn := body[indexAfterCompID(body):]
			if got := out[len(out)-r.Len():]; !bytes.Equal(got, payloadIn) {
				t.Errorf("v%d %s: payload %x, want %x", tc.version, shape.name, got, payloadIn)
			}
			// And the client's echo (creative set_slot) comes back canonical.
			var back []byte
			if !copyFullSlot(bytes.NewReader(out), &back, func(i int32) int32 { return i }, tc.version, true) {
				t.Fatalf("v%d %s: serverbound copy failed", tc.version, shape.name)
			}
			if !bytes.Equal(back, body) {
				t.Errorf("v%d %s: round trip %x, want %x", tc.version, shape.name, back, body)
			}
		}
	}
}

// A truncated payload must be refused, not half-copied: the copier's job is to
// know the component's exact length so the NEXT component starts in the right
// place.
func TestPotionContentsRejectsTruncated(t *testing.T) {
	body := potionSlot(1271, [][2]int32{{0, 3600}}, 0, false, false)
	for cut := indexAfterCompID(body) + 1; cut < len(body); cut++ {
		var out []byte
		if copyFullSlot(bytes.NewReader(body[:cut]), &out, func(i int32) int32 { return i }, 776, false) {
			t.Fatalf("cut at %d: accepted a truncated potion_contents", cut)
		}
	}
}

// stewSlot builds a suspicious stew carrying its effect, and repairSlot an
// item carrying an anvil prior-work penalty — both plain per-version
// renumberings over a payload that means the same on every client.
func stewSlot(item int32, effect, dur int32) []byte {
	b := AppendVarInt(nil, 1)
	b = AppendVarInt(b, item)
	b = AppendVarInt(b, 1)
	b = AppendVarInt(b, 0)
	b = AppendVarInt(b, componentStewEffects)
	b = AppendVarInt(b, 1)
	b = AppendVarInt(b, effect+1) // holder ref
	return AppendVarInt(b, dur)
}

func repairSlot(item, cost int32) []byte {
	b := AppendVarInt(nil, 1)
	b = AppendVarInt(b, item)
	b = AppendVarInt(b, 1)
	b = AppendVarInt(b, 0)
	b = AppendVarInt(b, componentRepairCost)
	return AppendVarInt(b, cost)
}

func TestStewAndRepairCostRenumber(t *testing.T) {
	for _, tc := range []struct {
		version              int32
		wantStew, wantRepair int32
	}{{770, 44, 16}, {774, 51, 19}, {776, 53, 19}, {777, 55, 19}} {
		for _, c := range []struct {
			name string
			body []byte
			want int32
		}{
			{"suspicious_stew_effects", stewSlot(1108, 22, 7), tc.wantStew},
			{"repair_cost", repairSlot(1043, 7), tc.wantRepair},
		} {
			var out []byte
			if !copyFullSlot(bytes.NewReader(c.body), &out, func(i int32) int32 { return i }, tc.version, false) {
				t.Fatalf("v%d %s: the case is missing", tc.version, c.name)
			}
			r := bytes.NewReader(out)
			for i := 0; i < 4; i++ {
				ReadVarInt(r)
			}
			if cid, _ := ReadVarInt(r); cid != c.want {
				t.Errorf("v%d %s: component id %d, want %d", tc.version, c.name, cid, c.want)
			}
			if got := out[len(out)-r.Len():]; !bytes.Equal(got, c.body[indexAfterCompID(c.body):]) {
				t.Errorf("v%d %s: payload changed", tc.version, c.name)
			}
			var back []byte
			if !copyFullSlot(bytes.NewReader(out), &back, func(i int32) int32 { return i }, tc.version, true) {
				t.Fatalf("v%d %s: serverbound copy failed", tc.version, c.name)
			}
			if !bytes.Equal(back, c.body) {
				t.Errorf("v%d %s: round trip %x, want %x", tc.version, c.name, back, c.body)
			}
		}
	}
}

// containerSlot builds a shulker box carrying contents: a positional list of
// Slots, so the empty ones ride along too.
func containerSlot(box int32, items []int32) []byte {
	b := AppendVarInt(nil, 1)
	b = AppendVarInt(b, box)
	b = AppendVarInt(b, 1)
	b = AppendVarInt(b, 0)
	b = AppendVarInt(b, componentContainer)
	b = AppendVarInt(b, int32(len(items)))
	for _, it := range items {
		if it == 0 {
			b = AppendVarInt(b, 0) // an empty slot
			continue
		}
		b = AppendVarInt(b, 1)
		b = AppendVarInt(b, it)
		b = AppendVarInt(b, 0) // no components of its own
		b = AppendVarInt(b, 0)
	}
	return b
}

// The box's own component id renumbers, and so do the ITEM ids inside it —
// that is the whole reason the copier recurses rather than skipping the
// payload.
func TestContainerContentsRemapInside(t *testing.T) {
	const shift = 3
	remap := func(i int32) int32 { return i + shift }
	for _, tc := range []struct {
		version int32
		wantID  int32
	}{{770, 66}, {774, 73}, {776, 75}, {777, 77}} {
		items := []int32{100, 0, 250, 0, 0}
		body := containerSlot(1190, items)
		var out []byte
		if !copyFullSlot(bytes.NewReader(body), &out, remap, tc.version, false) {
			t.Fatalf("v%d: the container case is missing", tc.version)
		}
		r := bytes.NewReader(out)
		ReadVarInt(r) // count
		if got, _ := ReadVarInt(r); got != 1190+shift {
			t.Errorf("v%d: the box itself is %d, want %d", tc.version, got, 1190+shift)
		}
		ReadVarInt(r) // add count
		ReadVarInt(r) // remove count
		if cid, _ := ReadVarInt(r); cid != tc.wantID {
			t.Errorf("v%d: component id %d, want %d", tc.version, cid, tc.wantID)
		}
		n, _ := ReadVarInt(r)
		if int(n) != len(items) {
			t.Fatalf("v%d: %d slots, want %d", tc.version, n, len(items))
		}
		for j, want := range items {
			cnt, _ := ReadVarInt(r)
			if want == 0 {
				if cnt != 0 {
					t.Errorf("v%d slot %d: count %d, want the empty slot", tc.version, j, cnt)
				}
				continue
			}
			id, _ := ReadVarInt(r)
			if id != want+shift {
				t.Errorf("v%d slot %d: item %d, want %d", tc.version, j, id, want+shift)
			}
			ReadVarInt(r)
			ReadVarInt(r)
		}
		// And the client's echo comes back canonical, items included.
		back := []byte{}
		if !copyFullSlot(bytes.NewReader(out), &back, func(i int32) int32 { return i - shift }, tc.version, true) {
			t.Fatalf("v%d: serverbound copy failed", tc.version)
		}
		if !bytes.Equal(back, body) {
			t.Errorf("v%d: round trip %x, want %x", tc.version, back, body)
		}
	}
}

// The Bad Omen level on a captain's bottle: one varint, per-version id.
func TestOminousBottleRenumbers(t *testing.T) {
	for _, tc := range []struct {
		version int32
		want    int32
	}{{770, 54}, {774, 61}, {776, 63}, {777, 65}} {
		b := AppendVarInt(nil, 1)
		b = AppendVarInt(b, 1504) // minecraft:ominous_bottle
		b = AppendVarInt(b, 1)
		b = AppendVarInt(b, 0)
		b = AppendVarInt(b, componentOminousBottle)
		body := AppendVarInt(b, 3)
		var out []byte
		if !copyFullSlot(bytes.NewReader(body), &out, func(i int32) int32 { return i }, tc.version, false) {
			t.Fatalf("v%d: the ominous_bottle_amplifier case is missing", tc.version)
		}
		r := bytes.NewReader(out)
		for i := 0; i < 4; i++ {
			ReadVarInt(r)
		}
		if cid, _ := ReadVarInt(r); cid != tc.want {
			t.Errorf("v%d: component id %d, want %d", tc.version, cid, tc.want)
		}
		if amp, _ := ReadVarInt(r); amp != 3 {
			t.Errorf("v%d: amplifier %d, want 3", tc.version, amp)
		}
		var back []byte
		if !copyFullSlot(bytes.NewReader(out), &back, func(i int32) int32 { return i }, tc.version, true) {
			t.Fatalf("v%d: serverbound copy failed", tc.version)
		}
		if !bytes.Equal(back, body) {
			t.Errorf("v%d: round trip %x, want %x", tc.version, back, body)
		}
	}
}
