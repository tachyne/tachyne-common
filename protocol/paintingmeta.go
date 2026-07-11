package protocol

// paintingmeta.go — the painting's variant rides entity metadata as a
// registry Holder (VarInt id+1; 0 would mean an inline definition). The
// registry is the synced minecraft:painting_variant list, whose order WE
// control and only ever append to — so a variant's holder id is identical on
// every client version. Only the SERIALIZER id shifts: canonical 770 says
// 30; 26.x dropped COMPOUND_TAG and inserted the sound-variant serializers,
// landing PAINTING_VARIANT on 34 (both from the vanilla
// EntityDataSerializers registration order).

import "bytes"

// PaintingVariantSerializer770 is the canonical (1.21.5) PAINTING_VARIANT
// entity-metadata serializer id the engine composes.
const PaintingVariantSerializer770 = 30

// paintingMeta776 is the 26.x shape of the variant entry: 26.x added a
// synched DIRECTION field on HangingEntity (index 8), pushing the painting
// variant to index 9, and its serializer renumbered to 34 (COMPOUND_TAG
// removed, sound-variant serializers added). The client's direction field
// is populated from the spawn packet's data, so only the variant entry
// needs rewriting.
type paintingMetaShape struct {
	index      byte
	serializer int32
}

var paintingVariantShape = map[int32]paintingMetaShape{776: {9, 34}}

// PaintingVariantIndex returns a variant's holder id: its index in the
// synced painting_variant registry (-1 if unknown). The 26.x-appended
// entries follow the base list — appended-only, so base ids are identical
// on every version (an appended variant is only valid on 26.x clients; the
// engine's canonical-1.21.11 placeable set never selects one).
func PaintingVariantIndex(name string) int32 {
	for _, reg := range SyncedRegistries {
		if reg.ID != "minecraft:painting_variant" {
			continue
		}
		for i, e := range reg.Entries {
			if e == name || e == "minecraft:"+name {
				return int32(i)
			}
		}
		for i, e := range extra26xEntries["minecraft:painting_variant"] {
			if e == name || e == "minecraft:"+name {
				return int32(len(reg.Entries) + i)
			}
		}
	}
	return -1
}

// PaintingVariantName is the inverse of PaintingVariantIndex ("" if out of
// range).
func PaintingVariantName(idx int32) string {
	for _, reg := range SyncedRegistries {
		if reg.ID != "minecraft:painting_variant" {
			continue
		}
		if int(idx) < len(reg.Entries) {
			return reg.Entries[idx]
		}
		extra := extra26xEntries["minecraft:painting_variant"]
		if i := int(idx) - len(reg.Entries); i < len(extra) {
			return extra[i]
		}
	}
	return ""
}

// PaintingComponentID is the minecraft:painting/variant item-component type
// id in a client version's registry (-1 when unknown) — the creative menu's
// painting presets carry the variant in this component.
func PaintingComponentID(version int32) int32 {
	switch {
	case version >= 770 && version <= 772:
		return 89
	case version == 776:
		return 103
	}
	return -1
}

// FixPaintingMeta rewrites a painting's metadata for the client's version:
// the PAINTING_VARIANT serializer id is renumbered, the holder value is
// version-stable. The engine sends paintings a single metadata entry
// (index 8, the variant); anything unexpected returns the body untouched.
func FixPaintingMeta(version int32, body []byte) []byte {
	shape, ok := paintingVariantShape[version]
	if !ok {
		return body
	}
	r := bytes.NewReader(body)
	eid, err := ReadVarInt(r)
	if err != nil {
		return body
	}
	idx, err1 := r.ReadByte()
	typ, err2 := ReadVarInt(r)
	if err1 != nil || err2 != nil || typ != PaintingVariantSerializer770 {
		return body
	}
	holder, err := ReadVarInt(r)
	if err != nil {
		return body
	}
	end, err := r.ReadByte()
	if err != nil || end != 0xff || r.Len() != 0 {
		return body
	}
	_ = idx
	out := AppendVarInt(nil, eid)
	out = append(out, shape.index)
	out = AppendVarInt(out, shape.serializer)
	out = AppendVarInt(out, holder)
	return append(out, 0xff)
}
