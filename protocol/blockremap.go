package protocol

import (
	"bytes"
	"io"
	"math"
)

// Block-state ID translation for chunk and block-update packets. We send the world
// in canonical 770 block-state IDs; a newer client decodes them against its own
// registry, so without remapping its blocks render wrong (shifted IDs). These
// rewriters translate the IDs in the two clientbound packets that carry them —
// chunk_data (palettes) and block_update (one ID) — using the generated per-version
// blockRemaps table. Applied by the chain at canonical IDs (see translate_chain.go).

// Canonical (770) play packet IDs that carry version-specific registry IDs.
const (
	canonChunkData          = 0x27 // clientbound Chunk Data (block-state IDs)
	canonBlockUpdate        = 0x08 // clientbound Block Update (one block-state ID)
	canonSpawnEntity        = 0x01 // clientbound Spawn Entity (entity-type ID)
	canonSetSlot            = 0x14 // clientbound Set Slot (item ID in a Slot)
	canonWindowItems        = 0x12 // clientbound Set Container Content (many Slots)
	canonEntityMetadata     = 0x5c // clientbound Set Entity Metadata (item-entity Slot)
	canonSetCreativeSlot    = 0x36 // serverbound Set Creative Mode Slot (item ID)
	canonWindowClick        = 0x10 // serverbound Click Container (HashedSlot item IDs)
	canonSetCursorItem      = 0x59 // clientbound Set Cursor Item (one Slot)
	canonEntityVelocity     = 0x5e // clientbound Set Entity Velocity/Motion
	canonSetEquipment       = 0x5f // clientbound Set Equipment (worn armor/held item)
	canonJoinGame           = 0x2b // clientbound Login / Join Game
	canonUpdateTime         = 0x6a // clientbound Update Time / set_time
	canonWorldEvent         = 0x28 // clientbound World Event (2001 carries a block-state ID)
	canonWorldParticles     = 0x29 // clientbound Level Particles (particle-type ID)
	canonUpdateAdvancements = 0x7b // clientbound Update Advancements (icon Slots)

	metaIndexItemStack = 8 // entity-metadata index of an item entity's stack
	metaTypeSlot       = 7 // entity-metadata value type: Slot
)

// remapClientboundIDs rewrites version-specific registry IDs in a clientbound play
// packet, dispatching by canonical (770) packet ID. Returns body unchanged when no
// registry applies for this client version.
func remapClientboundIDs(version, id int32, body []byte) []byte {
	switch id {
	case canonChunkData:
		if HasRemap(RegBlockState, version) {
			return remapChunkBlocks(version, body)
		}
	case canonBlockUpdate:
		if HasRemap(RegBlockState, version) {
			return remapBlockUpdate(version, body)
		}
	case canonSpawnEntity:
		if HasRemap(RegEntity, version) {
			return remapSpawnEntityType(version, body)
		}
	case canonSetSlot:
		if HasRemap(RegItem, version) {
			return remapSetSlot(version, body)
		}
	case canonWindowItems:
		if HasRemap(RegItem, version) {
			return remapWindowItems(version, body)
		}
	case canonEntityMetadata:
		if HasRemap(RegItem, version) { // item ids inside item-entity metadata (incl. 770 now)
			return remapEntityMeta(version, body)
		}
	case canonSetCursorItem:
		if HasRemap(RegItem, version) {
			r := bytes.NewReader(body) // body is a single Slot
			return remapTrailingSlot(body, r, func(i int32) int32 { return RemapID(RegItem, version, i) },
				version, false)
		}
	case canonSetEquipment:
		if HasRemap(RegItem, version) {
			return remapEquipment(version, body)
		}
	case canonEntityVelocity:
		// 1.21.9 (773) changed the velocity encoding from vec3i16 (3×i16 in
		// 1/8000 block/tick) to the low-precision vector ("lpVec3"). Sending the
		// old form disconnects the client ("failed to decode packet
		// clientbound/minecraft:set_entity_motion") — hit the first time we ever
		// sent this packet (zombie-bite knockback).
		if version >= 773 {
			return rewriteEntityVelocity773(body)
		}
	case canonJoinGame:
		// 26.2 (proto 776) appended a trailing boolean to Join (online/secure-chat
		// flag); we are offline, so write false. The field is appended at the end,
		// so no parsing is needed.
		if version >= 776 {
			return append(append([]byte(nil), body...), 0x00)
		}
	case canonUpdateTime:
		// 26.1 (proto 775+) reworked Update Time into the clock-based form.
		if version >= 775 {
			return rewriteSetTime26x(body)
		}
	case canonWorldEvent:
		if HasRemap(RegBlockState, version) {
			return remapWorldEvent(version, body)
		}
	case canonWorldParticles:
		if version > 770 {
			return remapWorldParticles(version, body)
		}
	case canonUpdateAdvancements:
		if HasRemap(RegItem, version) {
			return remapAdvancementIcons(version, body)
		}
	case canonAwardStats:
		// Four registries ride this packet; RemapID self-no-ops per registry
		// when a version needs no shift, so no HasRemap gate here.
		return remapAwardStats(version, body)
	}
	return body
}

// remapWorldEvent rewrites the data field of a World Event when the event is
// block-break (2001) — its data is a block-state ID. Layout: i32 event,
// position (8 bytes), i32 data, bool global.
func remapWorldEvent(version int32, body []byte) []byte {
	if len(body) < 17 {
		return body
	}
	event := int32(uint32(body[0])<<24 | uint32(body[1])<<16 | uint32(body[2])<<8 | uint32(body[3]))
	if event != 2001 {
		return body
	}
	state := int32(uint32(body[12])<<24 | uint32(body[13])<<16 | uint32(body[14])<<8 | uint32(body[15]))
	ns := RemapID(RegBlockState, version, state)
	if ns == state {
		return body
	}
	out := append([]byte(nil), body...)
	out[12], out[13], out[14], out[15] = byte(ns>>24), byte(ns>>16), byte(ns>>8), byte(ns)
	return out
}

// remapWorldParticles rewrites the TRAILING particle-type id of a Level
// Particles packet, for the handful of no-payload particles we emit (their
// per-version ids come from ViaVersion's mappings). A particle we don't know
// may carry a payload after the id, so the body is left untouched then.
// Layout: bool, bool, 3×f64, 4×f32, i32 count, VarInt particleId [payload].
func remapWorldParticles(version int32, body []byte) []byte {
	const prefix = 2 + 24 + 16 + 4
	r := bytes.NewReader(body)
	if !skip(r, prefix) {
		return body
	}
	pid, err := ReadVarInt(r)
	if err != nil || r.Len() != 0 {
		return body // payload follows — not one of ours, don't guess
	}
	np := remapParticleID(version, pid)
	if np == pid {
		return body
	}
	out := append([]byte(nil), body[:prefix]...)
	return AppendVarInt(out, np)
}

// remapParticleID maps the canonical (770) ids of the payload-free particles
// we emit to the client version's ids (verified against ViaVersion mappings:
// 1.21.9 and 26.1 inserted particles ahead of these; 26.2 inserted more).
func remapParticleID(version, id int32) int32 {
	switch id {
	case 21: // minecraft:explosion_emitter
		switch {
		case version >= 776:
			return 29
		case version >= 773:
			return 22
		}
	case 22: // minecraft:explosion
		switch {
		case version >= 776:
			return 30
		case version >= 773:
			return 23
		}
	case 56: // minecraft:poof
		switch {
		case version >= 776:
			return 66
		case version >= 775:
			return 59
		case version >= 773:
			return 57
		}
	case 5: // minecraft:crit
		switch {
		case version >= 776:
			return 13
		case version >= 773:
			return 6
		}
	}
	return id
}

// rewriteSetTime26x converts our Update Time (i64 gameTime, i64 dayTime, bool
// tickDayTime) into the 26.1 clock form: gameTime, then for one clock — VarInt
// count(1), VarInt clock id(0 = overworld), VarLong total ticks, Float partial
// tick(0), Float tick rate (1 if day advances, else 0).
func rewriteSetTime26x(body []byte) []byte {
	r := bytes.NewReader(body)
	var gameTime, dayBytes [8]byte
	if _, err := io.ReadFull(r, gameTime[:]); err != nil {
		return body
	}
	if _, err := io.ReadFull(r, dayBytes[:]); err != nil {
		return body
	}
	tick, err := r.ReadByte()
	if err != nil {
		return body
	}
	dayTime := int64(uint64(dayBytes[0])<<56 | uint64(dayBytes[1])<<48 | uint64(dayBytes[2])<<40 |
		uint64(dayBytes[3])<<32 | uint64(dayBytes[4])<<24 | uint64(dayBytes[5])<<16 |
		uint64(dayBytes[6])<<8 | uint64(dayBytes[7]))
	out := append([]byte(nil), gameTime[:]...) // Long game time (unchanged)
	out = AppendVarInt(out, 1)                 // one clock
	out = AppendVarInt(out, 0)                 // overworld clock id
	out = AppendVarLong(out, dayTime)          // total ticks
	out = AppendF32(out, 0)                    // partial tick
	rate := float32(0)
	if tick != 0 {
		rate = 1
	}
	return AppendF32(out, rate) // tick rate
}

// unmapServerboundIDs rewrites client-version IDs back to canonical in a serverbound
// play packet. The packet ID here is already canonical (the chain renumbered it).
func unmapServerboundIDs(version, id int32, body []byte) []byte {
	if id == canonSetCreativeSlot && HasRemap(RegItem, version) {
		return unmapCreativeSlot(version, body)
	}
	if id == canonWindowClick && HasRemap(RegItem, version) {
		return unmapWindowClick(version, body)
	}
	return body
}

// unmapWindowClick (serverbound): the click's changed-slots and cursor carry
// HashedSlots with the CLIENT version's item ids — translate each back to
// canonical. VarInt widths can change, so the body is rebuilt field by field;
// any parse trouble returns the body unchanged (the server's own parser will
// reject it rather than misread it).
func unmapWindowClick(version int32, body []byte) []byte {
	r := bytes.NewReader(body)
	out := make([]byte, 0, len(body)+8)
	ok := copyVarInt(r, &out) && copyVarInt(r, &out) && // windowId, stateId
		copyBytes(r, &out, 3) && // slot i16 + button i8
		copyVarInt(r, &out) // mode
	if !ok {
		return body
	}
	n, err := ReadVarInt(r)
	if err != nil || n < 0 || n > 128 {
		return body
	}
	out = AppendVarInt(out, n)
	for i := 0; i < int(n); i++ {
		if !copyBytes(r, &out, 2) || !unmapHashedSlot(version, r, &out) { // location i16 + slot
			return body
		}
	}
	if !unmapHashedSlot(version, r, &out) { // cursor
		return body
	}
	return append(out, body[len(body)-r.Len():]...) // any trailing bytes verbatim
}

// unmapHashedSlot copies one Option<HashedSlot>, translating its item id.
func unmapHashedSlot(version int32, r *bytes.Reader, out *[]byte) bool {
	has, err := r.ReadByte()
	if err != nil {
		return false
	}
	*out = append(*out, has)
	if has == 0 {
		return true
	}
	item, err := ReadVarInt(r)
	if err != nil {
		return false
	}
	*out = AppendVarInt(*out, UnmapID(RegItem, version, item))
	if !copyVarInt(r, out) { // count
		return false
	}
	nAdd, err := ReadVarInt(r)
	if err != nil || nAdd < 0 || nAdd > 64 {
		return false
	}
	*out = AppendVarInt(*out, nAdd)
	for i := 0; i < int(nAdd); i++ {
		if !copyVarInt(r, out) || !copyBytes(r, out, 4) { // component type + i32 hash
			return false
		}
	}
	nRm, err := ReadVarInt(r)
	if err != nil || nRm < 0 || nRm > 64 {
		return false
	}
	*out = AppendVarInt(*out, nRm)
	for i := 0; i < int(nRm); i++ {
		if !copyVarInt(r, out) {
			return false
		}
	}
	return true
}

func copyVarInt(r *bytes.Reader, out *[]byte) bool {
	v, err := ReadVarInt(r)
	if err != nil {
		return false
	}
	*out = AppendVarInt(*out, v)
	return true
}

func copyBytes(r *bytes.Reader, out *[]byte, n int) bool {
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return false
	}
	*out = append(*out, buf...)
	return true
}

// remapSpawnEntityType rewrites the entity-type ID in a Spawn Entity packet to the
// client version. Layout: entityId(VarInt) objectUUID(16) type(VarInt) … — only
// the type field is touched; the rest (which a later step may reorder for velocity)
// is copied through. Applied at the canonical layout, before any per-step rewrite.
func remapSpawnEntityType(version int32, body []byte) []byte {
	r := bytes.NewReader(body)
	if _, err := ReadVarInt(r); err != nil { // entityId
		return body
	}
	if !skip(r, 16) { // objectUUID
		return body
	}
	typeAt := len(body) - r.Len()
	typ, err := ReadVarInt(r)
	if err != nil {
		return body
	}
	// Substitute entities the client's version predates BEFORE the range-shift,
	// so a new mob renders as a real older stand-in instead of a wrong id (see
	// entity_substitute.go). The fallback is canonical and range-maps normally.
	sub := substituteEntityType(version, typ)
	nt := RemapID(RegEntity, version, sub)
	if sub == typ && nt == typ {
		return body // nothing to change
	}
	afterType := len(body) - r.Len()
	out := make([]byte, 0, len(body)+2)
	out = append(out, body[:typeAt]...)    // entityId + objectUUID
	out = AppendVarInt(out, nt)            // remapped type
	out = append(out, body[afterType:]...) // x,y,z, angles, objectData, velocity
	return out
}

// remapBlockUpdate rewrites the single block-state ID in a Block Update packet
// (8-byte packed position followed by a VarInt state).
func remapBlockUpdate(version int32, body []byte) []byte {
	if len(body) < 8 {
		return body
	}
	r := bytes.NewReader(body[8:])
	state, err := ReadVarInt(r)
	if err != nil {
		return body
	}
	ns := RemapID(RegBlockState, version, state)
	if ns == state {
		return body
	}
	out := append([]byte(nil), body[:8]...)
	return AppendVarInt(out, ns)
}

// remapChunkBlocks rewrites every block-state ID in a Chunk Data packet's section
// palettes to the client version. It rebuilds only the section-data ("col") region;
// the heightmap, block entities and light arrays are copied verbatim. On any parse
// surprise it returns the body unchanged (fail safe).
//
// map_chunk layout: cx(i32) cz(i32) heightmaps varint(colLen) col[colLen]
// block_entities light…
func remapChunkBlocks(version int32, body []byte) []byte {
	r := bytes.NewReader(body)
	if !skip(r, 8) { // cx, cz
		return body
	}
	// heightmaps: VarInt count, then per entry: VarInt type, VarInt nLongs, longs.
	hmCount, err := ReadVarInt(r)
	if err != nil {
		return body
	}
	for i := int32(0); i < hmCount; i++ {
		if _, err := ReadVarInt(r); err != nil { // type
			return body
		}
		nLongs, err := ReadVarInt(r)
		if err != nil || !skip(r, int(nLongs)*8) {
			return body
		}
	}
	colLenAt := len(body) - r.Len() // offset of the colLen VarInt
	colLen, err := ReadVarInt(r)
	if err != nil {
		return body
	}
	colAt := len(body) - r.Len()
	if colLen < 0 || colAt+int(colLen) > len(body) {
		return body
	}
	col := body[colAt : colAt+int(colLen)]
	newCol := remapSections(version, col)
	if newCol == nil {
		return body // parse failure — leave the chunk untouched
	}
	out := make([]byte, 0, len(body)+len(newCol)-len(col))
	out = append(out, body[:colLenAt]...)       // cx, cz, heightmaps
	out = AppendVarInt(out, int32(len(newCol))) // (re)write col length
	out = append(out, newCol...)
	out = append(out, body[colAt+int(colLen):]...) // block entities + light
	return out
}

// remapSections walks the back-to-back chunk sections in col, remapping each
// section's block palette and copying its biome container. Returns nil on a parse
// error. Sections run until col is exhausted (we don't need the section count).
func remapSections(version int32, col []byte) []byte {
	r := bytes.NewReader(col)
	out := make([]byte, 0, len(col))
	for r.Len() > 0 {
		var cnt [2]byte // non-air block count (i16)
		if _, err := io.ReadFull(r, cnt[:]); err != nil {
			return nil
		}
		out = append(out, cnt[:]...)
		// 26.1+ (proto 775+) added a per-section fluid count (i16) right after
		// the non-air block count — and it is LOAD-BEARING: the 26.x client
		// builds its fluid layer from it, which drives water RENDERING and SWIM
		// PHYSICS (writing 0 here made generated oceans invisible and players
		// sink like stones — the live 26.2 bug). ViaVersion computes it the
		// same way: count the section's fluid-state blocks (verified against
		// BlockItemPacketRewriter26_1, facts only).
		var fluids int
		fluidPtr := (*int)(nil)
		if version >= 775 {
			fluidPtr = &fluids
		}
		blockAt := len(out) // fluid count is written between cnt and the container
		// Block states (4096 entries) — remap palette IDs (+ count fluids).
		if !processContainerFluids(r, &out, 4096, func(s uint32) uint32 { return uint32(RemapID(RegBlockState, version, int32(s))) }, fluidPtr) {
			return nil
		}
		if version >= 775 {
			var fc [2]byte
			fc[0], fc[1] = byte(fluids>>8), byte(fluids)
			out = append(out[:blockAt], append(fc[:], out[blockAt:]...)...)
		}
		// Biomes (64 entries) — copy as-is (biome IDs not yet translated).
		if !processContainer(r, &out, 64, nil) {
			return nil
		}
	}
	return out
}

// processContainer reads one paletted container from r and writes it to *out,
// applying remap to palette IDs when non-nil. entryCount is 4096 for block
// containers, 64 for biome containers. Our encoder only emits single-valued (0
// bits) and indirect (palette + longs) containers, never direct, so a non-zero
// bits value always carries a palette.
// isFluidState reports whether a CANONICAL (770) block state is a fluid —
// water 86-101 or lava 102-117. Used to compute the 26.x per-section fluid
// count, which the client's fluid layer (rendering + swim physics) requires.
func isFluidState(v uint32) bool { return v >= 86 && v <= 117 }

func processContainer(r *bytes.Reader, out *[]byte, entryCount int, remap func(uint32) uint32) bool {
	return processContainerFluids(r, out, entryCount, remap, nil)
}

// processContainerFluids is processContainer with an optional fluid tally:
// when fluids is non-nil, it counts how many of the container's entries are
// canonical fluid states (evaluated BEFORE remapping — the tally is over the
// server's own ids, mirroring how ViaVersion derives the 26.1 fluid count).
func processContainerFluids(r *bytes.Reader, out *[]byte, entryCount int, remap func(uint32) uint32, fluids *int) bool {
	bits, err := r.ReadByte()
	if err != nil {
		return false
	}
	*out = append(*out, bits)
	if bits == 0 { // single-valued: one VarInt
		v, err := ReadVarInt(r)
		if err != nil {
			return false
		}
		if fluids != nil && isFluidState(uint32(v)) {
			*fluids += entryCount // the whole section is this one fluid
		}
		if remap != nil {
			v = int32(remap(uint32(v)))
		}
		*out = AppendVarInt(*out, v)
		return true
	}
	// Indirect: palette length, palette entries, then the compacted long array.
	palLen, err := ReadVarInt(r)
	if err != nil {
		return false
	}
	*out = AppendVarInt(*out, palLen)
	fluidEntry := make([]bool, palLen)
	for i := int32(0); i < palLen; i++ {
		v, err := ReadVarInt(r)
		if err != nil {
			return false
		}
		if fluids != nil && isFluidState(uint32(v)) {
			fluidEntry[i] = true
		}
		if remap != nil {
			v = int32(remap(uint32(v)))
		}
		*out = AppendVarInt(*out, v)
	}
	per := 64 / int(bits)
	nLongs := (entryCount + per - 1) / per
	buf := make([]byte, nLongs*8)
	if _, err := io.ReadFull(r, buf); err != nil {
		return false
	}
	if fluids != nil {
		// Decode the packed indices (big-endian longs, no straddling) and tally
		// entries that point at fluid palette slots.
		mask := uint64(1)<<bits - 1
		counted := 0
		for li := 0; li < nLongs && counted < entryCount; li++ {
			long := uint64(buf[li*8])<<56 | uint64(buf[li*8+1])<<48 | uint64(buf[li*8+2])<<40 |
				uint64(buf[li*8+3])<<32 | uint64(buf[li*8+4])<<24 | uint64(buf[li*8+5])<<16 |
				uint64(buf[li*8+6])<<8 | uint64(buf[li*8+7])
			for k := 0; k < per && counted < entryCount; k++ {
				idx := long >> (uint(k) * uint(bits)) & mask
				if int(idx) < len(fluidEntry) && fluidEntry[idx] {
					*fluids++
				}
				counted++
			}
		}
	}
	*out = append(*out, buf...)
	return true
}

// skip advances r by n bytes, reporting success.
func skip(r *bytes.Reader, n int) bool {
	if n < 0 || n > r.Len() {
		return false
	}
	_, err := r.Seek(int64(n), io.SeekCurrent)
	return err == nil
}

// Item-ID translation in Slots. A Slot is: VarInt count; if count>0: VarInt itemId
// then component data. We only ever rewrite the itemId; component bytes are copied
// verbatim, so we never need to parse them.

// remapTrailingSlot rewrites the Slot beginning at r's position (the remainder
// of body), copying the packet prefix before it. For packets where the Slot is
// the last field (or followed by a fixed tail like the metadata terminator).
// remap is RemapID for clientbound, UnmapID for serverbound; enchIn/enchOut
// are copyFullSlot's enchantments-component translation (in the serverbound
// direction the caller passes them swapped).
//
// Whitelisted slots are fully re-encoded (component ids translated); a slot
// carrying components we don't know falls back to the old behaviour — remap
// just the item id and copy the tail verbatim (imperfect for those exotic
// slots, but never corrupting).
func remapTrailingSlot(body []byte, r *bytes.Reader, remap func(int32) int32, version int32, serverbound bool) []byte {
	slotAt := len(body) - r.Len()
	prefixOut := append([]byte(nil), body[:slotAt]...)
	full := prefixOut
	if copyFullSlot(r, &full, remap, version, serverbound) {
		return full
	}
	r.Reset(body[slotAt:])
	count, err := ReadVarInt(r)
	if err != nil {
		return body
	}
	if count <= 0 {
		return body // empty slot — no item ID
	}
	item, err := ReadVarInt(r)
	if err != nil {
		return body
	}
	ni := remap(item)
	if ni == item {
		return body
	}
	afterItem := len(body[slotAt:]) - r.Len() + slotAt
	out := make([]byte, 0, len(body)+2)
	out = append(out, body[:slotAt]...)    // packet prefix
	out = AppendVarInt(out, count)         // count
	out = AppendVarInt(out, ni)            // remapped item ID
	out = append(out, body[afterItem:]...) // components + any tail
	return out
}

// remapSetSlot: VarInt window, VarInt stateId, i16 slot, Slot.
func remapSetSlot(version int32, body []byte) []byte {
	r := bytes.NewReader(body)
	if _, err := ReadVarInt(r); err != nil { // window
		return body
	}
	if _, err := ReadVarInt(r); err != nil { // stateId
		return body
	}
	if !skip(r, 2) { // slot (i16)
		return body
	}
	return remapTrailingSlot(body, r, func(i int32) int32 { return RemapID(RegItem, version, i) },
		version, false)
}

// Entity-data serializer type ids WE emit (canonical 770 numbering). Verified
// against ViaVersion's EntityDataTypes classes: everything below pose is
// identical from 1.21.5 through 26.2; pose alone shifted 21→20 at 1.21.9 (773)
// and stays 20 through 26.2.
const (
	metaTypeByte        = 0
	metaTypeVarInt      = 1
	metaTypeFloat       = 3
	metaTypeBoolean     = 8
	metaTypeBlockPos    = 10
	metaTypeOptBlockPos = 11
	metaTypePose        = 21 // → 20 for clients ≥773
)

// remapEntityMeta rewrites set_entity_data for a translated client: item ids
// inside Slot entries are remapped, and the pose serializer TYPE id shifts
// down by one at 773+. Only the entry shapes our encoders emit are understood
// (item stack, pose, sleeping position, and the simple scalar types) — an
// unknown type bails to the untouched body, so keep this switch in sync when
// a new emitter appears.
func remapEntityMeta(version int32, body []byte) []byte {
	r := bytes.NewReader(body)
	eid, err := ReadVarInt(r)
	if err != nil {
		return body
	}
	out := AppendVarInt(nil, eid)
	remapItem := func(i int32) int32 { return RemapID(RegItem, version, i) }
	for {
		idx, err := r.ReadByte()
		if err != nil {
			return body
		}
		if idx == 0xff { // terminator
			out = append(out, idx)
			return out
		}
		typ, err := ReadVarInt(r)
		if err != nil {
			return body
		}
		// NOTE: the 26.2 cube-mob index shift (slime/magma SIZE 16→18) is NOT
		// applied here. set_entity_data carries no entity type, and a creeper's
		// SWELL_DIR is *also* an index-16 VarInt — shifting it blindly moved it
		// onto 26.2's Boolean IS_POWERED and disconnected every client that
		// approached a creeper. The shift is entity-type-specific, so it lives in
		// ShiftCubeMobMeta, which the gateway calls only for slimes/magma cubes
		// (it tracks eid→type from spawn packets).
		out = append(out, idx)
		wireType := typ
		if typ == metaTypePose && version >= 773 {
			wireType = metaTypePose - 1
		}
		out = AppendVarInt(out, wireType)
		switch typ {
		case metaTypeByte, metaTypeBoolean:
			b, err := r.ReadByte()
			if err != nil {
				return body
			}
			out = append(out, b)
		case metaTypeVarInt, metaTypePose:
			v, err := ReadVarInt(r)
			if err != nil {
				return body
			}
			out = AppendVarInt(out, v)
		case metaTypeFloat:
			var f [4]byte
			if _, err := io.ReadFull(r, f[:]); err != nil {
				return body
			}
			out = append(out, f[:]...)
		case metaTypeSlot:
			if !copyFullSlot(r, &out, remapItem, version, false) {
				return body
			}
		case metaTypeBlockPos:
			var p [8]byte
			if _, err := io.ReadFull(r, p[:]); err != nil {
				return body
			}
			out = append(out, p[:]...)
		case metaTypeOptBlockPos:
			present, err := r.ReadByte()
			if err != nil {
				return body
			}
			out = append(out, present)
			if present != 0 {
				var p [8]byte
				if _, err := io.ReadFull(r, p[:]); err != nil {
					return body
				}
				out = append(out, p[:]...)
			}
		default:
			return body // a type we never emit — don't guess at its payload
		}
	}
}

// ShiftCubeMobMeta moves a slime/magma-cube SIZE field (canonical index 16,
// VarInt) up to index 18 for clients ≥776, where 26.2 inserted two
// ABSTRACT_CUBE_MOB fields at 16–17 (ViaVersion: addIndex 16+17 @26_2). It is
// entity-type-specific and MUST be called only for slimes and magma cubes:
// set_entity_data has no type field, and a creeper's SWELL_DIR is also an
// index-16 VarInt that must stay at 16. All bytes but the shifted index are
// copied verbatim — item stacks and pose (which the auto-path remaps) are left
// untouched, so this composes with remapEntityMeta without double-remapping.
// Cube-mob metadata never carries a Slot, so an unexpected type bails safely.
func ShiftCubeMobMeta(version int32, body []byte) []byte {
	if version < 776 {
		return body
	}
	r := bytes.NewReader(body)
	eid, err := ReadVarInt(r)
	if err != nil {
		return body
	}
	out := AppendVarInt(nil, eid)
	for {
		idx, err := r.ReadByte()
		if err != nil {
			return body
		}
		if idx == 0xff { // terminator
			return append(out, idx)
		}
		typ, err := ReadVarInt(r)
		if err != nil {
			return body
		}
		if idx == 16 && typ == metaTypeVarInt {
			idx = 18
		}
		out = append(out, idx)
		out = AppendVarInt(out, typ)
		switch typ {
		case metaTypeByte, metaTypeBoolean:
			b, err := r.ReadByte()
			if err != nil {
				return body
			}
			out = append(out, b)
		case metaTypeVarInt, metaTypePose:
			v, err := ReadVarInt(r)
			if err != nil {
				return body
			}
			out = AppendVarInt(out, v)
		case metaTypeFloat:
			var f [4]byte
			if _, err := io.ReadFull(r, f[:]); err != nil {
				return body
			}
			out = append(out, f[:]...)
		case metaTypeBlockPos:
			var p [8]byte
			if _, err := io.ReadFull(r, p[:]); err != nil {
				return body
			}
			out = append(out, p[:]...)
		case metaTypeOptBlockPos:
			present, err := r.ReadByte()
			if err != nil {
				return body
			}
			out = append(out, present)
			if present != 0 {
				var p [8]byte
				if _, err := io.ReadFull(r, p[:]); err != nil {
					return body
				}
				out = append(out, p[:]...)
			}
		default:
			return body // cube mobs never carry a Slot — bail, leave the body untouched
		}
	}
}

// unmapCreativeSlot (serverbound): i16 slot, Slot — translate the client's item ID
// back to canonical so the server stores the right item.
func unmapCreativeSlot(version int32, body []byte) []byte {
	r := bytes.NewReader(body)
	if !skip(r, 2) { // slot (i16)
		return body
	}
	// Serverbound: the client speaks ITS component ids — translate back.
	return remapTrailingSlot(body, r, func(i int32) int32 { return UnmapID(RegItem, version, i) },
		version, true)
}

// remapWindowItems: VarInt window, VarInt stateId, VarInt count, count Slots, then
// the carried (cursor) Slot. Each Slot is fully parsed to reach the next; bails to
// the original body if a Slot carries components (which our encoder never sends).
func remapWindowItems(version int32, body []byte) []byte {
	r := bytes.NewReader(body)
	window, e1 := ReadVarInt(r)
	stateID, e2 := ReadVarInt(r)
	count, e3 := ReadVarInt(r)
	if e1 != nil || e2 != nil || e3 != nil || count < 0 {
		return body
	}
	out := AppendVarInt(nil, window)
	out = AppendVarInt(out, stateID)
	out = AppendVarInt(out, count)
	remap := func(i int32) int32 { return RemapID(RegItem, version, i) }
	for i := int32(0); i <= count; i++ { // count slots + 1 carried (cursor) slot
		if !copyFullSlot(r, &out, remap, version, false) {
			return body
		}
	}
	return out
}

// Structured-component ids our encoder is allowed to attach to a Slot.
// max_damage/damage are bare-VarInt payloads and their ids are IDENTICAL from
// 1.21.5 through 26.2 (verified against ViaVersion's mappings). enchantments
// (a varint count of (id, level) varint pairs) sits at 10 through 1.21.10 —
// 1.21.11 (774) inserted components ahead of it, shifting it to 13. The
// ENCHANTMENT ids inside are our own declared registry order (identical for
// every client version, because the server declares the same list), so only
// the component id itself needs the per-version swap.
const (
	componentMaxDamage       = 2  // minecraft:max_damage (varint)
	componentDamage          = 3  // minecraft:damage (varint) — the durability bar
	componentEnchantments    = 10 // minecraft:enchantments, canonical (770-773)
	componentEnchantments774 = 13 // …at 1.21.11 / 26.1 / 26.2 (774+)
	componentCustomName      = 5  // minecraft:custom_name (NBT text), canonical
	componentStoredEnch      = 34 // minecraft:stored_enchantments (books), canonical
)

// enchCompID is the minecraft:enchantments component id at a client version.
func enchCompID(version int32) int32 {
	if version >= 774 {
		return componentEnchantments774
	}
	return componentEnchantments
}

// storedEnchCompID / customNameCompID: per-version ids (ViaVersion mappings —
// 1.21.11 inserted components; 26.2 inserted one more before stored_ench).
func storedEnchCompID(version int32) int32 {
	switch {
	case version >= 776:
		return 42
	case version >= 774:
		return 41
	}
	return componentStoredEnch
}

func customNameCompID(version int32) int32 {
	if version >= 774 {
		return 6
	}
	return componentCustomName
}

// copyFullSlot reads a complete Slot and writes it with the item ID remapped.
// Components are supported ONLY for the whitelisted ids above (all our encoder
// ever sends); anything richer returns false so the caller leaves the packet
// untouched rather than corrupt it. enchIn is the enchantments component id to
// expect on the way in, enchOut what to write out (they differ across the
// 774 boundary, and swap roles between clientbound and serverbound slots).
func copyFullSlot(r *bytes.Reader, out *[]byte, remap func(int32) int32, version int32, serverbound bool) bool {
	// Component-id translation pairs for this direction: canonical (770) ids on
	// the server side, the client version's ids on the wire side.
	enchIn, enchOut := int32(componentEnchantments), enchCompID(version)
	storedIn, storedOut := int32(componentStoredEnch), storedEnchCompID(version)
	nameIn, nameOut := int32(componentCustomName), customNameCompID(version)
	if serverbound {
		enchIn, enchOut = enchOut, enchIn
		storedIn, storedOut = storedOut, storedIn
		nameIn, nameOut = nameOut, nameIn
	}
	count, err := ReadVarInt(r)
	if err != nil {
		return false
	}
	*out = AppendVarInt(*out, count)
	if count <= 0 {
		return true
	}
	item, err := ReadVarInt(r)
	if err != nil {
		return false
	}
	*out = AppendVarInt(*out, remap(item))
	addC, e1 := ReadVarInt(r)
	remC, e2 := ReadVarInt(r)
	if e1 != nil || e2 != nil || addC < 0 || addC > 3 || remC != 0 {
		return false // richer components than we ever send — don't guess
	}
	*out = AppendVarInt(*out, addC)
	*out = AppendVarInt(*out, remC)
	for i := int32(0); i < addC; i++ {
		cid, err := ReadVarInt(r)
		if err != nil {
			return false
		}
		switch cid {
		case componentDamage, componentMaxDamage:
			val, err := ReadVarInt(r)
			if err != nil {
				return false
			}
			*out = AppendVarInt(*out, cid)
			*out = AppendVarInt(*out, val)
		case enchIn, storedIn:
			// enchantments / stored_enchantments share the wire shape:
			// varint count + (varint enchId, varint level) pairs.
			outID := enchOut
			if cid == storedIn {
				outID = storedOut
			}
			n, err := ReadVarInt(r)
			if err != nil || n < 0 || n > 8 {
				return false
			}
			*out = AppendVarInt(*out, outID)
			*out = AppendVarInt(*out, n)
			for j := int32(0); j < n; j++ {
				id, e1 := ReadVarInt(r)
				lvl, e2 := ReadVarInt(r)
				if e1 != nil || e2 != nil {
					return false
				}
				*out = AppendVarInt(*out, id) // our declared order — same on every version
				*out = AppendVarInt(*out, lvl)
			}
		case nameIn:
			// custom_name: an NBT text component. We only ever emit a nameless
			// TAG_String (0x08, u16 length, bytes) — anything else bails.
			tag, err := r.ReadByte()
			if err != nil || tag != 0x08 {
				return false
			}
			var ln [2]byte
			if _, err := io.ReadFull(r, ln[:]); err != nil {
				return false
			}
			strLen := int(ln[0])<<8 | int(ln[1])
			str := make([]byte, strLen)
			if _, err := io.ReadFull(r, str); err != nil {
				return false
			}
			*out = AppendVarInt(*out, nameOut)
			*out = append(*out, 0x08, ln[0], ln[1])
			*out = append(*out, str...)
		default:
			return false
		}
	}
	return true
}

// remapEquipment rewrites set_equipment: varint eid + a topBitSet-terminated
// list of (i8 slot, Slot) entries — the slot byte's high bit says another
// entry follows. The layout is unchanged 770→26.2 (ViaVersion only renumbers
// the packet id); only the item ids inside the Slots shift.
func remapEquipment(version int32, body []byte) []byte {
	r := bytes.NewReader(body)
	eid, err := ReadVarInt(r)
	if err != nil {
		return body
	}
	out := AppendVarInt(nil, eid)
	remap := func(i int32) int32 { return RemapID(RegItem, version, i) }
	for {
		slot, err := r.ReadByte()
		if err != nil {
			return body
		}
		out = append(out, slot)
		if !copyFullSlot(r, &out, remap, version, false) {
			return body
		}
		if slot&0x80 == 0 { // last entry
			return out
		}
	}
}

// rewriteEntityVelocity773 converts Set Entity Velocity from the ≤772 form
// (entityId + 3×i16, 1/8000 block/tick) to the 773+ form (entityId + lpVec3).
func rewriteEntityVelocity773(body []byte) []byte {
	r := bytes.NewReader(body)
	eid, err := ReadVarInt(r)
	if err != nil {
		return body
	}
	var v [6]byte
	if _, err := io.ReadFull(r, v[:]); err != nil {
		return body
	}
	vec := func(i int) float64 {
		return float64(int16(uint16(v[i])<<8|uint16(v[i+1]))) / 8000
	}
	out := AppendVarInt(nil, eid)
	return appendLpVec3(out, vec(0), vec(2), vec(4))
}

// appendLpVec3 encodes Mojang's 1.21.9+ low-precision movement vector (wire
// facts from ViaVersion's LowPrecisionVectorType): a zero vector is a single
// 0x00; otherwise a packed int64 — 2 scale bits + 1 continuation bit, then
// three 15-bit components quantized as (v/scale*0.5+0.5)*32766 at bit offsets
// 3/18/33 — written as u8, u8, u32be, plus a trailing VarInt with the rest of
// the scale when it exceeds 2 bits.
func appendLpVec3(b []byte, x, y, z float64) []byte {
	const maxQ = 32766.0
	sanitize := func(v float64) float64 {
		const absMax = float64(int64(1)<<34 - 1)
		if v != v { // NaN
			return 0
		}
		return math.Max(-absMax, math.Min(absMax, v))
	}
	x, y, z = sanitize(x), sanitize(y), sanitize(z)
	maxPart := math.Max(math.Abs(x), math.Max(math.Abs(y), math.Abs(z)))
	if maxPart < 1/maxQ {
		return append(b, 0x00)
	}
	scale := int64(math.Ceil(maxPart))
	scaleBits := scale & 3
	cont := scaleBits != scale
	if cont {
		scaleBits |= 4
	}
	pack := func(v float64) int64 {
		return int64(math.Round((v/float64(scale)*0.5 + 0.5) * maxQ))
	}
	packed := scaleBits | pack(x)<<3 | pack(y)<<18 | pack(z)<<33
	b = append(b, byte(packed), byte(packed>>8))
	b = AppendI32(b, int32(uint32(packed>>16)))
	if cont {
		b = AppendVarInt(b, int32(scale>>2))
	}
	return b
}

// weatheringCopperStateSerializer maps a client protocol to its
// WEATHERING_COPPER_STATE entity-data serializer id — the value-type of a copper
// golem's index-16 oxidation state. The engine emits it as a plain INT
// (metaTypeVarInt) placeholder because canonical-770 has no such serializer;
// FixCopperGolemMeta restores the real value-type for clients that have the
// copper golem (else index 16 would be an INT where the client expects
// WEATHERING_COPPER_STATE → "invalid entity data item type" disconnect). Ids from
// the vanilla EntityDataSerializers registration order (774 vs 776 differ by
// the four sound-variant serializers 26.1 inserted).
var weatheringCopperStateSerializer = map[int32]int32{774: 34, 775: 38, 776: 38}

// FixCopperGolemMeta rewrites a copper golem's index-16 metadata serializer type
// from the INT placeholder to WEATHERING_COPPER_STATE for the client version. The
// value (a VarInt ordinal, 0 unaffected → 3 oxidized) is unchanged. A gateway
// calls this only for entities it knows are copper golems (eid→type tracking).
func FixCopperGolemMeta(version int32, body []byte) []byte {
	weatherID, ok := weatheringCopperStateSerializer[version]
	if !ok {
		return body
	}
	r := bytes.NewReader(body)
	eid, err := ReadVarInt(r)
	if err != nil {
		return body
	}
	out := AppendVarInt(nil, eid)
	for {
		idx, err := r.ReadByte()
		if err != nil {
			return body
		}
		if idx == 0xff { // terminator
			return append(out, idx)
		}
		typ, err := ReadVarInt(r)
		if err != nil {
			return body
		}
		outType := typ
		if idx == 16 && typ == metaTypeVarInt {
			outType = weatherID // restore the WEATHERING_COPPER_STATE value-type
		}
		out = append(out, idx)
		out = AppendVarInt(out, outType)
		switch typ {
		case metaTypeByte, metaTypeBoolean:
			b, err := r.ReadByte()
			if err != nil {
				return body
			}
			out = append(out, b)
		case metaTypeVarInt, metaTypePose:
			v, err := ReadVarInt(r)
			if err != nil {
				return body
			}
			out = AppendVarInt(out, v)
		case metaTypeFloat:
			var f [4]byte
			if _, err := io.ReadFull(r, f[:]); err != nil {
				return body
			}
			out = append(out, f[:]...)
		case metaTypeBlockPos:
			var p [8]byte
			if _, err := io.ReadFull(r, p[:]); err != nil {
				return body
			}
			out = append(out, p[:]...)
		case metaTypeOptBlockPos:
			present, err := r.ReadByte()
			if err != nil {
				return body
			}
			out = append(out, present)
			if present != 0 {
				var p [8]byte
				if _, err := io.ReadFull(r, p[:]); err != nil {
					return body
				}
				out = append(out, p[:]...)
			}
		default:
			return body // copper golem metadata carries no Slot — bail untouched
		}
	}
}
