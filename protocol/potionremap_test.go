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
