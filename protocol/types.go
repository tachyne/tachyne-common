// Package protocol implements the low-level Minecraft Java Edition wire types
// and packet framing. Everything here is version-agnostic: VarInts, strings,
// and the length-prefixed frame format are stable across protocol versions.
package protocol

import (
	"bytes"
	"errors"
	"io"
)

const (
	segmentBits = 0x7F
	continueBit = 0x80
)

// ErrVarIntTooBig is returned when a VarInt exceeds its 5-byte maximum.
var ErrVarIntTooBig = errors.New("protocol: VarInt is too big")

// ReadVarInt reads a Minecraft VarInt: 7 data bits per byte, little-endian
// groups, with the high bit signalling continuation.
func ReadVarInt(r io.ByteReader) (int32, error) {
	var value int32
	var pos uint
	for {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		value |= int32(b&segmentBits) << pos
		if b&continueBit == 0 {
			return value, nil
		}
		pos += 7
		if pos >= 32 {
			return 0, ErrVarIntTooBig
		}
	}
}

// AppendVarInt appends the VarInt encoding of v to b and returns the result.
func AppendVarInt(b []byte, v int32) []byte {
	u := uint32(v)
	for {
		if u&^uint32(segmentBits) == 0 {
			return append(b, byte(u))
		}
		b = append(b, byte(u&segmentBits|continueBit))
		u >>= 7
	}
}

// AppendVarLong appends a 64-bit LEB128 VarLong (same scheme as VarInt, 64-bit).
func AppendVarLong(b []byte, v int64) []byte {
	u := uint64(v)
	for {
		if u&^uint64(segmentBits) == 0 {
			return append(b, byte(u))
		}
		b = append(b, byte(u&segmentBits|continueBit))
		u >>= 7
	}
}

// ReadString reads a VarInt-length-prefixed UTF-8 string.
func ReadString(r *bytes.Reader) (string, error) {
	n, err := ReadVarInt(r)
	if err != nil {
		return "", err
	}
	if n < 0 || int(n) > r.Len() {
		return "", errors.New("protocol: invalid string length")
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

// AppendString appends a VarInt-length-prefixed UTF-8 string.
func AppendString(b []byte, s string) []byte {
	b = AppendVarInt(b, int32(len(s)))
	return append(b, s...)
}
