package protocol

import (
	"bytes"
	"io"
	"testing"
)

func TestRemapBlockState(t *testing.T) {
	// canonical is 1.21.11 (774); lantern 20638 -> 770 (1.21.5) 19529
	if got := RemapID(RegBlockState, 770, 20638); got != 19529 {
		t.Errorf("RemapBlockState(770, lantern 20638) = %d, want 19529", got)
	}
	// glass (562) is unchanged across the shift
	if got := RemapID(RegBlockState, 770, 562); got != 562 {
		t.Errorf("RemapBlockState(770, glass 562) = %d, want 562", got)
	}
	// 774 itself is the canonical id version — identity
	if got := RemapID(RegBlockState, 774, 20638); got != 20638 {
		t.Errorf("RemapBlockState(774, ...) should be identity, got %d", got)
	}
}

func TestRemapBlockUpdate(t *testing.T) {
	pos := make([]byte, 8)
	body := append(pos, AppendVarInt(nil, 20638)...) // lantern (1.21.11)
	out := remapBlockUpdate(770, body)
	r := bytes.NewReader(out[8:])
	got, _ := ReadVarInt(r)
	if got != 19529 {
		t.Errorf("block update state = %d, want 19529", got)
	}
}

func TestRemapChunkPalettes(t *testing.T) {
	// Build a real chunk body with two sections: one uniform lantern section
	// (single palette) and one mixed (indirect palette of glass + lantern).
	uniform := make([]uint32, 4096)
	for i := range uniform {
		uniform[i] = 20638 // lantern (1.21.11)
	}
	mixed := make([]uint32, 4096)
	for i := range mixed {
		if i%2 == 0 {
			mixed[i] = 562 // glass
		} else {
			mixed[i] = 20638 // lantern (1.21.11)
		}
	}
	var col []byte
	col = AppendSection(col, uniform, 1)
	col = AppendSection(col, mixed, 1)

	body := AppendI32(nil, 3)    // cx
	body = AppendI32(body, 4)    // cz
	body = AppendVarInt(body, 0) // heightmaps: 0 entries
	body = AppendVarInt(body, int32(len(col)))
	body = append(body, col...)
	body = AppendVarInt(body, 0)    // block entities
	body = append(body, 0xAA, 0xBB) // stand-in for light tail

	out := remapChunkBlocks(770, body)
	if bytes.Equal(out, body) {
		t.Fatal("chunk was not remapped")
	}
	// Re-read: cx/cz unchanged, then walk sections and confirm lantern -> 20638.
	r := bytes.NewReader(out)
	skip(r, 8)
	ReadVarInt(r) // heightmaps count 0
	colLen, _ := ReadVarInt(r)
	newCol := make([]byte, colLen)
	r.Read(newCol)
	// section 1: i16 count, bits u8 (0 single), palette varint == 20638
	sr := bytes.NewReader(newCol)
	skip(sr, 2)           // block count
	b, _ := sr.ReadByte() // bits
	if b != 0 {
		t.Fatalf("section 1 should be single-valued, bits=%d", b)
	}
	if v, _ := ReadVarInt(sr); v != 19529 {
		t.Errorf("uniform lantern remapped to %d, want 19529", v)
	}
	// trailing light bytes preserved
	if out[len(out)-2] != 0xAA || out[len(out)-1] != 0xBB {
		t.Error("light tail corrupted")
	}
}

func TestRemapEntityType(t *testing.T) {
	if got := RemapID(RegEntity, 770, 30); got != 28 { // cow 30 (1.21.11) -> 28 (1.21.5)
		t.Errorf("RemapEntityType(770, cow 30) = %d, want 28", got)
	}
	if got := RemapID(RegEntity, 770, 155); got != 148 { // player 155 -> 148
		t.Errorf("RemapEntityType(770, player 155) = %d, want 148", got)
	}
	if got := RemapID(RegEntity, 774, 30); got != 30 { // canonical: identity
		t.Errorf("RemapEntityType(774, ...) should be identity, got %d", got)
	}
}

func TestRemapSpawnEntityType(t *testing.T) {
	// spawn_entity: entityId, uuid(16), type, x,y,z, angles(3), objectData, vel(6)
	body := AppendVarInt(nil, 7) // entityId
	body = append(body, make([]byte, 16)...)
	body = AppendVarInt(body, 30)            // type = cow (1.21.11)
	body = append(body, make([]byte, 24)...) // x,y,z
	body = append(body, 1, 2, 3)             // angles
	body = AppendVarInt(body, 0)             // objectData
	body = append(body, 0, 0, 0, 0, 0, 0)    // velocity

	out := remapSpawnEntityType(770, body)
	r := bytes.NewReader(out)
	ReadVarInt(r) // entityId
	skip(r, 16)   // uuid
	if typ, _ := ReadVarInt(r); typ != 28 {
		t.Errorf("cow type remapped to %d, want 28", typ)
	}
	// tail intact
	if len(out) != len(body) {
		t.Errorf("len changed %d->%d (30 and 28 are both 1-byte varints)", len(body), len(out))
	}
}

func slotBytes(item int32) []byte {
	b := AppendVarInt(nil, 1) // count
	b = AppendVarInt(b, item) // item id
	b = AppendVarInt(b, 0)    // add components
	return AppendVarInt(b, 0) // remove components
}

func TestItemRoundTrip(t *testing.T) {
	// diamond 898 (1.21.11) -> 845 (1.21.5) on 770; UnmapID must invert it.
	if got := RemapID(RegItem, 770, 898); got != 845 {
		t.Errorf("RemapID(item, 770, diamond 898) = %d, want 845", got)
	}
	if got := UnmapID(RegItem, 770, 845); got != 898 {
		t.Errorf("UnmapID(item, 770, 845) = %d, want 898", got)
	}
	// Property: round-trips for a spread of item ids.
	for _, id := range []int32{1, 898, 913, 1100, 1400} {
		if UnmapID(RegItem, 770, RemapID(RegItem, 770, id)) != id {
			t.Errorf("item %d did not round-trip through 770", id)
		}
	}
}

func TestRemapSetSlotItem(t *testing.T) {
	body := AppendVarInt(nil, 0)           // window
	body = AppendVarInt(body, 1)           // stateId
	body = AppendI16(body, 3)              // slot
	body = append(body, slotBytes(898)...) // diamond (1.21.11)
	out := remapSetSlot(770, body)
	r := bytes.NewReader(out)
	ReadVarInt(r)
	ReadVarInt(r)
	skip(r, 2)
	ReadVarInt(r) // count
	if item, _ := ReadVarInt(r); item != 845 {
		t.Errorf("set_slot item = %d, want 845", item)
	}
}

func TestUnmapCreativeSlot(t *testing.T) {
	// Client (770/1.21.5) sends diamond as its id 845; we must store canonical 898.
	body := AppendI16(nil, 36)             // slot
	body = append(body, slotBytes(845)...) // diamond in 1.21.5 ids
	out := unmapCreativeSlot(770, body)
	r := bytes.NewReader(out)
	skip(r, 2)
	ReadVarInt(r) // count
	if item, _ := ReadVarInt(r); item != 898 {
		t.Errorf("creative slot unmapped to %d, want canonical 898", item)
	}
}

func TestRemapWindowItems(t *testing.T) {
	body := AppendVarInt(nil, 0)                 // window
	body = AppendVarInt(body, 1)                 // stateId
	body = AppendVarInt(body, 2)                 // 2 slots
	body = append(body, slotBytes(898)...)       // diamond (1.21.11)
	body = append(body, AppendVarInt(nil, 0)...) // empty slot (count 0)
	body = append(body, AppendVarInt(nil, 0)...) // carried: empty
	out := remapWindowItems(770, body)
	r := bytes.NewReader(out)
	ReadVarInt(r) // window
	ReadVarInt(r) // stateId
	ReadVarInt(r) // count
	ReadVarInt(r) // slot0 count
	if item, _ := ReadVarInt(r); item != 845 {
		t.Errorf("window_items slot0 item = %d, want 845", item)
	}
}

func TestRemapWindowItemsWithDamageComponent(t *testing.T) {
	// A worn tool's Slot carries the minecraft:damage component (id 3, varint).
	// Its id is identical 770→776, so the rewriter must remap the ITEM id and
	// pass the component through byte-for-byte — not bail and leave the whole
	// packet with canonical item ids (which renders as the wrong items on 26.x).
	damaged := AppendVarInt(nil, 1)              // count
	damaged = AppendVarInt(damaged, 898)         // diamond (1.21.11 canonical id, stand-in tool)
	damaged = AppendVarInt(damaged, 1)           // 1 component to add
	damaged = AppendVarInt(damaged, 0)           // 0 to remove
	damaged = AppendVarInt(damaged, 3)           // minecraft:damage
	damaged = AppendVarInt(damaged, 37)          // wear
	body := AppendVarInt(nil, 0)                 // window
	body = AppendVarInt(body, 1)                 // stateId
	body = AppendVarInt(body, 1)                 // 1 slot
	body = append(body, damaged...)              //
	body = append(body, AppendVarInt(nil, 0)...) // carried: empty

	out := remapWindowItems(770, body)
	r := bytes.NewReader(out)
	ReadVarInt(r) // window
	ReadVarInt(r) // stateId
	ReadVarInt(r) // count
	ReadVarInt(r) // slot count
	if item, _ := ReadVarInt(r); item != 845 {
		t.Fatalf("damaged slot item = %d, want remapped 845", item)
	}
	if add, _ := ReadVarInt(r); add != 1 {
		t.Fatalf("component add-count = %d, want 1", add)
	}
	ReadVarInt(r) // remove-count
	if cid, _ := ReadVarInt(r); cid != 3 {
		t.Fatalf("component id = %d, want 3 (damage)", cid)
	}
	if v, _ := ReadVarInt(r); v != 37 {
		t.Fatalf("damage value = %d, want 37", v)
	}
}

func TestRemapEntityMetaPose(t *testing.T) {
	// A sleep-pose metadata packet: eid, [idx 6, type 21 pose, SLEEPING=2],
	// [idx 14, type 11 optional_block_pos, present + 8-byte pos], 0xff.
	body := AppendVarInt(nil, 9)
	body = append(body, 6)
	body = AppendVarInt(body, 21)
	body = AppendVarInt(body, 2)
	body = append(body, 14)
	body = AppendVarInt(body, 11)
	body = append(body, 1)
	body = AppendPosition(body, 4, 70, 4)
	body = append(body, 0xff)

	// ≤772 clients share 1.21.5's serializer table — byte-identical passthrough.
	if out := remapEntityMeta(771, body); !bytes.Equal(out, body) {
		t.Fatalf("771 should pass pose metadata through unchanged\n in %v\nout %v", body, out)
	}
	// 773+ (through 26.2): pose serializer id shifted 21 → 20; all else intact.
	out := remapEntityMeta(776, body)
	r := bytes.NewReader(out)
	ReadVarInt(r) // eid
	r.ReadByte()  // index 6
	if typ, _ := ReadVarInt(r); typ != 20 {
		t.Fatalf("pose type for 776 = %d, want 20", typ)
	}
	if v, _ := ReadVarInt(r); v != 2 {
		t.Fatalf("pose value = %d, want SLEEPING=2", v)
	}
	if idx, _ := r.ReadByte(); idx != 14 {
		t.Fatalf("second entry index = %d, want 14", idx)
	}
	if typ, _ := ReadVarInt(r); typ != 11 {
		t.Fatalf("optional_block_pos type must stay 11, got %d", typ)
	}
	if out[len(out)-1] != 0xff {
		t.Fatal("terminator lost")
	}
}

func TestRemapEntityMetaItemStack(t *testing.T) {
	// The dropped-item metadata shape (index 8, type 7 Slot) must still get its
	// item id remapped by the generalized walker.
	body := AppendVarInt(nil, 5)
	body = append(body, 8)
	body = AppendVarInt(body, 7)
	body = append(body, slotBytes(898)...) // diamond, canonical (1.21.11)
	body = append(body, 0xff)
	out := remapEntityMeta(770, body)
	r := bytes.NewReader(out)
	ReadVarInt(r) // eid
	r.ReadByte()  // index
	ReadVarInt(r) // type 7
	ReadVarInt(r) // count
	if item, _ := ReadVarInt(r); item != 845 {
		t.Fatalf("item-entity stack remapped to %d, want 845", item)
	}
}

func TestRemapEquipmentItems(t *testing.T) {
	// set_equipment: eid + topBitSet-terminated (i8 slot, Slot) — a helmet in
	// slot 5 and a mainhand diamond, both needing item-id remap on 773.
	body := AppendVarInt(nil, 4)
	body = append(body, 0x80|0)            // mainhand, more follow
	body = append(body, slotBytes(898)...) // diamond (1.21.11)
	body = append(body, 5)                 // head, last entry
	body = AppendVarInt(body, 1)           // count
	body = AppendVarInt(body, 898)
	body = AppendVarInt(body, 1) // 1 component: worn helmet with damage
	body = AppendVarInt(body, 0)
	body = AppendVarInt(body, 3)  // minecraft:damage
	body = AppendVarInt(body, 12) // wear

	out := remapEquipment(770, body)
	r := bytes.NewReader(out)
	ReadVarInt(r) // eid
	r.ReadByte()  // mainhand marker
	ReadVarInt(r) // count
	if item, _ := ReadVarInt(r); item != 845 {
		t.Fatalf("mainhand item = %d, want 845", item)
	}
	ReadVarInt(r) // add comps
	ReadVarInt(r) // rem comps
	if m, _ := r.ReadByte(); m != 5 {
		t.Fatalf("second marker = %d, want 5 (no continuation)", m)
	}
	ReadVarInt(r) // count
	if item, _ := ReadVarInt(r); item != 845 {
		t.Fatalf("helmet item = %d, want 845", item)
	}
	ReadVarInt(r) // add comps = 1
	ReadVarInt(r) // rem
	if cid, _ := ReadVarInt(r); cid != 3 {
		t.Fatalf("damage component id = %d", cid)
	}
	if v, _ := ReadVarInt(r); v != 12 {
		t.Fatalf("damage value = %d", v)
	}
	if r.Len() != 0 {
		t.Fatalf("%d trailing bytes", r.Len())
	}
}

func TestChunkFluidCount775(t *testing.T) {
	// A uniform stone section through the 775 remapper must gain an i16 fluid count
	// (0) right after the block count.
	sec := make([]uint32, 4096)
	for i := range sec {
		sec[i] = 1 // stone
	}
	var col []byte
	col = AppendSection(col, sec, 1)
	body := AppendI32(nil, 0)
	body = AppendI32(body, 0)
	body = AppendVarInt(body, 0) // heightmaps
	body = AppendVarInt(body, int32(len(col)))
	body = append(body, col...)
	body = AppendVarInt(body, 0) // block entities
	body = append(body, 0xEE)    // light stand-in

	out := remapChunkBlocks(775, body)
	// new col is 2 bytes longer (one fluid-count short)
	r := bytes.NewReader(out)
	skip(r, 8)
	ReadVarInt(r)
	colLen, _ := ReadVarInt(r)
	if int(colLen) != len(col)+2 {
		t.Errorf("775 col len = %d, want %d (+2 fluid count)", colLen, len(col)+2)
	}
	nc := make([]byte, colLen)
	r.Read(nc)
	sr := bytes.NewReader(nc)
	skip(sr, 2) // block count
	if fluid, _ := readI16(sr); fluid != 0 {
		t.Errorf("fluid count = %d, want 0", fluid)
	}
}

func readI16(r *bytes.Reader) (int16, error) {
	var b [2]byte
	_, err := io.ReadFull(r, b[:])
	return int16(b[0])<<8 | int16(b[1]), err
}

func TestJoinBoolean776(t *testing.T) {
	body := []byte{1, 2, 3}
	out := remapClientboundIDs(776, canonJoinGame, body)
	if len(out) != 4 || out[3] != 0x00 {
		t.Errorf("776 join should append a false boolean, got %v", out)
	}
	// 775 client gets no extra boolean.
	if out2 := remapClientboundIDs(775, canonJoinGame, body); len(out2) != 3 {
		t.Errorf("775 join should be unchanged, got %v", out2)
	}
}

func TestLoginFinishedSessionID776(t *testing.T) {
	// 770 login_finished: uuid(16) + name + properties(0). For 776 a trailing
	// Session ID (16 bytes) must be appended.
	uuid := make([]byte, 16)
	uuid[0] = 0x7A
	body := append([]byte(nil), uuid...)
	body = AppendString(body, "tester")
	body = AppendVarInt(body, 0)

	tr := TranslatorFor(776)
	id, out, drop := tr.Clientbound(StateLogin, loginFinishedID, body)
	if drop || id != loginFinishedID {
		t.Fatalf("login_finished id=%#x drop=%v", id, drop)
	}
	if len(out) != len(body)+16 {
		t.Errorf("776 login_finished should append a 16-byte session id, got +%d", len(out)-len(body))
	}
	// trailing session id == the player's uuid
	if !bytes.Equal(out[len(out)-16:], uuid) {
		t.Error("appended session id should equal the player uuid")
	}
	// A 775 client must NOT get the extra field.
	if _, out775, _ := TranslatorFor(775).Clientbound(StateLogin, loginFinishedID, body); len(out775) != len(body) {
		t.Errorf("775 login_finished should be unchanged, got +%d", len(out775)-len(body))
	}
}

// TestEnchantmentsComponentRenumbered: the minecraft:enchantments component id
// is 10 through 1.21.10 but 13 from 1.21.11 (774) — the slot walker must
// renumber it (clientbound 10→13, serverbound back), keeping enchantment ids
// (our declared registry order) untouched.
func TestEnchantmentsComponentRenumbered(t *testing.T) {
	slot := AppendVarInt(nil, 1)    // count
	slot = AppendVarInt(slot, 878)  // diamond sword (canonical id)
	slot = AppendVarInt(slot, 1)    // components to add
	slot = AppendVarInt(slot, 0)    // components to remove
	slot = AppendVarInt(slot, 10)   // minecraft:enchantments (canonical)
	slot = AppendVarInt(slot, 1)    // one enchantment
	slot = AppendVarInt(slot, 32)   // sharpness (declared order)
	slot = AppendVarInt(slot, 3)    // level 3
	body := AppendVarInt(nil, 1)    // window
	body = AppendVarInt(body, 7)    // stateId
	body = append(body, 0x00, 0x05) // slot i16
	body = append(body, slot...)

	out := remapSetSlot(776, body)
	r := bytes.NewReader(out)
	ReadVarInt(r) // window
	ReadVarInt(r) // stateId
	var s2 [2]byte
	io.ReadFull(r, s2[:])    // slot
	ReadVarInt(r)            // count
	item, _ := ReadVarInt(r) // item id (remapped for 776)
	if want := RemapID(RegItem, 776, 878); item != want {
		t.Fatalf("item id = %d, want %d", item, want)
	}
	ReadVarInt(r) // addC
	ReadVarInt(r) // remC
	cid, _ := ReadVarInt(r)
	if cid != 13 {
		t.Fatalf("enchantments component id for 776 = %d, want 13", cid)
	}
	n, _ := ReadVarInt(r)
	id, _ := ReadVarInt(r)
	lvl, _ := ReadVarInt(r)
	if n != 1 || id != 32 || lvl != 3 {
		t.Fatalf("enchantment payload changed: n=%d id=%d lvl=%d", n, id, lvl)
	}

	// Serverbound (creative slot from a 776 client): 13 comes back as 10.
	cs := append([]byte{0x00, 0x09}, slot...) // i16 slot + Slot…
	// …but with the CLIENT's ids: component 13 and the client item id.
	cs = nil
	cs = append(cs, 0x00, 0x09)
	cs = AppendVarInt(cs, 1)
	cs = AppendVarInt(cs, RemapID(RegItem, 776, 878))
	cs = AppendVarInt(cs, 1)
	cs = AppendVarInt(cs, 0)
	cs = AppendVarInt(cs, 13)
	cs = AppendVarInt(cs, 1)
	cs = AppendVarInt(cs, 32)
	cs = AppendVarInt(cs, 3)
	back := unmapCreativeSlot(776, cs)
	r = bytes.NewReader(back)
	io.ReadFull(r, s2[:])
	ReadVarInt(r)           // count
	item, _ = ReadVarInt(r) // canonical item id
	if item != 878 {
		t.Fatalf("serverbound item = %d, want canonical 878", item)
	}
	ReadVarInt(r)
	ReadVarInt(r)
	cid, _ = ReadVarInt(r)
	if cid != 10 {
		t.Fatalf("serverbound enchantments component = %d, want canonical 10", cid)
	}
}

func TestWorldEventBlockBreakRemap(t *testing.T) {
	b := AppendI32(nil, 2001)
	b = AppendPosition(b, 1, 64, 1)
	b = AppendI32(b, 10) // canonical block state
	b = AppendBool(b, false)
	out := remapWorldEvent(776, b)
	got := int32(uint32(out[12])<<24 | uint32(out[13])<<16 | uint32(out[14])<<8 | uint32(out[15]))
	if want := RemapID(RegBlockState, 776, 10); got != want {
		t.Fatalf("2001 data = %d, want %d", got, want)
	}
	// A non-2001 event passes through untouched.
	b2 := append([]byte(nil), b...)
	b2[3] = 0xD2 // 2002
	if !bytes.Equal(remapWorldEvent(776, b2), b2) {
		t.Fatal("non-break events must pass through")
	}
}

func TestWorldParticlesRemap(t *testing.T) {
	prefix := make([]byte, 46)
	body := AppendVarInt(append([]byte(nil), prefix...), 21) // explosion_emitter
	out := remapWorldParticles(776, body)
	r := bytes.NewReader(out[46:])
	if id, _ := ReadVarInt(r); id != 29 {
		t.Fatalf("explosion_emitter @776 = %d, want 29", id)
	}
	if id, _ := ReadVarInt(bytes.NewReader(remapWorldParticles(773, body)[46:])); id != 22 {
		t.Fatalf("explosion_emitter @773 should be 22, got %d", id)
	}
	// A particle with a payload after the id must be left untouched.
	withPayload := append(append([]byte(nil), body...), 0x05)
	if !bytes.Equal(remapWorldParticles(776, withPayload), withPayload) {
		t.Fatal("payload particles must pass through untouched")
	}
}

// TestChunkFluidCount26x: the 26.1+ per-section fluid count must be the REAL
// number of fluid blocks — the client builds its fluid layer (water rendering
// + swim physics) from it. Writing 0 made generated oceans invisible and
// unswimmable on 26.x (live bug).
func TestChunkFluidCount26x(t *testing.T) {
	// Section A: all water (single-valued palette). Section B: stone with two
	// water blocks and one lava (indirect palette).
	var states [4096]uint32
	for i := range states {
		states[i] = 86 // water source
	}
	col := AppendSection(nil, states[:], 0)
	for i := range states {
		states[i] = 1 // stone
	}
	states[7] = 86    // water
	states[99] = 101  // flowing water
	states[512] = 110 // lava
	col = AppendSection(col, states[:], 0)

	body := AppendI32(nil, 0)    // cx
	body = AppendI32(body, 0)    // cz
	body = AppendVarInt(body, 0) // no heightmaps
	body = AppendVarInt(body, int32(len(col)))
	body = append(body, col...)

	out := remapChunkBlocks(776, body)
	r := bytes.NewReader(out)
	skip(r, 8)
	ReadVarInt(r) // heightmap count
	ReadVarInt(r) // col len
	readSection := func() (blockCount, fluidCount int) {
		var v [4]byte
		io.ReadFull(r, v[:]) // blockCount i16 + fluidCount i16
		blockCount = int(v[0])<<8 | int(v[1])
		fluidCount = int(v[2])<<8 | int(v[3])
		// skip the two containers: block states + biomes
		for c := 0; c < 2; c++ {
			n := 4096
			if c == 1 {
				n = 64
			}
			var sink []byte
			if !processContainer(r, &sink, n, nil) {
				t.Fatal("container parse failed")
			}
		}
		return
	}
	b1, f1 := readSection()
	if b1 != 4096 || f1 != 4096 {
		t.Fatalf("all-water section: blocks=%d fluids=%d, want 4096/4096", b1, f1)
	}
	b2, f2 := readSection()
	if b2 != 4096 || f2 != 3 {
		t.Fatalf("mixed section: blocks=%d fluids=%d, want 4096/3", b2, f2)
	}
	// Pre-26.x clients must NOT get the field: same body at 773 keeps sections
	// at blockCount + containers only.
	out773 := remapChunkBlocks(773, body)
	if len(out773) >= len(out) {
		t.Fatal("773 chunks must not carry fluid counts")
	}
}

// TestStoredEnchAndNameRenumbered: stored_enchantments shifts twice across the
// chain (34 → 41 @774 → 42 @776) and custom_name once (5 → 6 @774); both must
// renumber per version — 774/775 and 776 genuinely differ for stored.
func TestStoredEnchAndNameRenumbered(t *testing.T) {
	slot := AppendVarInt(nil, 1)   // count
	slot = AppendVarInt(slot, 967) // book (canonical id)
	slot = AppendVarInt(slot, 2)   // two components
	slot = AppendVarInt(slot, 0)
	slot = AppendVarInt(slot, 34) // stored_enchantments
	slot = AppendVarInt(slot, 1)
	slot = AppendVarInt(slot, 13) // fortune (declared order)
	slot = AppendVarInt(slot, 2)
	slot = AppendVarInt(slot, 5) // custom_name: TAG_String "Hi"
	slot = append(slot, 0x08, 0x00, 0x02, 'H', 'i')
	body := AppendVarInt(nil, 1)
	body = AppendVarInt(body, 7)
	body = append(body, 0x00, 0x05)
	body = append(body, slot...)

	check := func(version, wantStored, wantName int32) {
		out := remapSetSlot(version, body)
		r := bytes.NewReader(out)
		ReadVarInt(r)
		ReadVarInt(r)
		var s2 [2]byte
		io.ReadFull(r, s2[:])
		ReadVarInt(r)           // count
		ReadVarInt(r)           // item (remapped)
		ReadVarInt(r)           // addC
		ReadVarInt(r)           // remC
		cid, _ := ReadVarInt(r) // stored component id
		if cid != wantStored {
			t.Fatalf("v%d stored_enchantments = %d, want %d", version, cid, wantStored)
		}
		ReadVarInt(r) // n
		ReadVarInt(r) // ench id
		ReadVarInt(r) // lvl
		nid, _ := ReadVarInt(r)
		if nid != wantName {
			t.Fatalf("v%d custom_name = %d, want %d", version, nid, wantName)
		}
	}
	check(773, 34, 5)
	check(774, 41, 6)
	check(775, 41, 6)
	check(776, 42, 6)
}
