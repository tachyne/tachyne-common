package protocol

// Minimal network-NBT writer (Java Edition 1.20.2+ uses a nameless root tag)
// plus a skipper for walking past NBT values inside packet bodies. Only the
// tag types we need are implemented on the writer side; the skipper handles
// every tag type.

import (
	"bytes"
	"encoding/binary"
	"io"
)

const (
	nbtEnd      = 0
	nbtByte     = 1
	nbtInt      = 3
	nbtFloat    = 5
	nbtDouble   = 6
	nbtString   = 8
	nbtList     = 9
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

// NBTStringList writes a named list of strings (element type TAG_String).
func NBTStringList(b []byte, name string, vals []string) []byte {
	b = append(b, nbtList)
	b = nbtName(b, name)
	b = append(b, nbtString)
	b = AppendI32(b, int32(len(vals)))
	for _, v := range vals {
		b = AppendU16(b, uint16(len(v)))
		b = append(b, v...)
	}
	return b
}

// NBTCompound opens a named child compound; close it with its own NBTEnd.
func NBTCompound(b []byte, name string) []byte {
	b = append(b, nbtCompound)
	return nbtName(b, name)
}

// NBTEnd closes the current compound.
func NBTEnd(b []byte) []byte { return append(b, nbtEnd) }

// SkipNetworkNBT advances r past one network-NBT value (nameless root: a type
// byte followed directly by the payload — the 1.20.2+ wire form used for text
// components). Returns an error on malformed input.
func SkipNetworkNBT(r *bytes.Reader) error {
	t, err := r.ReadByte()
	if err != nil {
		return err
	}
	return skipNBTPayload(r, t)
}

func skipNBTPayload(r *bytes.Reader, t byte) error {
	readI32 := func() (int32, error) {
		var buf [4]byte
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return 0, err
		}
		return int32(binary.BigEndian.Uint32(buf[:])), nil
	}
	switch t {
	case nbtEnd:
		return nil
	case 1: // byte
		return skipN(r, 1)
	case 2: // short
		return skipN(r, 2)
	case 3, 5: // int, float
		return skipN(r, 4)
	case 4, 6: // long, double
		return skipN(r, 8)
	case 7: // byte array
		n, err := readI32()
		if err != nil || n < 0 {
			return orEOF(err)
		}
		return skipN(r, int64(n))
	case nbtString:
		var buf [2]byte
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return err
		}
		return skipN(r, int64(binary.BigEndian.Uint16(buf[:])))
	case 9: // list
		et, err := r.ReadByte()
		if err != nil {
			return err
		}
		n, err := readI32()
		if err != nil || n < 0 {
			return orEOF(err)
		}
		for i := int32(0); i < n; i++ {
			if err := skipNBTPayload(r, et); err != nil {
				return err
			}
		}
		return nil
	case nbtCompound:
		for {
			ct, err := r.ReadByte()
			if err != nil {
				return err
			}
			if ct == nbtEnd {
				return nil
			}
			var buf [2]byte
			if _, err := io.ReadFull(r, buf[:]); err != nil {
				return err
			}
			if err := skipN(r, int64(binary.BigEndian.Uint16(buf[:]))); err != nil {
				return err
			}
			if err := skipNBTPayload(r, ct); err != nil {
				return err
			}
		}
	case 11: // int array
		n, err := readI32()
		if err != nil || n < 0 {
			return orEOF(err)
		}
		return skipN(r, 4*int64(n))
	case 12: // long array
		n, err := readI32()
		if err != nil || n < 0 {
			return orEOF(err)
		}
		return skipN(r, 8*int64(n))
	}
	return io.ErrUnexpectedEOF // unknown tag type
}

// skipN advances exactly n bytes, erroring at EOF (Seek alone would happily
// run past the end of a bytes.Reader).
func skipN(r *bytes.Reader, n int64) error {
	if int64(r.Len()) < n {
		return io.ErrUnexpectedEOF
	}
	_, err := r.Seek(n, io.SeekCurrent)
	return err
}

func orEOF(err error) error {
	if err != nil {
		return err
	}
	return io.ErrUnexpectedEOF
}
