package protocol

import (
	"bytes"
	"testing"
)

// The sulfur cube is not new to 26.2 clients: 26.2 registered it (entity
// 130) and 26.3 moved it to 133. So it is shifted to the 26.2 id and never
// substituted — a slime stand-in would have hidden the swallowed block.
func TestSulfurCubeKnownTo26_2(t *testing.T) {
	canon := CanonicalEntity("sulfur_cube")
	if IsSubstituted(776, canon) || IsSubstituted(777, canon) {
		t.Fatal("the sulfur cube is substituted for a client that has it")
	}
	if !IDPresent(RegEntity, 776, canon) {
		t.Fatal("26.2 reads as lacking the sulfur cube")
	}
	if got := RemapID(RegEntity, 776, canon); got != 130 {
		t.Errorf("26.2 id %d, want 130", got)
	}
	if got := RemapID(RegEntity, 777, canon); got != canon {
		t.Errorf("26.3 id %d, want the canonical %d", got, canon)
	}
}

// The engine sends a sulfur cube's own fields at their 26.x indices (size
// 18, MAX_FUSE 19, FROM_BUCKET 20, next to the baby flag at 16). Nothing on
// the way to a 26.x client may move them: not the chain's serializer remap,
// and not the slime shift (which moves a VarInt at 16 only).
func TestSulfurCubeMetaPassesThrough(t *testing.T) {
	body := AppendVarInt(nil, 77)
	body = append(body, 16)
	body = AppendVarInt(body, 8) // baby: Boolean
	body = append(body, 1)
	body = append(body, 18)
	body = AppendVarInt(body, 1) // size: VarInt
	body = AppendVarInt(body, 1)
	body = append(body, 19)
	body = AppendVarInt(body, 1) // MAX_FUSE: VarInt
	body = AppendVarInt(body, 120)
	body = append(body, 20)
	body = AppendVarInt(body, 8) // FROM_BUCKET: Boolean
	body = append(body, 0)
	body = append(body, 0xff)
	for _, v := range []int32{776, 777} {
		if out := remapEntityMeta(v, body); !bytes.Equal(out, body) {
			t.Errorf("proto %d: the chain rewrote the sulfur cube's data: % x", v, out)
		}
		if out := ShiftCubeMobMeta(v, body); !bytes.Equal(out, body) {
			t.Errorf("proto %d: the slime shift moved the sulfur cube's data: % x", v, out)
		}
	}
}

// ItemTag263 hands a gateway the 26.3 tag data as names.
func TestItemTag263(t *testing.T) {
	if got := ItemTag263("minecraft:sulfur_cube_archetype/explosive"); len(got) != 1 || got[0] != "minecraft:tnt" {
		t.Errorf("explosive archetype %v, want [minecraft:tnt]", got)
	}
	if got := ItemTag263("minecraft:sulfur_cube_archetype/light"); len(got) != 16 {
		t.Errorf("light archetype has %d items, want the 16 wools", len(got))
	}
	if ItemTag263("minecraft:no_such_tag") != nil {
		t.Error("an unknown tag resolved")
	}
	a := ItemTag263("minecraft:sulfur_cube_archetype/hot")
	a[0] = "changed"
	if ItemTag263("minecraft:sulfur_cube_archetype/hot")[0] != "minecraft:magma_block" {
		t.Error("the caller can write into the tag data")
	}
}
