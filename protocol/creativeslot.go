package protocol

import (
	"bytes"
	"encoding/binary"
	"io"
	"strings"
)

// The creative-mode slot, serverbound. ServerboundSetCreativeModeSlotPacket
// carries its stack with ItemStack.OPTIONAL_UNTRUSTED_STREAM_CODEC: the
// component patch is DataComponentPatch.DELIMITED_STREAM_CODEC, every added
// component's value behind a VarInt byte length (1.21.5 through 26.3). The
// chain turns it into the canonical stack the rest of the pipeline speaks:
// the item id unmapped, the patch undelimited and each component's id and
// layout brought back to canonical, as the slot copier does for every other
// serverbound stack.

// Canonical ids of two components the creative menu fills in.
const (
	// ComponentPaintingVariant770 is painting/variant: a painting preset's
	// Holder<PaintingVariant> (registry id + 1).
	ComponentPaintingVariant770 = componentPaintingVariant
	// ComponentEntityData770 is entity_data: the spawned entity's tag — an
	// armor stand item's pose and flags.
	ComponentEntityData770 = componentEntityData
)

// unmapCreativeSlot (serverbound): i16 slot, then the Slot. An item the
// canonical registry never had cannot be shifted back — the reverse table
// would land it on whatever entry now holds that id (a 26.3 client's poplar
// planks became redstone ore in the world) — so the slot is emptied, and the
// inventory the engine pushes back tells the client so. Components the
// canonical copier does not know, and removed components, are left out: the
// engine keeps only what it can read.
func unmapCreativeSlot(version int32, body []byte) []byte {
	r := bytes.NewReader(body)
	if !skip(r, 2) { // slot (i16)
		return body
	}
	at := len(body) - r.Len()
	count, err := ReadVarInt(r)
	if err != nil || count <= 0 {
		return body // an empty slot: nothing to translate
	}
	item, err := ReadVarInt(r)
	if err != nil {
		return body
	}
	out := append([]byte(nil), body[:at]...)
	if IDAdded(RegItem, version, item) {
		return AppendVarInt(out, 0) // empty slot
	}
	out = AppendVarInt(out, count)
	out = AppendVarInt(out, UnmapID(RegItem, version, item))
	patch, ok := canonicalPatchFromDelimited(r, version)
	if !ok {
		patch = []byte{0, 0} // unreadable components: the bare item
	}
	return append(out, patch...)
}

// canonicalPatchFromDelimited reads a DELIMITED component patch in the
// client's numbering and returns the canonical (undelimited) patch: each
// added component the copier knows, translated; the rest dropped; no
// removals. False only when the patch itself cannot be read.
func canonicalPatchFromDelimited(r *bytes.Reader, version int32) ([]byte, bool) {
	addC, e1 := ReadVarInt(r)
	remC, e2 := ReadVarInt(r)
	if e1 != nil || e2 != nil || addC < 0 || remC < 0 || addC > 256 || remC > 256 {
		return nil, false
	}
	remap := func(i int32) int32 { return UnmapID(RegItem, version, i) }
	var entries []byte
	kept := int32(0)
	for i := int32(0); i < addC; i++ {
		cid, err := ReadVarInt(r)
		if err != nil {
			return nil, false
		}
		n, err := ReadVarInt(r)
		if err != nil || n < 0 || int(n) > r.Len() {
			return nil, false
		}
		payload := make([]byte, n)
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, false
		}
		if entry, ok := translateComponent(cid, payload, remap, version, true); ok {
			entries = append(entries, entry...)
			kept++
		}
	}
	for i := int32(0); i < remC; i++ { // removed components: ids only, dropped
		if _, err := ReadVarInt(r); err != nil {
			return nil, false
		}
	}
	out := AppendVarInt(nil, kept)
	out = AppendVarInt(out, 0)
	return append(out, entries...), true
}

// translateComponent runs one component (id + payload, undelimited) through
// the slot copier and returns the translated id + payload. The copier walks
// whole patches, so the component rides in a patch of one; false when the
// copier does not know it or the payload is not exactly one value.
func translateComponent(cid int32, payload []byte, remap func(int32) int32, version int32, serverbound bool) ([]byte, bool) {
	mini := AppendVarInt(AppendVarInt(AppendVarInt(nil, 1), 0), cid)
	mini = append(mini, payload...)
	mr := bytes.NewReader(mini)
	var conv []byte
	if !copyComponentPatch(mr, &conv, remap, version, serverbound, 0) || mr.Len() != 0 || len(conv) < 2 {
		return nil, false
	}
	return conv[2:], true // past the patch-of-one's counts (1, 0)
}

// WalkCanonicalComponents calls fn with each added component of a canonical
// component patch (add count, remove count, entries — attach.ItemStack's
// Components), its id and its payload. False when the patch holds a
// component the copier cannot measure (fn has seen the ones before it).
func WalkCanonicalComponents(patch []byte, fn func(id int32, payload []byte)) bool {
	r := bytes.NewReader(patch)
	addC, e1 := ReadVarInt(r)
	remC, e2 := ReadVarInt(r)
	if e1 != nil || e2 != nil || addC < 0 || remC < 0 {
		return false
	}
	same := func(i int32) int32 { return i }
	for i := int32(0); i < addC; i++ {
		start := len(patch) - r.Len()
		cid, err := ReadVarInt(r)
		if err != nil {
			return false
		}
		body := len(patch) - r.Len()
		// A patch of one over everything from this entry on: the copier
		// reads exactly one component and stops, which measures it.
		mini := append(AppendVarInt(AppendVarInt(nil, 1), 0), patch[start:]...)
		mr := bytes.NewReader(mini)
		var sink []byte
		if !copyComponentPatch(mr, &sink, same, Target, false, 0) {
			return false
		}
		used := len(mini) - mr.Len() - 2 // bytes of this entry (id + payload)
		end := start + used
		if end < body || end > len(patch) {
			return false
		}
		fn(cid, patch[body:end])
		if _, err := r.Seek(int64(end), io.SeekStart); err != nil {
			return false
		}
	}
	return true
}

// copyEntityData copies entity_data. 1.21.5's is a CustomData compound
// naming the entity type in its "id"; from 1.21.9 it is TypedEntityData —
// the entity type (a registry id) then the compound, whose "id" the client
// strips on decode. The canonical form is 1.21.5's: serverbound the type
// goes back into the compound as "id", clientbound it comes out of it.
func copyEntityData(r *bytes.Reader, out *[]byte, version int32, serverbound bool) bool {
	if version < 773 {
		return copyNBTValue(r, out)
	}
	if serverbound {
		typ, err := ReadVarInt(r)
		if err != nil {
			return false
		}
		name, ok := clientEntityName(version, typ)
		if !ok {
			return false
		}
		var tag []byte
		if !copyNBTValue(r, &tag) || len(tag) < 2 || tag[0] != nbtCompound {
			return false
		}
		*out = append(*out, nbtCompound)
		if _, had := nbtTopString(tag, "id"); !had {
			*out = NBTString(*out, "id", name)
		}
		*out = append(*out, tag[1:]...)
		return true
	}
	var tag []byte
	if !copyNBTValue(r, &tag) || len(tag) < 2 || tag[0] != nbtCompound {
		return false
	}
	name, ok := nbtTopString(tag, "id")
	if !ok {
		return false
	}
	id, ok := ClientEntity(version, strings.TrimPrefix(name, "minecraft:"))
	if !ok {
		return false // a type this client does not have
	}
	*out = AppendVarInt(*out, id)
	*out = append(*out, tag...) // the client strips the id itself
	return true
}

// clientEntityName is the "minecraft:"-prefixed name of a 26.2 or 26.3
// client's entity type id; false for any other version or an unknown id.
func clientEntityName(version, id int32) (string, bool) {
	var m map[string]int32
	switch {
	case version >= 777:
		m = entity263ID
	case version == 776:
		m = entity26xID
	default:
		return "", false
	}
	for name, v := range m {
		if v == id {
			if !strings.HasPrefix(name, "minecraft:") {
				name = "minecraft:" + name
			}
			return name, true
		}
	}
	return "", false
}

// nbtTopString finds a top-level string tag in a network-NBT compound (the
// nameless root: type byte, then entries).
func nbtTopString(tag []byte, key string) (string, bool) {
	if len(tag) < 1 || tag[0] != nbtCompound {
		return "", false
	}
	r := bytes.NewReader(tag[1:])
	for {
		t, err := r.ReadByte()
		if err != nil || t == nbtEnd {
			return "", false
		}
		var l [2]byte
		if _, err := io.ReadFull(r, l[:]); err != nil {
			return "", false
		}
		name := make([]byte, binary.BigEndian.Uint16(l[:]))
		if _, err := io.ReadFull(r, name); err != nil {
			return "", false
		}
		if t == nbtString && string(name) == key {
			if _, err := io.ReadFull(r, l[:]); err != nil {
				return "", false
			}
			v := make([]byte, binary.BigEndian.Uint16(l[:]))
			if _, err := io.ReadFull(r, v); err != nil {
				return "", false
			}
			return string(v), true
		}
		if err := skipNBTPayload(r, t); err != nil {
			return "", false
		}
	}
}
