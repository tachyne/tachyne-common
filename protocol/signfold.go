package protocol

import (
	"bytes"
	"strings"
	"sync"
)

// 26.3 moved a sign item's text out of its block entity tag into three item
// components of its own (sign_text_front, sign_text_back, waxed). A 26.2
// client has none of them: it keeps a copied sign's text where 26.2's
// BlockItem does, in block_entity_data — the sign's own tag, front_text,
// back_text and is_waxed beside the rest. signFold gathers the three on
// their way to a 26.2 client and writes them back into block_entity_data,
// so a creative client that sends the stack back hands the text in with it
// (the engine reads a block_entity_data's sign keys like the components).

// signFold is what a stack's 26.3 sign components hold, as tag entries.
type signFold struct {
	entries []byte // named NBT entries: front_text, back_text, is_waxed
}

func (f *signFold) empty() bool { return len(f.entries) == 0 }

// dyeNames is DyeColor by id (SignText's color is the id on the wire and
// the name in the tag).
var dyeNames = [16]string{"white", "orange", "magenta", "light_blue", "yellow", "lime", "pink",
	"gray", "light_gray", "cyan", "purple", "blue", "brown", "green", "red", "black"}

// foldSignText reads one SignText.STREAM_CODEC value and adds it to the fold
// as the tag SignText.DIRECT_CODEC writes: messages, filtered_messages when
// they are there, color and has_glowing_text.
func (f *signFold) foldSignText(r *bytes.Reader, key string) bool {
	lines := func() ([][]byte, bool) {
		out := make([][]byte, 4)
		for i := range out {
			var v []byte
			if !copyNBTValue(r, &v) {
				return nil, false
			}
			c, ok := textAsCompound(v)
			if !ok {
				return nil, false
			}
			out[i] = c
		}
		return out, true
	}
	msgs, ok := lines()
	if !ok {
		return false
	}
	has, err := r.ReadByte()
	if err != nil || has > 1 {
		return false
	}
	var filtered [][]byte
	if has == 1 {
		if filtered, ok = lines(); !ok {
			return false
		}
	}
	color, err := ReadVarInt(r)
	if err != nil || color < 0 || color > 15 {
		return false
	}
	glow, err := r.ReadByte()
	if err != nil || glow > 1 {
		return false
	}
	b := NBTCompound(f.entries, key)
	list := func(name string, ls [][]byte) {
		b = NBTCompoundList(b, name, len(ls))
		for _, c := range ls {
			b = append(b, c...) // entries and their End
		}
	}
	list("messages", msgs)
	if filtered != nil {
		list("filtered_messages", filtered)
	}
	b = NBTString(b, "color", dyeNames[color])
	b = NBTBool(b, "has_glowing_text", glow == 1)
	f.entries = NBTEnd(b)
	return true
}

// textAsCompound is a network-NBT text component as a list element of a
// list of compounds: a compound's own body, or a plain string as
// {text: "…"} (a tag list holds one element type).
func textAsCompound(v []byte) ([]byte, bool) {
	if len(v) < 1 {
		return nil, false
	}
	switch v[0] {
	case nbtCompound:
		return v[1:], true
	case nbtString:
		if len(v) < 3 {
			return nil, false
		}
		n := int(v[1])<<8 | int(v[2])
		if len(v) != 3+n {
			return nil, false
		}
		b := NBTString(nil, "text", string(v[3:]))
		return NBTEnd(b), true
	}
	return nil, false
}

// hangingSignItems are the canonical ids of the hanging sign items, whose
// block entity is hanging_sign rather than sign.
var hangingSignItems = sync.OnceValue(func() map[int32]bool {
	m := map[int32]bool{}
	for name, id := range canonicalItemIDs {
		if strings.HasSuffix(name, "_hanging_sign") {
			m[id] = true
		}
	}
	return m
})

// signBlockEntityName is the block entity a canonical sign item places.
func signBlockEntityName(item int32) string {
	if hangingSignItems()[item] {
		return "minecraft:hanging_sign"
	}
	return "minecraft:sign"
}

// apply writes the fold into a stack's copied patch for a client of version:
// into the block_entity_data copied at [bedAt, len) of its payload (bedAt
// < 0: none), or as a block_entity_data of the sign's own. countAt is the
// patch's added-component count byte. It reports whether it could.
func (f *signFold) apply(out *[]byte, countAt, bedStart, bedPayload, bedEnd int, version, item int32) bool {
	var typ int32
	var tag []byte // the compound's entries, without its End
	if bedStart >= 0 {
		r := bytes.NewReader((*out)[bedPayload:bedEnd])
		t, err := ReadVarInt(r)
		if err != nil {
			return false
		}
		rest := (*out)[bedEnd-r.Len() : bedEnd]
		if len(rest) < 2 || rest[0] != nbtCompound || rest[len(rest)-1] != nbtEnd {
			return false
		}
		typ, tag = t, append([]byte(nil), rest[1:len(rest)-1]...)
	} else {
		name := signBlockEntityName(item)
		t, ok := BlockEntityTypeID(version, name)
		if !ok {
			return false
		}
		typ = t
		tag = NBTString(nil, "id", name)
	}
	comp := AppendVarInt(nil, laterCompID(componentBlockEntityData, version))
	comp = AppendVarInt(comp, typ)
	comp = append(comp, nbtCompound)
	comp = append(comp, tag...)
	comp = append(comp, f.entries...)
	comp = append(comp, nbtEnd)
	if bedStart >= 0 {
		tail := append([]byte(nil), (*out)[bedEnd:]...)
		*out = append(append((*out)[:bedStart], comp...), tail...)
		return true
	}
	*out = append(*out, comp...)
	(*out)[countAt]++
	return true
}
