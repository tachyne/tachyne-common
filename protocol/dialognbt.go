package protocol

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"math"
	"sort"
	"unicode/utf16"
)

// Generic JSON ⇄ network-NBT, for data that crosses the wire through a codec
// rather than a fixed layout: a dialog (Dialog.DIRECT_CODEC over NbtOps) out,
// a custom click action's payload tag in.
//
// JSON → NBT follows what NbtOps builds from the same data: strings are
// strings, booleans bytes, whole numbers ints (longs past the int range),
// other numbers doubles, objects compounds, arrays lists. A list of one tag
// type is a list of that type; a mixed list is a list of compounds with each
// non-compound element wrapped as {"": element} (ListTag.wrapIfNeeded), which
// the reading side unwraps. The codecs read any numeric tag as a number, so
// the widths need not match the declared field types.

const (
	nbtByteArrayTag = 7
	nbtLongArrayTag = 12
)

// JSONToNetworkNBT converts one JSON value into network NBT: the root's type
// byte then its payload (no name). ok is false for invalid JSON or null.
func JSONToNetworkNBT(raw []byte) ([]byte, bool) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil || d.More() || v == nil {
		return nil, false
	}
	return appendGenericTag(nil, v), true
}

// genericTagType is the NBT type a JSON value becomes (nbtEnd for null).
func genericTagType(v any) byte {
	switch x := v.(type) {
	case string:
		return nbtString
	case bool:
		return nbtByte
	case json.Number:
		if i, err := x.Int64(); err == nil {
			if i >= math.MinInt32 && i <= math.MaxInt32 {
				return nbtInt
			}
			return nbtLong
		}
		return nbtDouble
	case map[string]any:
		return nbtCompound
	case []any:
		return nbtList
	}
	return nbtEnd
}

// appendGenericTag writes a type byte and the payload.
func appendGenericTag(b []byte, v any) []byte {
	return appendGenericPayload(append(b, genericTagType(v)), v)
}

func appendGenericPayload(b []byte, v any) []byte {
	switch x := v.(type) {
	case string:
		return appendNBTStringPayload(b, x)
	case bool:
		if x {
			return append(b, 1)
		}
		return append(b, 0)
	case json.Number:
		switch genericTagType(x) {
		case nbtInt:
			i, _ := x.Int64()
			return AppendI32(b, int32(i))
		case nbtLong:
			i, _ := x.Int64()
			return AppendI64(b, i)
		}
		f, _ := x.Float64()
		return AppendF64(b, f)
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k, e := range x {
			if e != nil { // NbtOps has no null: an absent field
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			b = append(b, genericTagType(x[k]))
			b = appendNBTStringPayload(b, k)
			b = appendGenericPayload(b, x[k])
		}
		return append(b, nbtEnd)
	case []any:
		var elems []any
		for _, e := range x {
			if e != nil {
				elems = append(elems, e)
			}
		}
		if len(elems) == 0 {
			return AppendI32(append(b, nbtEnd), 0)
		}
		t := genericTagType(elems[0])
		for _, e := range elems[1:] {
			if genericTagType(e) != t {
				t = 0xff // mixed
				break
			}
		}
		if t != 0xff && !(t == nbtCompound && anyWrapperShaped(elems)) {
			b = append(b, t)
			b = AppendI32(b, int32(len(elems)))
			for _, e := range elems {
				b = appendGenericPayload(b, e)
			}
			return b
		}
		b = append(b, nbtCompound)
		b = AppendI32(b, int32(len(elems)))
		for _, e := range elems {
			if m, ok := e.(map[string]any); ok && !wrapperShaped(m) {
				b = appendGenericPayload(b, m)
				continue
			}
			b = appendGenericPayload(b, map[string]any{"": e}) // wrapElement
		}
		return b
	}
	return b
}

// wrapperShaped is ListTag.isWrapper: a compound whose only key is "" would
// be unwrapped on reading, so it has to be wrapped itself.
func wrapperShaped(m map[string]any) bool {
	_, ok := m[""]
	return ok && len(m) == 1
}

func anyWrapperShaped(elems []any) bool {
	for _, e := range elems {
		if m, ok := e.(map[string]any); ok && wrapperShaped(m) {
			return true
		}
	}
	return false
}

// maxNBTDepth bounds nesting (NbtAccounter allows 512).
const maxNBTDepth = 512

var errNBTDepth = errors.New("protocol: NBT nested too deep")

// NetworkNBTToJSON reads one network-NBT value (type byte, then payload) as
// JSON: compounds become objects, lists arrays (with ListTag's {"": x}
// wrappers unwrapped), numbers numbers, byte/int/long arrays arrays of
// numbers, strings strings. An End root (no tag) reads as null.
func NetworkNBTToJSON(r *bytes.Reader) (json.RawMessage, error) {
	t, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	if t == nbtEnd {
		return json.RawMessage("null"), nil
	}
	v, err := readGenericPayload(r, t, 0)
	if err != nil {
		return nil, err
	}
	return json.Marshal(v)
}

func readGenericPayload(r *bytes.Reader, t byte, depth int) (any, error) {
	if depth > maxNBTDepth {
		return nil, errNBTDepth
	}
	var buf [8]byte
	fixed := func(n int) ([]byte, error) {
		if _, err := io.ReadFull(r, buf[:n]); err != nil {
			return nil, err
		}
		return buf[:n], nil
	}
	length := func() (int, error) {
		x, err := fixed(4)
		if err != nil {
			return 0, err
		}
		n := int32(binary.BigEndian.Uint32(x))
		if n < 0 || int(n) > r.Len() {
			return 0, io.ErrUnexpectedEOF
		}
		return int(n), nil
	}
	switch t {
	case nbtByte:
		x, err := fixed(1)
		if err != nil {
			return nil, err
		}
		return int8(x[0]), nil
	case nbtShort:
		x, err := fixed(2)
		if err != nil {
			return nil, err
		}
		return int16(binary.BigEndian.Uint16(x)), nil
	case nbtInt:
		x, err := fixed(4)
		if err != nil {
			return nil, err
		}
		return int32(binary.BigEndian.Uint32(x)), nil
	case nbtLong:
		x, err := fixed(8)
		if err != nil {
			return nil, err
		}
		return int64(binary.BigEndian.Uint64(x)), nil
	case nbtFloat:
		x, err := fixed(4)
		if err != nil {
			return nil, err
		}
		return float64(math.Float32frombits(binary.BigEndian.Uint32(x))), nil
	case nbtDouble:
		x, err := fixed(8)
		if err != nil {
			return nil, err
		}
		return math.Float64frombits(binary.BigEndian.Uint64(x)), nil
	case nbtByteArrayTag:
		n, err := length()
		if err != nil {
			return nil, err
		}
		out := make([]int8, n)
		for i := range out {
			c, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			out[i] = int8(c)
		}
		return out, nil
	case nbtString:
		return readModifiedUTF8(r)
	case nbtList:
		et, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		n, err := length()
		if err != nil {
			return nil, err
		}
		out := make([]any, 0, n)
		for i := 0; i < n; i++ {
			v, err := readGenericPayload(r, et, depth+1)
			if err != nil {
				return nil, err
			}
			if m, ok := v.(map[string]any); ok && et == nbtCompound && wrapperShaped(m) {
				v = m[""] // ListTag.tryUnwrap
			}
			out = append(out, v)
		}
		return out, nil
	case nbtCompound:
		out := map[string]any{}
		for {
			et, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			if et == nbtEnd {
				return out, nil
			}
			name, err := readModifiedUTF8(r)
			if err != nil {
				return nil, err
			}
			v, err := readGenericPayload(r, et, depth+1)
			if err != nil {
				return nil, err
			}
			out[name] = v
		}
	case nbtIntArray, nbtLongArrayTag:
		n, err := length()
		if err != nil {
			return nil, err
		}
		out := make([]int64, n)
		for i := range out {
			if t == nbtIntArray {
				x, err := fixed(4)
				if err != nil {
					return nil, err
				}
				out[i] = int64(int32(binary.BigEndian.Uint32(x)))
			} else {
				x, err := fixed(8)
				if err != nil {
					return nil, err
				}
				out[i] = int64(binary.BigEndian.Uint64(x))
			}
		}
		return out, nil
	}
	return nil, errors.New("protocol: unknown NBT tag type")
}

// readModifiedUTF8 reads an NBT string: a u16 length and Java's modified
// UTF-8 (NUL as C0 80, supplementary characters as surrogate pairs).
func readModifiedUTF8(r *bytes.Reader) (string, error) {
	var l [2]byte
	if _, err := io.ReadFull(r, l[:]); err != nil {
		return "", err
	}
	n := int(binary.BigEndian.Uint16(l[:]))
	if n > r.Len() {
		return "", io.ErrUnexpectedEOF
	}
	raw := make([]byte, n)
	io.ReadFull(r, raw)
	units := make([]uint16, 0, n)
	for i := 0; i < len(raw); {
		c := raw[i]
		switch {
		case c < 0x80:
			units = append(units, uint16(c))
			i++
		case c&0xE0 == 0xC0 && i+1 < len(raw):
			units = append(units, uint16(c&0x1F)<<6|uint16(raw[i+1]&0x3F))
			i += 2
		case c&0xF0 == 0xE0 && i+2 < len(raw):
			units = append(units, uint16(c&0x0F)<<12|uint16(raw[i+1]&0x3F)<<6|uint16(raw[i+2]&0x3F))
			i += 3
		default:
			return "", errors.New("protocol: malformed NBT string")
		}
	}
	return string(utf16.Decode(units)), nil
}
