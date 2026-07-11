package protocol

// signnbt.go — the sign block entity's update tag, the shared building block
// for both delivery paths: the chunk packet's block-entity section (composed
// engine-side into ChunkHeader.BEs) and the standalone block_entity_data
// packet (composed in render770). Layout mirrors the vanilla SignText codec:
// front_text/back_text compounds each holding messages (a 4-string list —
// plain literal components serialize as bare NBT strings), color and
// has_glowing_text, plus is_waxed on the root. filtered_messages is omitted
// (vanilla writes it only when text filtering diverges from the raw lines).

// SignSideNBT is one side's serialized state: four plain-text lines, the dye
// color name ("" = black, the vanilla default) and the glow-ink flag.
type SignSideNBT struct {
	Lines [4]string
	Color string
	Glow  bool
}

// AppendSignNBT appends a sign block entity's update tag as network NBT (a
// nameless root compound): front_text, back_text, is_waxed.
func AppendSignNBT(b []byte, front, back SignSideNBT, waxed bool) []byte {
	b = append(b, NBTRoot()...)
	b = appendSignSide(b, "front_text", front)
	b = appendSignSide(b, "back_text", back)
	b = NBTBool(b, "is_waxed", waxed)
	return NBTEnd(b)
}

func appendSignSide(b []byte, name string, s SignSideNBT) []byte {
	color := s.Color
	if color == "" {
		color = "black"
	}
	b = NBTCompound(b, name)
	b = NBTStringList(b, "messages", s.Lines[:])
	b = NBTString(b, "color", color)
	b = NBTBool(b, "has_glowing_text", s.Glow)
	return NBTEnd(b)
}
