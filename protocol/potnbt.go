package protocol

// potnbt.go — the decorated pot block entity's update tag. Vanilla syncs its
// "sherds" list (PotDecorations): the four faces as item NAMES, back, left,
// right, front. Names rather than ids, so nothing here renumbers between
// versions. The pot's CONTENTS are not synced — vanilla keeps what is inside
// a pot a secret until it breaks, and so does this.

// AppendPotNBT appends a decorated pot's update tag as network NBT (nameless
// root compound). All four faces are written, empty ones as the plain brick,
// because the list is positional. A pot with nothing set at all writes no
// list, which is the undecorated pot.
func AppendPotNBT(b []byte, sherds [4]string) []byte {
	b = append(b, NBTRoot()...)
	any := false
	for _, s := range sherds {
		if s != "" {
			any = true
		}
	}
	if !any {
		return NBTEnd(b)
	}
	out := sherds
	for i, s := range out {
		if s == "" {
			out[i] = "minecraft:brick"
		}
	}
	b = NBTStringList(b, "sherds", out[:])
	return NBTEnd(b)
}
