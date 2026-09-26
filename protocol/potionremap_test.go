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
			var cnt, id int32
			if templateStacks(tc.version) {
				// 26.x: Optional<ItemStackTemplate> — a present flag, then
				// item before count.
				flag, _ := r.ReadByte()
				if flag == 1 {
					id, _ = ReadVarInt(r)
					cnt, _ = ReadVarInt(r)
				}
			} else {
				cnt, _ = ReadVarInt(r)
				if cnt > 0 {
					id, _ = ReadVarInt(r)
				}
			}
			if want == 0 {
				if cnt != 0 {
					t.Errorf("v%d slot %d: count %d, want the empty slot", tc.version, j, cnt)
				}
				continue
			}
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

// A rocket's flight duration and bursts, and a star's single burst. The
// colours inside a burst are fixed-width ints, which is the part a walker
// gets wrong if it assumes varints everywhere.
func TestFireworkComponentsRenumber(t *testing.T) {
	burst := func(b []byte) []byte {
		b = AppendVarInt(b, 2)     // shape: large ball
		b = AppendVarInt(b, 2)     // two colours
		b = AppendI32(b, 0xE84A4A) //
		b = AppendI32(b, 0x4A90E8) //
		b = AppendVarInt(b, 1)     // one fade colour
		b = AppendI32(b, 0xF0F0F0) //
		return append(b, 1, 0)     // trail, no twinkle
	}
	for _, tc := range []struct {
		version              int32
		wantRocket, wantStar int32
	}{{770, 60, 59}, {774, 67, 66}, {776, 69, 68}, {777, 71, 70}} {
		rocket := AppendVarInt(nil, 1)
		rocket = AppendVarInt(rocket, 1242) // firework_rocket
		rocket = AppendVarInt(rocket, 1)
		rocket = AppendVarInt(rocket, 0)
		rocket = AppendVarInt(rocket, componentFireworks)
		rocket = AppendVarInt(rocket, 3) // flight duration
		rocket = AppendVarInt(rocket, 1) // one burst
		rocket = burst(rocket)

		star := AppendVarInt(nil, 1)
		star = AppendVarInt(star, 1243) // firework_star
		star = AppendVarInt(star, 1)
		star = AppendVarInt(star, 0)
		star = AppendVarInt(star, componentFireworkStar)
		star = burst(star)

		for _, c := range []struct {
			name string
			body []byte
			want int32
		}{{"fireworks", rocket, tc.wantRocket}, {"firework_explosion", star, tc.wantStar}} {
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

// pot_decorations is the one component whose payload is ITEM ids, so the
// items inside it must be remapped exactly like the stack's own — a
// pass-through would put the wrong sherd on every face of every pot.
func TestPotDecorationsRemapTheirItems(t *testing.T) {
	const shift = 7
	remap := func(i int32) int32 { return i + shift }
	faces := []int32{1445, 1025, 1450, 1467} // angler, brick, heart, snort
	for _, tc := range []struct {
		version int32
		wantID  int32
	}{{770, 65}, {774, 72}, {776, 74}, {777, 76}} {
		body := AppendVarInt(nil, 1)
		body = AppendVarInt(body, 319) // decorated_pot
		body = AppendVarInt(body, 1)
		body = AppendVarInt(body, 0)
		body = AppendVarInt(body, componentPotDecorations)
		body = AppendVarInt(body, int32(len(faces)))
		for _, f := range faces {
			body = AppendVarInt(body, f)
		}
		var out []byte
		if !copyFullSlot(bytes.NewReader(body), &out, remap, tc.version, false) {
			t.Fatalf("v%d: the pot_decorations case is missing", tc.version)
		}
		r := bytes.NewReader(out)
		ReadVarInt(r) // count
		if got, _ := ReadVarInt(r); got != 319+shift {
			t.Errorf("v%d: the pot itself is %d, want %d", tc.version, got, 319+shift)
		}
		ReadVarInt(r) // add
		ReadVarInt(r) // remove
		if cid, _ := ReadVarInt(r); cid != tc.wantID {
			t.Errorf("v%d: component id %d, want %d", tc.version, cid, tc.wantID)
		}
		n, _ := ReadVarInt(r)
		if int(n) != len(faces) {
			t.Fatalf("v%d: %d faces, want %d", tc.version, n, len(faces))
		}
		for i, want := range faces {
			if got, _ := ReadVarInt(r); got != want+shift {
				t.Errorf("v%d face %d: item %d, want %d (remapped)", tc.version, i, got, want+shift)
			}
		}
		// The client's echo comes back canonical, faces included.
		var back []byte
		if !copyFullSlot(bytes.NewReader(out), &back, func(i int32) int32 { return i - shift }, tc.version, true) {
			t.Fatalf("v%d: serverbound copy failed", tc.version)
		}
		if !bytes.Equal(back, body) {
			t.Errorf("v%d: round trip %x, want %x", tc.version, back, body)
		}
	}
}

// instrument renumbers, and 26.x drops the EitherHolder flag: the goat horn
// a 26.3 client gets is the holder VarInt alone, and its echo comes back
// canonical with the flag restored.
func TestInstrumentFlagFollowsTheVersion(t *testing.T) {
	body := AppendVarInt(nil, 1)
	body = AppendVarInt(body, 1100) // goat_horn (any id: identity remap)
	body = AppendVarInt(body, 1)
	body = AppendVarInt(body, 0)
	body = AppendVarInt(body, componentInstrument)
	body = append(body, 1)         // EitherHolder: the holder side
	body = AppendVarInt(body, 4+1) // ponder, declared index 4
	for _, tc := range []struct {
		version int32
		want    []byte // the component on the wire
	}{
		{770, []byte{52, 1, 5}}, {774, []byte{59, 1, 5}},
		{776, []byte{61, 5}}, {777, []byte{63, 5}},
	} {
		var out []byte
		if !copyFullSlot(bytes.NewReader(body), &out, func(i int32) int32 { return i }, tc.version, false) {
			t.Fatalf("v%d: the instrument case is missing", tc.version)
		}
		if got := out[len(out)-len(tc.want):]; !bytes.Equal(got, tc.want) || len(out) != len(body)-3+len(tc.want) {
			t.Errorf("v%d: slot %x, want it to end %x", tc.version, out, tc.want)
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
