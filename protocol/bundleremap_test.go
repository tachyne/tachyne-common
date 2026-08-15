package protocol

import (
	"bytes"
	"testing"
)

// bundleSlot builds a bundle stack whose bundle_contents holds the given items,
// in canonical (770) numbering.
func bundleSlot(bundleItem int32, contents ...int32) []byte {
	b := AppendVarInt(nil, 1)                    // count
	b = AppendVarInt(b, bundleItem)              // the bundle itself
	b = AppendVarInt(b, 1)                       // 1 component to add
	b = AppendVarInt(b, 0)                       // 0 to remove
	b = AppendVarInt(b, componentBundleContents) // bundle_contents
	b = AppendVarInt(b, int32(len(contents)))    // list length
	for _, it := range contents {
		b = append(b, slotBytes(it)...)
	}
	return b
}

// readSlotItems pulls the item id of a slot and, if it carries bundle_contents,
// the ids of everything inside — decoded independently of the writer.
func readSlotItems(t *testing.T, r *bytes.Reader) (int32, int32, []int32) {
	t.Helper()
	count, _ := ReadVarInt(r)
	if count == 0 {
		return 0, 0, nil
	}
	item, _ := ReadVarInt(r)
	addC, _ := ReadVarInt(r)
	ReadVarInt(r) // remove count
	var compID int32
	var inner []int32
	for i := int32(0); i < addC; i++ {
		cid, _ := ReadVarInt(r)
		compID = cid
		n, _ := ReadVarInt(r)
		for j := int32(0); j < n; j++ {
			_, it, _ := readSlotItems(t, r)
			inner = append(inner, it)
		}
	}
	return compID, item, inner
}

// The point of recursing into bundle_contents: the ids of the items INSIDE the
// bundle are remapped too. A bundle full of diamonds sent to a 770 client must
// contain 770 diamonds, not canonical ones.
func TestBundleContentsRemapsWhatIsInside(t *testing.T) {
	const canonDiamond, wireDiamond = 898, 845 // 1.21.11 -> 1.21.5
	body := bundleSlot(canonDiamond, canonDiamond, canonDiamond)

	var out []byte
	r := bytes.NewReader(body)
	if !copyFullSlot(r, &out, func(i int32) int32 { return RemapID(RegItem, 770, i) }, 770, false) {
		t.Fatal("a bundle stack could not be copied; the component case is missing")
	}

	compID, item, inner := readSlotItems(t, bytes.NewReader(out))
	if compID != componentBundleContents {
		t.Errorf("component id %d, want %d for a 770 client", compID, componentBundleContents)
	}
	if item != wireDiamond {
		t.Errorf("bundle item id %d, want %d", item, wireDiamond)
	}
	if len(inner) != 2 {
		t.Fatalf("%d items inside, want 2", len(inner))
	}
	for _, it := range inner {
		if it != wireDiamond {
			t.Errorf("contained item %d, want the remapped %d — the ids inside a "+
				"bundle need translating too", it, wireDiamond)
		}
	}
}

// The component id itself renumbers per version, from the datagen reports.
func TestBundleComponentIDPerVersion(t *testing.T) {
	for _, c := range []struct {
		version int32
		want    int32
	}{{770, 41}, {772, 41}, {774, 48}, {776, 50}} {
		if got := bundleContentsCompID(c.version); got != c.want {
			t.Errorf("bundleContentsCompID(%d) = %d, want %d", c.version, got, c.want)
		}
	}
}

// Serverbound, the pair swaps: a client's id comes in and canonical goes out.
func TestBundleComponentSwapsDirectionServerbound(t *testing.T) {
	const canonDiamond = 898
	body := AppendVarInt(nil, 1)
	body = AppendVarInt(body, RemapID(RegItem, 776, canonDiamond))
	body = AppendVarInt(body, 1)
	body = AppendVarInt(body, 0)
	body = AppendVarInt(body, 50) // 26.2's bundle_contents
	body = AppendVarInt(body, 0)  // empty bundle

	var out []byte
	if !copyFullSlot(bytes.NewReader(body), &out, func(i int32) int32 { return UnmapID(RegItem, 776, i) }, 776, true) {
		t.Fatal("a 26.2 bundle could not be lifted to canonical")
	}
	compID, item, _ := readSlotItems(t, bytes.NewReader(out))
	if compID != componentBundleContents {
		t.Errorf("component id %d, want the canonical %d", compID, componentBundleContents)
	}
	if item != canonDiamond {
		t.Errorf("bundle item %d, want canonical %d", item, canonDiamond)
	}
}

// Unbounded nesting from a hostile client must not blow the stack.
func TestDeeplyNestedBundlesAreRefused(t *testing.T) {
	body := AppendVarInt(nil, 1)
	body = AppendVarInt(body, 1)
	body = AppendVarInt(body, 1)
	body = AppendVarInt(body, 0)
	inner := body
	for i := 0; i < maxBundleNesting+4; i++ {
		inner = AppendVarInt(inner, componentBundleContents)
		inner = AppendVarInt(inner, 1) // one item, itself a bundle
		inner = AppendVarInt(inner, 1) // count
		inner = AppendVarInt(inner, 1) // item
		inner = AppendVarInt(inner, 1) // 1 component
		inner = AppendVarInt(inner, 0) // 0 removed
	}
	var out []byte
	if copyFullSlot(bytes.NewReader(inner), &out, func(i int32) int32 { return i }, 770, true) {
		t.Error("a bundle nested past the limit was accepted")
	}
}
