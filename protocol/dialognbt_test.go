package protocol

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

// The bytes NbtOps would build: compound keys (sorted here), strings as
// strings, whole numbers ints, fractions doubles, booleans bytes.
func TestJSONToNetworkNBTBytes(t *testing.T) {
	got, ok := JSONToNetworkNBT([]byte(`{"type":"minecraft:notice","title":"Hi","w":200,"f":1.5,"b":true}`))
	if !ok {
		t.Fatal("not converted")
	}
	want := []byte{nbtCompound}
	want = append(want, nbtByte, 0, 1, 'b', 1)
	want = append(want, nbtDouble, 0, 1, 'f')
	want = AppendF64(want, 1.5)
	want = append(want, nbtString, 0, 5, 't', 'i', 't', 'l', 'e', 0, 2, 'H', 'i')
	want = append(want, nbtString, 0, 4, 't', 'y', 'p', 'e', 0, 16)
	want = append(want, "minecraft:notice"...)
	want = append(want, nbtInt, 0, 1, 'w', 0, 0, 0, 200)
	want = append(want, nbtEnd)
	if !bytes.Equal(got, want) {
		t.Fatalf("\n got % x\nwant % x", got, want)
	}
}

// A mixed list is a compound list with the non-compounds wrapped as {"": x}
// (ListTag.wrapIfNeeded); an empty list is an End list.
func TestJSONToNetworkNBTMixedList(t *testing.T) {
	got, ok := JSONToNetworkNBT([]byte(`{"a":["x",{"b":1}],"e":[]}`))
	if !ok {
		t.Fatal("not converted")
	}
	want := []byte{nbtCompound, nbtList, 0, 1, 'a', nbtCompound, 0, 0, 0, 2}
	want = append(want, nbtString, 0, 0, 0, 1, 'x', nbtEnd) // {"": "x"}
	want = append(want, nbtInt, 0, 1, 'b', 0, 0, 0, 1, nbtEnd)
	want = append(want, nbtList, 0, 1, 'e', nbtEnd, 0, 0, 0, 0)
	want = append(want, nbtEnd)
	if !bytes.Equal(got, want) {
		t.Fatalf("\n got % x\nwant % x", got, want)
	}
	if _, ok := JSONToNetworkNBT([]byte(`null`)); ok {
		t.Fatal("null converted")
	}
}

// Reading back unwraps the wrappers and restores the shape.
func TestNetworkNBTRoundTrip(t *testing.T) {
	in := `{"type":"minecraft:multi_action","title":{"text":"Pick","color":"gold"},"columns":2,` +
		`"actions":[{"label":"One","action":{"type":"minecraft:run_command","command":"say one"}}],` +
		`"inputs":[{"type":"minecraft:boolean","key":"ok","label":"OK?","initial":true}],"mixed":["s",{"text":"c"}],"n":[1,2,3]}`
	nbt, ok := JSONToNetworkNBT([]byte(in))
	if !ok {
		t.Fatal("not converted")
	}
	js, err := NetworkNBTToJSON(bytes.NewReader(nbt))
	if err != nil {
		t.Fatal(err)
	}
	var got, want map[string]any
	json.Unmarshal(js, &got)
	json.Unmarshal([]byte(in), &want)
	// NBT has no booleans: true comes back as the byte 1.
	want["inputs"].([]any)[0].(map[string]any)["initial"] = float64(1)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip\n got %s\nwant %s", js, in)
	}
	if js, err := NetworkNBTToJSON(bytes.NewReader([]byte{nbtEnd})); err != nil || string(js) != "null" {
		t.Fatalf("an End root = %s (%v)", js, err)
	}
}
