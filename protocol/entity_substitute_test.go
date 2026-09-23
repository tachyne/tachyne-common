package protocol

import (
	"bytes"
	"testing"
)

// spawnEntityBody builds a minimal clientbound Spawn Entity body with the given
// entity-type id (entityId + 16-byte uuid + type + trailing bytes remapper skips).
func spawnEntityBody(typ int32) []byte {
	b := AppendVarInt(nil, 1234)
	b = append(b, make([]byte, 16)...)
	b = AppendVarInt(b, typ)
	return append(b, make([]byte, 20)...)
}

func spawnEntityType(body []byte) int32 {
	r := bytes.NewReader(body)
	ReadVarInt(r)
	r.Read(make([]byte, 16))
	t, _ := ReadVarInt(r)
	return t
}

// TestEntitySubstitutionOldClient: a 1.21.5 (770) client must receive a real
// older stand-in for every post-1.21.5 entity, never the dangling canonical id
// (which would render a wrong mob and risk a metadata-decode disconnect). The
// wire ids are the 1.21.5 ids of the fallback species.
func TestEntitySubstitutionOldClient(t *testing.T) {
	want := map[int32]int32{ // canonical id -> expected 1.21.5 wire id
		CanonicalEntity("happy_ghast"):     55,  // -> ghast
		CanonicalEntity("copper_golem"):    53,  // -> frog
		CanonicalEntity("mannequin"):       5,   // -> armor_stand
		CanonicalEntity("camel_husk"):      19,  // -> camel
		CanonicalEntity("nautilus"):        121, // -> squid
		CanonicalEntity("zombie_nautilus"): 58,  // -> glow_squid
		CanonicalEntity("parched"):         109, // -> skeleton
	}
	for canon, wire := range want {
		got := spawnEntityType(remapSpawnEntityType(770, spawnEntityBody(canon)))
		if got != wire {
			t.Errorf("770 spawn of canonical %d: wire type %d, want %d", canon, got, wire)
		}
		// The dangling canonical id must never reach a 1.21.5 client.
		if got == canon {
			t.Errorf("770 spawn of canonical %d passed through unsubstituted", canon)
		}
	}
}

// TestEntitySubstitutionRespectsIntroductionVersion: an entity is only
// substituted for clients OLDER than the version that introduced it; a client
// that has the entity natively receives it unchanged (via the normal range
// shift), and the canonical version itself is a pure passthrough.
func TestEntitySubstitutionRespectsIntroductionVersion(t *testing.T) {
	// happy_ghast (added proto 771): substituted for 770, native for 771+.
	if substituteEntityType(770, CanonicalEntity("happy_ghast")) != CanonicalEntity("ghast") {
		t.Error("happy_ghast should substitute to ghast for 770")
	}
	if substituteEntityType(771, CanonicalEntity("happy_ghast")) != CanonicalEntity("happy_ghast") {
		t.Error("happy_ghast should be native for 771")
	}
	if substituteEntityType(776, CanonicalEntity("happy_ghast")) != CanonicalEntity("happy_ghast") {
		t.Error("happy_ghast should be native for 26.2 (776)")
	}
	// copper_golem (added proto 773): substituted for 772, native for 773+.
	if substituteEntityType(772, CanonicalEntity("copper_golem")) != CanonicalEntity("frog") {
		t.Error("copper_golem should substitute to frog for 772")
	}
	if substituteEntityType(773, CanonicalEntity("copper_golem")) != CanonicalEntity("copper_golem") {
		t.Error("copper_golem should be native for 773")
	}
	// A vanilla-since-forever entity is never touched.
	if substituteEntityType(770, CanonicalEntity("ghast")) != CanonicalEntity("ghast") {
		t.Error("ghast must never be substituted")
	}
}
