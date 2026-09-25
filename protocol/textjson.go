package protocol

import (
	"bytes"
	"encoding/json"
	"math"
	"sort"
	"strconv"
)

// TextComponentNBT turns a JSON text component (what /tellraw takes) into
// its network-NBT form, the type byte then the payload of a nameless root.
// It follows ComponentSerialization's shapes: a string is a plain text
// component, an array is its first element with the rest appended as extra,
// an object keeps its keys. Children in extra and with are always written
// as compounds, so every list holds one tag type. Booleans become bytes,
// whole numbers ints and other numbers doubles — the component codec reads
// any number tag. ok is false for JSON that is not a component.
func TextComponentNBT(raw []byte) ([]byte, bool) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil || d.More() {
		return nil, false
	}
	switch x := v.(type) {
	case string:
		return appendNBTStringPayload([]byte{nbtString}, x), true
	case []any, map[string]any:
		c, ok := componentObject(x)
		if !ok {
			return nil, false
		}
		return appendNBTCompoundPayload([]byte{nbtCompound}, c), true
	}
	return nil, false
}

// componentObject is a component as a compound: strings and numbers become
// {"text": …}, an array folds its tail into the head's extra.
func componentObject(v any) (map[string]any, bool) {
	switch x := v.(type) {
	case map[string]any:
		return x, true
	case string:
		return map[string]any{"text": x}, true
	case json.Number:
		return map[string]any{"text": x.String()}, true
	case bool:
		return map[string]any{"text": strconv.FormatBool(x)}, true
	case []any:
		if len(x) == 0 {
			return nil, false
		}
		head, ok := componentObject(x[0])
		if !ok {
			return nil, false
		}
		out := make(map[string]any, len(head)+1)
		for k, v := range head {
			out[k] = v
		}
		extra, _ := out["extra"].([]any)
		out["extra"] = append(append([]any{}, extra...), x[1:]...)
		return out, true
	}
	return nil, false
}

// componentListKeys hold lists of components.
var componentListKeys = map[string]bool{"extra": true, "with": true}

func appendNBTCompoundPayload(b []byte, m map[string]any) []byte {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys) // deterministic bytes
	for _, k := range keys {
		v := m[k]
		if list, ok := v.([]any); ok && componentListKeys[k] {
			b = append(b, nbtList)
			b = appendNBTStringPayload(b, k)
			b = append(b, nbtCompound)
			b = AppendI32(b, int32(len(list)))
			for _, e := range list {
				c, ok := componentObject(e)
				if !ok {
					c = map[string]any{"text": ""}
				}
				b = appendNBTCompoundPayload(b, c)
			}
			continue
		}
		b = appendNBTNamed(b, k, v)
	}
	return append(b, nbtEnd)
}

// appendNBTNamed writes one named value of any JSON shape.
func appendNBTNamed(b []byte, name string, v any) []byte {
	switch x := v.(type) {
	case nil:
		return b
	case string:
		b = append(b, nbtString)
		b = appendNBTStringPayload(b, name)
		return appendNBTStringPayload(b, x)
	case bool:
		b = append(b, nbtByte)
		b = appendNBTStringPayload(b, name)
		if x {
			return append(b, 1)
		}
		return append(b, 0)
	case json.Number:
		if i, err := x.Int64(); err == nil && i >= math.MinInt32 && i <= math.MaxInt32 {
			b = append(b, nbtInt)
			b = appendNBTStringPayload(b, name)
			return AppendI32(b, int32(i))
		}
		f, _ := x.Float64()
		b = append(b, nbtDouble)
		b = appendNBTStringPayload(b, name)
		return AppendF64(b, f)
	case map[string]any:
		b = append(b, nbtCompound)
		b = appendNBTStringPayload(b, name)
		return appendNBTCompoundPayload(b, x)
	case []any:
		b = append(b, nbtList)
		b = appendNBTStringPayload(b, name)
		allStrings := len(x) > 0
		for _, e := range x {
			if _, ok := e.(string); !ok {
				allStrings = false
			}
		}
		if allStrings {
			b = append(b, nbtString)
			b = AppendI32(b, int32(len(x)))
			for _, e := range x {
				b = appendNBTStringPayload(b, e.(string))
			}
			return b
		}
		if len(x) == 0 {
			return AppendI32(append(b, nbtEnd), 0)
		}
		b = append(b, nbtCompound) // mixed: each element as a component compound
		b = AppendI32(b, int32(len(x)))
		for _, e := range x {
			c, ok := componentObject(e)
			if !ok {
				c = map[string]any{}
			}
			b = appendNBTCompoundPayload(b, c)
		}
		return b
	}
	return b
}

// appendNBTStringPayload writes an NBT string: a u16 length and Java's
// modified UTF-8 (NUL as two bytes, supplementary characters as surrogate
// pairs).
func appendNBTStringPayload(b []byte, s string) []byte {
	var enc []byte
	for _, r := range s {
		if len(enc) > 65535-6 { // room for the longest encoding; never split one
			break
		}
		switch {
		case r == 0:
			enc = append(enc, 0xC0, 0x80)
		case r < 0x80:
			enc = append(enc, byte(r))
		case r < 0x800:
			enc = append(enc, 0xC0|byte(r>>6), 0x80|byte(r&0x3F))
		case r < 0x10000:
			enc = append(enc, 0xE0|byte(r>>12), 0x80|byte((r>>6)&0x3F), 0x80|byte(r&0x3F))
		default:
			r -= 0x10000
			for _, u := range []rune{0xD800 + (r >> 10), 0xDC00 + (r & 0x3FF)} {
				enc = append(enc, 0xE0|byte(u>>12), 0x80|byte((u>>6)&0x3F), 0x80|byte(u&0x3F))
			}
		}
	}
	b = AppendU16(b, uint16(len(enc)))
	return append(b, enc...)
}
