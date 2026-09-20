package protocol

import (
	"bytes"
	"testing"
)

func TestPotNBTNamesItsFaces(t *testing.T) {
	b := AppendPotNBT(nil, [4]string{"minecraft:angler_pottery_sherd", "", "minecraft:heart_pottery_sherd", ""})
	for _, want := range []string{"sherds", "minecraft:angler_pottery_sherd", "minecraft:heart_pottery_sherd", "minecraft:brick"} {
		if !bytes.Contains(b, []byte(want)) {
			t.Errorf("the update tag never mentions %q", want)
		}
	}
	// An undecorated pot writes an empty compound, not a list of bricks.
	if plain := AppendPotNBT(nil, [4]string{}); bytes.Contains(plain, []byte("sherds")) {
		t.Error("a plain pot should carry no sherds list")
	}
}
