package protocol

import (
	"bytes"
	"io"
	"testing"
)

func TestChainSupportedRange(t *testing.T) {
	if TranslatorFor(771) == nil {
		t.Error("771 should be served by the chain")
	}
	if TranslatorFor(772) == nil {
		t.Error("772 should be served by the chain (771→772 is a no-op step)")
	}
	if TranslatorFor(777) != nil {
		t.Error("777 is above MaxTranslated — should be rejected")
	}
	if got := TranslatorFor(772); got.Version() != 772 {
		t.Errorf("chain Version() = %d, want 772", got.Version())
	}
}

func TestChainServerboundRemap(t *testing.T) {
	tr := TranslatorFor(771)
	// 770→771 renumbered serverbound play ids; 771 id 5 maps down to canonical 4
	// (from the generated sbDown table). The client speaks 771, the server 770.
	id, _, drop := tr.Serverbound(StatePlay, 5, []byte{0xAB})
	if drop {
		t.Fatal("id 5 should not be dropped")
	}
	if id != 4 {
		t.Errorf("Serverbound 771 id 5 → %d, want canonical 4", id)
	}
	// A genuinely new 771 serverbound packet (no 770 equivalent) is dropped.
	if _, _, drop := tr.Serverbound(StatePlay, 4, nil); !drop {
		t.Error("new-in-771 serverbound id 4 should be dropped")
	}
}

func TestChainClientboundIdentityFor771(t *testing.T) {
	// 770→771 has no clientbound ID changes, so a clientbound packet keeps its id.
	tr := TranslatorFor(771)
	id, body, drop := tr.Clientbound(StatePlay, 0x27, []byte{1, 2, 3})
	if drop || id != 0x27 {
		t.Errorf("clientbound 771 id 0x27 → (%#x, drop=%v), want unchanged", id, drop)
	}
	if len(body) != 3 {
		t.Errorf("body unexpectedly changed: %v", body)
	}
}

func TestChainLoginUnchanged(t *testing.T) {
	// Login state is identical 770↔772, so a 772 client's login flow needs no
	// translation (this is why login_finished decodes fine for 772, unlike 776).
	tr := TranslatorFor(772)
	const loginFinished = 0x02 // clientbound login_finished / Login Success
	id, _, drop := tr.Clientbound(StateLogin, loginFinished, []byte{9})
	if drop || id != loginFinished {
		t.Errorf("login_finished (0x02) should pass through unchanged for 772, got %#x drop=%v", id, drop)
	}
}

func TestKnownPacksVersionRewrite(t *testing.T) {
	mk := func(ver string) []byte {
		b := AppendVarInt(nil, 1)
		b = AppendString(b, "minecraft")
		b = AppendString(b, "core")
		return AppendString(b, ver)
	}
	readVer := func(body []byte) string {
		r := bytes.NewReader(body)
		ReadVarInt(r)
		ReadString(r)
		ReadString(r)
		v, _ := ReadString(r)
		return v
	}
	for _, c := range []struct {
		ver  int32
		want string
	}{{771, "1.21.6"}, {772, "1.21.8"}} {
		tr := TranslatorFor(c.ver)
		_, out, drop := tr.Clientbound(StateConfiguration, cfgKnownPacksID, mk("1.21.5"))
		if drop {
			t.Fatalf("known packs dropped for %d", c.ver)
		}
		if got := readVer(out); got != c.want {
			t.Errorf("client %d: known-packs version = %q, want %q", c.ver, got, c.want)
		}
	}
}

func TestSpawnEntityReorder772to773(t *testing.T) {
	// Build a 772-format spawn_entity: id,uuid,type,x,y,z,pitch,yaw,headPitch,objectData,vel(6)
	body := AppendVarInt(nil, 42) // entityId
	uuid := make([]byte, 16)
	uuid[0] = 0xAB
	body = append(body, uuid...)
	body = AppendVarInt(body, 7)          // type
	body = AppendF64(body, 1.0)           // x
	body = AppendF64(body, 2.0)           // y
	body = AppendF64(body, 3.0)           // z
	body = append(body, 10, 20, 30)       // pitch,yaw,headPitch
	body = AppendVarInt(body, 99)         // objectData
	body = append(body, 0, 0, 0, 0, 0, 0) // vec3i16 velocity = 0

	out := rewriteSpawnEntity772to773(StatePlay, body)

	// Decode as 773: id,uuid,type,x,y,z,velocity(lpVec3=0x00),pitch,yaw,headPitch,objectData
	r := bytes.NewReader(out)
	if v, _ := ReadVarInt(r); v != 42 {
		t.Errorf("entityId=%d, want 42", v)
	}
	got := make([]byte, 16)
	io.ReadFull(r, got)
	if got[0] != 0xAB {
		t.Error("uuid corrupted")
	}
	if v, _ := ReadVarInt(r); v != 7 {
		t.Errorf("type=%d, want 7", v)
	}
	xyz := make([]byte, 24)
	io.ReadFull(r, xyz)
	vel, _ := r.ReadByte()
	if vel != 0x00 {
		t.Errorf("lpVec3 velocity byte = %#x, want 0x00", vel)
	}
	ang := make([]byte, 3)
	io.ReadFull(r, ang)
	if ang[0] != 10 || ang[1] != 20 || ang[2] != 30 {
		t.Errorf("angles reordered wrong: %v", ang)
	}
	if v, _ := ReadVarInt(r); v != 99 {
		t.Errorf("objectData=%d, want 99", v)
	}
	if r.Len() != 0 {
		t.Errorf("%d trailing bytes", r.Len())
	}
	// 773 body should be 6 bytes shorter than 772 (6-byte vel -> 1-byte lpVec3 ... minus reorder no change; net -5)
	if len(out) != len(body)-5 {
		t.Errorf("773 body len=%d, 772 len=%d (want -5)", len(out), len(body))
	}
}

func TestChainServesThrough776(t *testing.T) {
	for _, v := range []int32{773, 774, 775, 776} {
		if TranslatorFor(v) == nil {
			t.Errorf("%d should be served", v)
		}
	}
	if TranslatorFor(777) != nil {
		t.Error("777 is above MaxTranslated — should be rejected")
	}
}
