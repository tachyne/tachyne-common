package handover

import (
	"encoding/json"
	"reflect"
	"testing"
)

// roundTrip marshals v to JSON and unmarshals into a fresh value of the same
// type, asserting deep equality — the transport must be lossless over the wire.
func roundTrip[T any](t *testing.T, v T) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %T: %v", v, err)
	}
	var out T
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal %T: %v", v, err)
	}
	if !reflect.DeepEqual(v, out) {
		t.Fatalf("%T round-trip mismatch:\n in: %+v\nout: %+v\njson: %s", v, v, out, b)
	}
}

func samplePlayer() PlayerState {
	p := PlayerState{
		EID: 63, Name: "wesley", UUID: [16]byte{1, 2, 3, 15}, Dim: 0,
		X: -12.5, Y: 71, Z: 0.25, Yaw: 90, Pitch: -10,
		OnGround: true, Sprinting: true, Airborne: false, PeakY: 80,
		Gamemode: 0,
		Health:   18.5, Absorption: 4, Food: 17, Saturation: 2.5, Exhaustion: 1.2,
		Air: 280, FireSecs: 3,
		XPLevel: 12, XPPoints: 7,
		Effects:  []EffectState{{ID: 1, Amp: 1, Left: 45}, {ID: 10, Amp: 0, Left: 8}},
		BedSpawn: &[3]int32{-40, 64, 8},
	}
	// A couple of non-empty inventory rows [item,count,dmg,ench].
	p.Slots[0] = [5]int32{278, 1, 12, 0x03040000, 0} // a diamond pickaxe w/ enchant pack
	p.Slots[9] = [5]int32{1, 64, 0, 0, 0}            // a stack of stone
	p.Armor[3] = [5]int32{310, 1, 40, 0, 0}          // a helmet
	p.Offhand = [5]int32{289, 16, 0, 0, 0}           // shield-ish
	return p
}

func TestPlayerStateRoundTrip(t *testing.T) {
	roundTrip(t, samplePlayer())
	// Also the zero value + a nil-bed variant.
	roundTrip(t, PlayerState{})
	p := samplePlayer()
	p.BedSpawn = nil
	p.Effects = nil
	roundTrip(t, p)
}

func TestMigrateEnvelopeRoundTrip(t *testing.T) {
	pl := samplePlayer()
	roundTrip(t, MigrateEntity{Kind: KindPlayer, MigID: "abc-123", Player: &pl})
	roundTrip(t, MigrateEntity{Kind: KindVehicle, MigID: "v-9", Vehicle: &VehicleState{
		EID: 128, Dim: 0, UUID: [16]byte{9}, EType: 5, X: 1, Y: 2, Z: 3, Yaw: 45, Rider: 63,
	}})
	roundTrip(t, MigrateEntity{Kind: KindMob, MigID: "m-7", Mob: &MobState{
		EID: 64, EType: 3, Dim: 0, X: 5, Y: 65, Z: 5, Health: 20, Hostile: true,
		Owner: 0, Rider: 0, Riders: []int32{63, 127},
	}})
}

func TestPeerFramesRoundTrip(t *testing.T) {
	roundTrip(t, PeerHello{SID: 1, Topo: "deadbeef"})
	roundTrip(t, Ack{MigID: "abc-123", OK: true})
	roundTrip(t, Ack{MigID: "abc-123", OK: false, Err: "stale token"})
	roundTrip(t, Shadow{
		EID: 63, Name: "wesley", UUID: [16]byte{1}, Dim: 0,
		X: -8, Y: 71, Z: 0, Yaw: 90, Pitch: 0, HeadYaw: 88,
		OnGround: true, Sprinting: true, Sneaking: false,
	})
	roundTrip(t, ShadowGone{EID: 63})
}

// The MigrateEntity discriminant must select exactly one payload — a decoder
// should see only the field matching Kind populated.
func TestMigrateEntityOmitsUnsetPayloads(t *testing.T) {
	pl := samplePlayer()
	b, err := json.Marshal(MigrateEntity{Kind: KindPlayer, MigID: "x", Player: &pl})
	if err != nil {
		t.Fatal(err)
	}
	var m MigrateEntity
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m.Player == nil || m.Mob != nil || m.Vehicle != nil {
		t.Fatalf("expected only Player set, got player=%v mob=%v vehicle=%v", m.Player != nil, m.Mob != nil, m.Vehicle != nil)
	}
}
