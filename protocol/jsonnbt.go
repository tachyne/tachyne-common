package protocol

import (
	"bytes"
	"encoding/json"
	"math"
	"sort"
)

// JSONToNBT turns a JSON value — a data pack's registry entry, as the
// engine hands it over — into network NBT (the type byte, then the payload
// of a nameless root), the way NbtOps writes what a codec decoded from
// JSON: strings stay strings, booleans become bytes, whole numbers ints
// (longs past the int range) and other numbers doubles, objects compounds.
// A list whose elements share one tag type is a list of that type (whole
// and fractional numbers together become doubles); a mixed list is a list
// of compounds with each non-compound element wrapped as {"": value}
// (ListTag's heterogeneous form). Every codec reads any number tag, so the
// widths need not match the codec's. ok is false for invalid JSON or null.
func JSONToNBT(raw []byte) ([]byte, bool) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil || d.More() || v == nil {
		return nil, false
	}
	t := jsonTagType(v)
	return appendJSONPayload([]byte{t}, v, t), true
}

// jsonTagType is the tag a JSON value becomes on its own.
func jsonTagType(v any) byte {
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

// listElemType is the element tag a JSON array's list takes.
func listElemType(list []any) byte {
	if len(list) == 0 {
		return nbtEnd
	}
	t := jsonTagType(list[0])
	for _, e := range list[1:] {
		et := jsonTagType(e)
		if et == t {
			continue
		}
		num := func(x byte) bool { return x == nbtInt || x == nbtLong || x == nbtDouble }
		if num(t) && num(et) {
			if t == nbtDouble || et == nbtDouble {
				t = nbtDouble
			} else {
				t = nbtLong
			}
			continue
		}
		return nbtCompound // heterogeneous: wrapped compounds
	}
	return t
}

// appendJSONPayload writes v's payload as tag type t (t may widen a number).
func appendJSONPayload(b []byte, v any, t byte) []byte {
	switch t {
	case nbtString:
		s, _ := v.(string)
		return appendNBTStringPayload(b, s)
	case nbtByte:
		if x, _ := v.(bool); x {
			return append(b, 1)
		}
		return append(b, 0)
	case nbtInt, nbtLong, nbtDouble:
		n, _ := v.(json.Number)
		switch t {
		case nbtInt:
			i, _ := n.Int64()
			return AppendI32(b, int32(i))
		case nbtLong:
			i, err := n.Int64()
			if err != nil {
				f, _ := n.Float64()
				i = int64(f)
			}
			return AppendI64(b, i)
		}
		f, _ := n.Float64()
		return AppendF64(b, f)
	case nbtCompound:
		m, ok := v.(map[string]any)
		if !ok { // a heterogeneous list's element: wrapped under the empty key
			et := jsonTagType(v)
			b = append(b, et)
			b = appendNBTStringPayload(b, "")
			b = appendJSONPayload(b, v, et)
			return append(b, nbtEnd)
		}
		keys := make([]string, 0, len(m))
		for k := range m {
			if m[k] != nil {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys) // deterministic bytes
		for _, k := range keys {
			et := jsonTagType(m[k])
			b = append(b, et)
			b = appendNBTStringPayload(b, k)
			b = appendJSONPayload(b, m[k], et)
		}
		return append(b, nbtEnd)
	case nbtList:
		list, _ := v.([]any)
		et := listElemType(list)
		b = append(b, et)
		b = AppendI32(b, int32(len(list)))
		for _, e := range list {
			b = appendJSONPayload(b, e, et)
		}
		return b
	}
	return b
}
