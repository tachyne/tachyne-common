package protocol

// Minimal network-NBT writer (Java Edition 1.20.2+ uses a nameless root tag).
// Only the writer side and the tag types we need are implemented.

const (
	nbtEnd      = 0
	nbtByte     = 1
	nbtInt      = 3
	nbtFloat    = 5
	nbtDouble   = 6
	nbtString   = 8
	nbtCompound = 10
)

// NBTRoot starts a nameless root compound (the network-NBT framing). Append
// fields with the NBT* helpers, then close with NBTEnd.
func NBTRoot() []byte { return []byte{nbtCompound} }

func nbtName(b []byte, name string) []byte {
	b = AppendU16(b, uint16(len(name)))
	return append(b, name...)
}

func NBTByte(b []byte, name string, v int8) []byte {
	b = append(b, nbtByte)
	b = nbtName(b, name)
	return append(b, byte(v))
}

func NBTBool(b []byte, name string, v bool) []byte {
	if v {
		return NBTByte(b, name, 1)
	}
	return NBTByte(b, name, 0)
}

func NBTInt(b []byte, name string, v int32) []byte {
	b = append(b, nbtInt)
	b = nbtName(b, name)
	return AppendI32(b, v)
}

func NBTFloat(b []byte, name string, v float32) []byte {
	b = append(b, nbtFloat)
	b = nbtName(b, name)
	return AppendF32(b, v)
}

func NBTDouble(b []byte, name string, v float64) []byte {
	b = append(b, nbtDouble)
	b = nbtName(b, name)
	return AppendF64(b, v)
}

func NBTString(b []byte, name, v string) []byte {
	b = append(b, nbtString)
	b = nbtName(b, name)
	b = AppendU16(b, uint16(len(v)))
	return append(b, v...)
}

// NBTCompound opens a named child compound; close it with its own NBTEnd.
func NBTCompound(b []byte, name string) []byte {
	b = append(b, nbtCompound)
	return nbtName(b, name)
}

// NBTEnd closes the current compound.
func NBTEnd(b []byte) []byte { return append(b, nbtEnd) }
