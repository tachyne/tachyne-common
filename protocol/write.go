package protocol

import (
	"encoding/binary"
	"math"
)

// Fixed-width big-endian append helpers for building packet payloads.

func AppendBool(b []byte, v bool) []byte {
	if v {
		return append(b, 1)
	}
	return append(b, 0)
}

func AppendU8(b []byte, v uint8) []byte   { return append(b, v) }
func AppendI8(b []byte, v int8) []byte    { return append(b, byte(v)) }
func AppendU16(b []byte, v uint16) []byte { return binary.BigEndian.AppendUint16(b, v) }
func AppendI16(b []byte, v int16) []byte  { return binary.BigEndian.AppendUint16(b, uint16(v)) }
func AppendI32(b []byte, v int32) []byte  { return binary.BigEndian.AppendUint32(b, uint32(v)) }
func AppendI64(b []byte, v int64) []byte  { return binary.BigEndian.AppendUint64(b, uint64(v)) }

func AppendF32(b []byte, v float32) []byte {
	return binary.BigEndian.AppendUint32(b, math.Float32bits(v))
}

func AppendF64(b []byte, v float64) []byte {
	return binary.BigEndian.AppendUint64(b, math.Float64bits(v))
}
