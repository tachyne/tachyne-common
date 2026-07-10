package protocol

import "encoding/binary"

// A block Position is packed into one i64: x in the high 26 bits, z in the
// middle 26, y in the low 12 (all signed).

// AppendPosition appends a packed block position.
func AppendPosition(b []byte, x, y, z int) []byte {
	v := (uint64(x)&0x3FFFFFF)<<38 | (uint64(z)&0x3FFFFFF)<<12 | (uint64(y) & 0xFFF)
	return AppendI64(b, int64(v))
}

// ReadPosition decodes a packed block position from the first 8 bytes of data,
// sign-extending each field.
func ReadPosition(data []byte) (x, y, z int) {
	v := int64(binary.BigEndian.Uint64(data))
	x = int(v >> 38)
	y = int(v << 52 >> 52)
	z = int(v << 26 >> 38)
	return
}
