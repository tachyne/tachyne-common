package protocol

import (
	"encoding/binary"
	"testing"
)

// walkNBT validates a single named-tag payload starting at b[0] for the given
// tag type, returning the number of bytes consumed. It is an independent reader
// (not sharing code with the writer) so it catches structural encoding bugs.
func walkNBT(t *testing.T, b []byte, tag byte) int {
	t.Helper()
	switch tag {
	case nbtByte:
		return 1
	case 2: // short
		return 2
	case nbtInt, nbtFloat:
		return 4
	case 4, nbtDouble: // long, double
		return 8
	case nbtString:
		n := int(binary.BigEndian.Uint16(b))
		return 2 + n
	case nbtCompound:
		return walkCompound(t, b)
	default:
		t.Fatalf("unsupported NBT tag id %d", tag)
		return 0
	}
}

// walkCompound walks a compound payload (named tags until TAG_End).
func walkCompound(t *testing.T, b []byte) int {
	t.Helper()
	pos := 0
	for {
		tag := b[pos]
		pos++
		if tag == nbtEnd {
			return pos
		}
		nameLen := int(binary.BigEndian.Uint16(b[pos:]))
		pos += 2 + nameLen
		pos += walkNBT(t, b[pos:], tag)
	}
}

// walkRoot validates a nameless-root network-NBT value.
func walkRoot(t *testing.T, b []byte) int {
	t.Helper()
	if b[0] != nbtCompound {
		t.Fatalf("root tag = %d, want compound (%d)", b[0], nbtCompound)
	}
	return 1 + walkCompound(t, b[1:])
}

// TestRegistryPacketsStructurallyValid builds every Registry Data packet body
// exactly as the server sends it and re-parses it strictly, ensuring each
// entry — and any inline NBT — is well formed with no trailing bytes.
func TestRegistryPacketsStructurallyValid(t *testing.T) {
	withData := 0
	for _, reg := range SyncedRegistries {
		body := AppendString(nil, reg.ID)
		body = AppendVarInt(body, int32(len(reg.Entries)))
		for _, entry := range reg.Entries {
			body = AppendString(body, entry)
			if nbt, ok := RegistryEntryData(reg.ID, entry); ok {
				body = AppendBool(body, true)
				body = append(body, nbt...)
				withData++
			} else {
				body = AppendBool(body, false)
			}
		}

		// Re-parse strictly.
		r := newReader(body)
		r.string(t)          // registry id
		count := r.varint(t) // entry count
		if int(count) != len(reg.Entries) {
			t.Fatalf("%s: count %d != %d", reg.ID, count, len(reg.Entries))
		}
		for i := int32(0); i < count; i++ {
			r.string(t) // key
			has := r.byte(t)
			if has == 1 {
				n := walkRoot(t, r.rest())
				r.advance(n)
			}
		}
		if r.remaining() != 0 {
			t.Errorf("%s: %d trailing bytes after parse", reg.ID, r.remaining())
		}
	}
	if withData == 0 {
		t.Error("expected some registries to carry inline data")
	}
}

// minimal cursor over a byte slice for the strict re-parse.
type reader struct {
	b   []byte
	pos int
}

func newReader(b []byte) *reader { return &reader{b: b} }
func (r *reader) rest() []byte   { return r.b[r.pos:] }
func (r *reader) remaining() int { return len(r.b) - r.pos }
func (r *reader) advance(n int)  { r.pos += n }

func (r *reader) byte(t *testing.T) byte {
	t.Helper()
	v := r.b[r.pos]
	r.pos++
	return v
}

func (r *reader) varint(t *testing.T) int32 {
	t.Helper()
	v, err := ReadVarInt(&byteSliceReader{r: r})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func (r *reader) string(t *testing.T) string {
	t.Helper()
	n := r.varint(t)
	s := string(r.b[r.pos : r.pos+int(n)])
	r.pos += int(n)
	return s
}

// byteSliceReader adapts reader to io.ByteReader for ReadVarInt.
type byteSliceReader struct{ r *reader }

func (b *byteSliceReader) ReadByte() (byte, error) {
	v := b.r.b[b.r.pos]
	b.r.pos++
	return v, nil
}
