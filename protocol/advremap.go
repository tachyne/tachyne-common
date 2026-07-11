package protocol

import (
	"bytes"
	"io"
)

// update_advancements translation. Two rewrites ride the same packet walk:
//
//   - Registry pass (canonical ids, every client version incl. 770): each
//     display icon's item id is canonical 1.21.11(774) and must be remapped to
//     the client version — remapAdvancementIcons, dispatched from
//     remapClientboundIDs like every other Slot-carrying packet.
//   - Layout pass (the 774→775 step): 26.1 changed DisplayInfo's icon from a
//     full Slot (count, item, components) to an ItemStackTemplate (item,
//     count, components) — the 26.1+ DisplayInfo/ItemStackTemplate codecs.
//     rewriteAdvIconTemplate reorders the fields; registered in
//     translate_steps.go under the packet's 774-space id.
//
// Packet shape (vanilla 1.21.5 ClientboundUpdateAdvancementsPacket): bool
// reset · VarInt added × (string id · opt string parent · opt DisplayInfo ·
// VarInt×(VarInt×string) requirements · bool telemetry) · trailer (removed +
// progress + showAdvancements — icon-free, copied raw). DisplayInfo: NBT
// title · NBT description · Slot icon · VarInt frame · i32 flags · [string
// background if flags&1] · f32 x · f32 y.

// advancementsID774 is update_advancements in the 774 id space — the id the
// 774→775 step sees after the 773 step's renumbering (123 → 128). The
// tripwire test pins it against the generated protomap route.
const advancementsID774 = 0x80

// walkAdvancementIcons rewrites each present display icon via iconFn (read
// the Slot from r, append the replacement to out; false = bail). Everything
// else is copied verbatim. Any parse failure returns body unchanged — same
// don't-guess rule as the other walkers.
func walkAdvancementIcons(body []byte, iconFn func(r *bytes.Reader, out *[]byte) bool) []byte {
	r := bytes.NewReader(body)
	out := make([]byte, 0, len(body)+64)
	pos := func() int { return len(body) - r.Len() }
	flushed := 0
	if _, err := r.ReadByte(); err != nil { // reset
		return body
	}
	n, err := ReadVarInt(r)
	if err != nil || n < 0 {
		return body
	}
	skipString := func() bool {
		l, err := ReadVarInt(r)
		if err != nil || l < 0 || int(l) > r.Len() {
			return false
		}
		return skipN(r, int64(l)) == nil
	}
	for i := int32(0); i < n; i++ {
		if !skipString() { // id
			return body
		}
		hasParent, err := r.ReadByte()
		if err != nil {
			return body
		}
		if hasParent != 0 && !skipString() {
			return body
		}
		hasDisplay, err := r.ReadByte()
		if err != nil {
			return body
		}
		if hasDisplay != 0 {
			if SkipNetworkNBT(r) != nil || SkipNetworkNBT(r) != nil { // title, description
				return body
			}
			// flush everything up to the icon, rewrite it, resume copying after
			out = append(out, body[flushed:pos()]...)
			if !iconFn(r, &out) {
				return body
			}
			flushed = pos()
			if _, err := ReadVarInt(r); err != nil { // frame
				return body
			}
			var flags [4]byte
			if _, err := io.ReadFull(r, flags[:]); err != nil {
				return body
			}
			if flags[3]&1 != 0 && !skipString() { // background
				return body
			}
			if skipN(r, 8) != nil { // x, y
				return body
			}
		}
		ng, err := ReadVarInt(r) // requirements groups
		if err != nil || ng < 0 {
			return body
		}
		for g := int32(0); g < ng; g++ {
			nc, err := ReadVarInt(r)
			if err != nil || nc < 0 {
				return body
			}
			for c := int32(0); c < nc; c++ {
				if !skipString() {
					return body
				}
			}
		}
		if _, err := r.ReadByte(); err != nil { // telemetry
			return body
		}
	}
	// trailer (removed + progress + showAdvancements) carries no icons
	out = append(out, body[flushed:]...)
	return out
}

// remapAdvancementIcons is the registry pass: icon item ids canonical → the
// client version's. Uses copyFullSlot, the shared Slot copier.
func remapAdvancementIcons(version int32, body []byte) []byte {
	remap := func(i int32) int32 { return RemapID(RegItem, version, i) }
	return walkAdvancementIcons(body, func(r *bytes.Reader, out *[]byte) bool {
		return copyFullSlot(r, out, remap, version, false)
	})
}

// rewriteAdvIconTemplate is the 774→775 layout pass: Slot (count, item,
// components) → ItemStackTemplate (item, count, components). Icons we compose
// are always plain items (0 added / 0 removed components — the renderer
// strips them); richer patches bail out untouched.
func rewriteAdvIconTemplate(_ State, body []byte) []byte {
	return walkAdvancementIcons(body, func(r *bytes.Reader, out *[]byte) bool {
		count, err := ReadVarInt(r)
		if err != nil || count <= 0 {
			return false
		}
		item, err := ReadVarInt(r)
		if err != nil {
			return false
		}
		addC, e1 := ReadVarInt(r)
		remC, e2 := ReadVarInt(r)
		if e1 != nil || e2 != nil || addC != 0 || remC != 0 {
			return false
		}
		*out = AppendVarInt(*out, item)
		*out = AppendVarInt(*out, count)
		*out = AppendVarInt(*out, 0)
		*out = AppendVarInt(*out, 0)
		return true
	})
}
