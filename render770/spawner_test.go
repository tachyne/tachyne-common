package render770

import (
	"bytes"
	"testing"

	"github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// The spawner's update tag is what draws the little mob turning inside the
// cage. Re-parse it rather than trusting the writer: position, the
// block-entity type, and NBT whose SpawnData names the mob.
func TestSpawnerDataCarriesItsMob(t *testing.T) {
	p := SpawnerData(attach.SpawnerData{X: 10, Y: -59, Z: -300, Entity: "minecraft:blaze"})
	if p.ID != IDBlockEntityData {
		t.Fatalf("packet id %d, want block_entity_data (%d)", p.ID, IDBlockEntityData)
	}
	r := bytes.NewReader(p.Body)
	var pos [8]byte
	if _, err := r.Read(pos[:]); err != nil {
		t.Fatal(err)
	}
	x, y, z := protocol.ReadPosition(pos[:])
	if x != 10 || y != -59 || z != -300 {
		t.Errorf("position %d,%d,%d, want 10,-59,-300", x, y, z)
	}
	if typ, _ := protocol.ReadVarInt(r); typ != beTypeSpawner {
		t.Errorf("block-entity type %d, want %d", typ, beTypeSpawner)
	}
	nbt := p.Body[len(p.Body)-r.Len():]
	// The name has to be in there, inside a SpawnData → entity → id chain.
	for _, want := range []string{"SpawnData", "entity", "id", "minecraft:blaze",
		"Delay", "MinSpawnDelay", "RequiredPlayerRange"} {
		if !bytes.Contains(nbt, []byte(want)) {
			t.Errorf("the update tag never mentions %q", want)
		}
	}
	if got := bytes.Count(nbt, []byte{0}); got == 0 {
		t.Error("the compound was never closed")
	}
}

// A spawner nothing has set yet carries no SpawnData at all — vanilla draws
// that one empty rather than guessing a mob.
func TestSpawnerDataWithoutAMob(t *testing.T) {
	p := SpawnerData(attach.SpawnerData{X: 1, Y: 2, Z: 3})
	if bytes.Contains(p.Body, []byte("SpawnData")) {
		t.Error("an unset spawner should carry no SpawnData")
	}
	if !bytes.Contains(p.Body, []byte("MaxNearbyEntities")) {
		t.Error("it should still carry its configuration")
	}
}

// The configuration defaults are BaseSpawner's, and a caller's own values
// override them — checked by reading the shorts back out of the NBT.
func TestSpawnerConfigDefaults(t *testing.T) {
	def := SpawnerData(attach.SpawnerData{Entity: "minecraft:zombie"})
	if v, ok := nbtShortAfter(def.Body, "MinSpawnDelay"); !ok || v != 200 {
		t.Errorf("default MinSpawnDelay %d (found=%v), want 200", v, ok)
	}
	if v, ok := nbtShortAfter(def.Body, "RequiredPlayerRange"); !ok || v != 16 {
		t.Errorf("default RequiredPlayerRange %d, want 16", v)
	}
	set := SpawnerData(attach.SpawnerData{Entity: "minecraft:zombie", MinDelay: 40, PlayerRange: 8})
	if v, _ := nbtShortAfter(set.Body, "MinSpawnDelay"); v != 40 {
		t.Errorf("MinSpawnDelay %d, want the 40 we asked for", v)
	}
	if v, _ := nbtShortAfter(set.Body, "RequiredPlayerRange"); v != 8 {
		t.Errorf("RequiredPlayerRange %d, want 8", v)
	}
}

// nbtShortAfter finds a named TAG_Short and returns its value.
func nbtShortAfter(body []byte, name string) (int16, bool) {
	i := bytes.Index(body, []byte(name))
	if i < 0 || i+len(name)+2 > len(body) {
		return 0, false
	}
	v := body[i+len(name):]
	return int16(v[0])<<8 | int16(v[1]), true
}
