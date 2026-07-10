package render770

import (
	"bytes"
	"testing"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// The oracles below are verbatim copies of the gomc hub's packet builders at
// the moment they were deleted (stage 1 of the domain-events refactor). The
// renderer must produce byte-identical output — this test IS the refactor's
// byte-diff verification, kept executable.

func oracleSpawnEntity(eid int32, etype int32, uuid [16]byte, x, y, z float64, yaw, pitch float32, data int32, vx, vy, vz float64) []byte {
	b := protocol.AppendVarInt(nil, eid)
	b = append(b, uuid[:]...)
	b = protocol.AppendVarInt(b, etype)
	b = protocol.AppendF64(b, x)
	b = protocol.AppendF64(b, y)
	b = protocol.AppendF64(b, z)
	b = protocol.AppendU8(b, angleByte(pitch))
	b = protocol.AppendU8(b, angleByte(yaw))
	b = protocol.AppendU8(b, angleByte(yaw)) // head yaw
	b = protocol.AppendVarInt(b, data)
	b = protocol.AppendI16(b, int16(vx*8000))
	b = protocol.AppendI16(b, int16(vy*8000))
	b = protocol.AppendI16(b, int16(vz*8000))
	return b
}

func oracleMoveLook(eid int32, dx, dy, dz int16, yaw, pitch float32, onGround bool) []byte {
	b := protocol.AppendVarInt(nil, eid)
	b = protocol.AppendI16(b, dx)
	b = protocol.AppendI16(b, dy)
	b = protocol.AppendI16(b, dz)
	b = protocol.AppendU8(b, angleByte(yaw))
	b = protocol.AppendU8(b, angleByte(pitch))
	return protocol.AppendBool(b, onGround)
}

func oracleSync(eid int32, x, y, z float64, yaw, pitch float32, onGround bool) []byte {
	b := protocol.AppendVarInt(nil, eid)
	b = protocol.AppendF64(b, x)
	b = protocol.AppendF64(b, y)
	b = protocol.AppendF64(b, z)
	b = protocol.AppendF64(b, 0)
	b = protocol.AppendF64(b, 0)
	b = protocol.AppendF64(b, 0)
	b = protocol.AppendF32(b, yaw)
	b = protocol.AppendF32(b, pitch)
	return protocol.AppendBool(b, onGround)
}

func oraclePlayerInfoAdd(uuid [16]byte, name string, props []attach.Property) []byte {
	b := protocol.AppendU8(nil, 0x01|0x04|0x08|0x10)
	b = protocol.AppendVarInt(b, 1)
	b = append(b, uuid[:]...)
	b = protocol.AppendString(b, name)
	b = protocol.AppendVarInt(b, int32(len(props)))
	for _, pr := range props {
		b = protocol.AppendString(b, pr.Name)
		b = protocol.AppendString(b, pr.Value)
		b = protocol.AppendBool(b, pr.Signature != "")
		if pr.Signature != "" {
			b = protocol.AppendString(b, pr.Signature)
		}
	}
	b = protocol.AppendVarInt(b, 1)
	b = protocol.AppendVarInt(b, 1)
	b = protocol.AppendVarInt(b, 0)
	return b
}

func eq(t *testing.T, what string, got Packet, wantID int32, want []byte) {
	t.Helper()
	if got.ID != wantID {
		t.Fatalf("%s: packet id 0x%02x, want 0x%02x", what, got.ID, wantID)
	}
	if !bytes.Equal(got.Body, want) {
		t.Fatalf("%s: body mismatch\n got %x\nwant %x", what, got.Body, want)
	}
}

func TestAddMatchesOracle(t *testing.T) {
	uuid := [16]byte{1, 2, 3, 4}
	v := NewEntityView()
	// A plain mob spawn (no data/velocity) and a projectile spawn.
	eq(t, "mob add",
		v.Add(attach.EntityAdd{EID: 7, UUID: uuid, Type: 95, X: 1.5, Y: 64, Z: -3.25, Yaw: 90, Pitch: -10}),
		IDSpawnEntity, oracleSpawnEntity(7, 95, uuid, 1.5, 64, -3.25, 90, -10, 0, 0, 0, 0))
	eq(t, "arrow add",
		v.Add(attach.EntityAdd{EID: 8, UUID: uuid, Type: 4, X: 0, Y: 70, Z: 0, Yaw: 45, Pitch: 30, Data: 12, VX: 1.2, VY: 0.4, VZ: -0.9}),
		IDSpawnEntity, oracleSpawnEntity(8, 4, uuid, 0, 70, 0, 45, 30, 12, 1.2, 0.4, -0.9))
}

func TestMoveDecisionTable(t *testing.T) {
	v := NewEntityView()
	// First sight without an Add → absolute sync.
	eq(t, "unknown entity",
		v.Move(attach.EntityMove{EID: 1, X: 10, Y: 64, Z: 10, Yaw: 5, Pitch: 1, OnGround: true}),
		IDEntitySync, oracleSync(1, 10, 64, 10, 5, 1, true))
	// A small move after that → relative, deltas against the last position.
	eq(t, "small move",
		v.Move(attach.EntityMove{EID: 1, X: 10.5, Y: 64, Z: 9.75, Yaw: 6, Pitch: 1, OnGround: true}),
		IDEntityMoveRot, oracleMoveLook(1, int16(0.5*4096), 0, int16(-0.25*4096), 6, 1, true))
	// A jump ≥ 7.5 blocks → absolute sync (i16 deltas can't carry it).
	eq(t, "big jump",
		v.Move(attach.EntityMove{EID: 1, X: 20, Y: 64, Z: 9.75, Yaw: 6, Pitch: 1}),
		IDEntitySync, oracleSync(1, 20, 64, 9.75, 6, 1, false))
	// An Add seeds the baseline, so the next move is relative.
	v.Add(attach.EntityAdd{EID: 2, X: 0, Y: 64, Z: 0})
	eq(t, "move after add",
		v.Move(attach.EntityMove{EID: 2, X: 0.25, Y: 64, Z: 0, OnGround: true}),
		IDEntityMoveRot, oracleMoveLook(2, int16(0.25*4096), 0, 0, 0, 0, true))
	// Remove forgets: the next move resyncs absolutely.
	v.Remove(attach.EntityRemove{EIDs: []int32{2}})
	eq(t, "move after remove",
		v.Move(attach.EntityMove{EID: 2, X: 0.5, Y: 64, Z: 0, OnGround: true}),
		IDEntitySync, oracleSync(2, 0.5, 64, 0, 0, 0, true))
}

func TestPeriodicResync(t *testing.T) {
	v := NewEntityView()
	v.Add(attach.EntityAdd{EID: 3, X: 0, Y: 64, Z: 0})
	syncs := 0
	for i := 0; i < 100; i++ {
		p := v.Move(attach.EntityMove{EID: 3, X: float64(i) * 0.1, Y: 64, Z: 0, OnGround: true})
		if p.ID == IDEntitySync {
			syncs++
		}
	}
	if syncs != 2 { // every 40th move over 100 moves
		t.Fatalf("expected 2 periodic resyncs in 100 small moves, got %d", syncs)
	}
}

// NoSync movement (the dragon: 776 clients lose entities to
// sync_entity_position) must never resync after first sight — oversized
// deltas saturate and converge, exactly the monolith's proven behavior.
func TestNoSyncNeverResyncs(t *testing.T) {
	v := NewEntityView()
	v.Add(attach.EntityAdd{EID: 9, X: 0, Y: 100, Z: 0})
	for i := 0; i < 200; i++ { // far past the 40-move periodic resync
		p := v.Move(attach.EntityMove{EID: 9, X: float64(i), Y: 100, Z: 0, NoSync: true})
		if p.ID != IDEntityMoveRot {
			t.Fatalf("move %d rendered 0x%02x, want relative 0x%02x", i, p.ID, IDEntityMoveRot)
		}
	}
	// A 20-block jump saturates (~8 blocks/packet) and converges over moves.
	v2 := NewEntityView()
	v2.Add(attach.EntityAdd{EID: 9, X: 0, Y: 100, Z: 0})
	for i := 0; i < 4; i++ {
		if p := v2.Move(attach.EntityMove{EID: 9, X: 20, Y: 100, Z: 0, NoSync: true}); p.ID != IDEntityMoveRot {
			t.Fatalf("jump move %d rendered 0x%02x, want relative", i, p.ID)
		}
	}
	if got := v2.pos[9][0]; got != 20 {
		t.Fatalf("saturated deltas did not converge: baseline x=%v, want 20", got)
	}
}

func TestPlayerInfoAndRemove(t *testing.T) {
	uuid := [16]byte{9, 9}
	props := []attach.Property{{Name: "textures", Value: "abc", Signature: "sig"}}
	eq(t, "info no props", PlayerInfoAdd(attach.PlayerInfo{UUID: uuid, Name: "Steve"}),
		IDPlayerInfo, oraclePlayerInfoAdd(uuid, "Steve", nil))
	eq(t, "info with skin", PlayerInfoAdd(attach.PlayerInfo{UUID: uuid, Name: "Steve", Props: props}),
		IDPlayerInfo, oraclePlayerInfoAdd(uuid, "Steve", props))
	eq(t, "player remove", PlayerRemove(attach.PlayerGone{UUID: uuid}),
		IDPlayerRemove, append(protocol.AppendVarInt(nil, 1), uuid[:]...))

	v := NewEntityView()
	eq(t, "head", Head(attach.EntityHead{EID: 5, Yaw: 180}),
		IDEntityHead, protocol.AppendU8(protocol.AppendVarInt(nil, 5), angleByte(180)))
	eq(t, "destroy", v.Remove(attach.EntityRemove{EIDs: []int32{5, 6}}),
		IDEntityDestroy, protocol.AppendVarInt(protocol.AppendVarInt(protocol.AppendVarInt(nil, 2), 5), 6))
}

// Render must dispatch every entity-family event and refuse anything else.
func TestRenderDispatch(t *testing.T) {
	v := NewEntityView()
	for _, ev := range []any{
		attach.EntityAdd{EID: 1}, attach.EntityMove{EID: 1}, attach.EntityHead{EID: 1},
		attach.EntityRemove{EIDs: []int32{1}}, attach.PlayerInfo{Name: "x"}, attach.PlayerGone{},
	} {
		if _, ok := v.Render(ev); !ok {
			t.Fatalf("Render refused %T", ev)
		}
	}
	if _, ok := v.Render(attach.Bye{Reason: "x"}); ok {
		t.Fatal("Render accepted a non-event value")
	}
}
