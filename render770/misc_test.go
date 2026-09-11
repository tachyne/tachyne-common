package render770

import (
	"bytes"
	"testing"

	"github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// A respawn carrying the last death location writes the optional GlobalPos:
// present, the dimension key, then the packed position.
func TestRespawnDeathLocation(t *testing.T) {
	plain := Respawn(attach.Dimension{Dim: 0, Gamemode: 0})
	withDeath := Respawn(attach.Dimension{Dim: 1, Gamemode: 0, Death: &attach.DeathPos{Dim: 1, X: 10, Y: 64, Z: -3}})
	if len(withDeath.Body) <= len(plain.Body) {
		t.Fatal("the death location should lengthen the packet")
	}
	want := protocol.AppendBool(nil, true)
	want = protocol.AppendString(want, "minecraft:the_nether")
	want = protocol.AppendPosition(want, 10, 64, -3)
	if !bytes.Contains(withDeath.Body, want) {
		t.Fatalf("death location bytes missing: %x", withDeath.Body)
	}
	if !bytes.Contains(plain.Body, []byte{0, 0, 63}) { // absent, cooldown 0, sea level 63
		t.Fatal("plain respawn should write an absent death location")
	}
}
