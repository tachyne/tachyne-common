package protocol

import (
	"bytes"
	"testing"
)

// A 26.2 client's left-click Attack packet (0x01, body = target VarInt) must
// translate to canonical use_entity (0x18) with mouse=1 so the server attacks.
func TestAttack776ToUseEntity(t *testing.T) {
	tr := TranslatorFor(776)
	body := AppendVarInt(nil, 27) // target = cow eid 27
	id, out, drop := tr.Serverbound(StatePlay, 0x01, body)
	if drop || id != 0x18 {
		t.Fatalf("Attack should map to use_entity 0x18, got id=0x%02x drop=%v", id, drop)
	}
	r := bytes.NewReader(out)
	target, _ := ReadVarInt(r)
	mouse, _ := ReadVarInt(r)
	if target != 27 || mouse != 1 {
		t.Fatalf("want target=27 mouse=1 (attack), got target=%d mouse=%d", target, mouse)
	}
}

// A 26.2 right-click Interact packet (0x1A) must map to use_entity with mouse=0
// (interact, not attack) so right-click no longer accidentally deals damage.
func TestInteract776ToUseEntity(t *testing.T) {
	tr := TranslatorFor(776)
	body := AppendVarInt(nil, 27)                           // target
	body = AppendVarInt(body, 0)                            // hand
	body = append(body, 0x81, 0x39, 0xb9, 0x97, 0xaa, 0x85) // hit-location lpVec3 (6 bytes)
	body = AppendBool(body, false)                          // usingSecondaryAction
	id, out, drop := tr.Serverbound(StatePlay, 0x1a, body)
	if drop || id != 0x18 {
		t.Fatalf("Interact should map to use_entity 0x18, got id=0x%02x drop=%v", id, drop)
	}
	r := bytes.NewReader(out)
	target, _ := ReadVarInt(r)
	mouse, _ := ReadVarInt(r)
	if target != 27 || mouse != 0 {
		t.Fatalf("want target=27 mouse=0 (interact), got target=%d mouse=%d", target, mouse)
	}
}

// A native 770 client must be unaffected (Identity translator, no rewrite).
func TestAttackPacketUntouchedOn770(t *testing.T) {
	tr := TranslatorFor(770)
	body := AppendVarInt(nil, 27)
	id, _, _ := tr.Serverbound(StatePlay, 0x18, body) // 770 sends use_entity directly
	if id != 0x18 {
		t.Fatalf("770 use_entity should pass through as 0x18, got 0x%02x", id)
	}
}

// A 26.2/26.3 middle click (pick_item_from_block 0x24, pick_item_from_entity
// 0x25) and paddle_boat (0x23) reach the canonical ids 0x22/0x23/0x21 with
// their bodies untouched.
func TestPickItem26xToCanonical(t *testing.T) {
	for _, v := range []int32{776, 777} {
		tr := TranslatorFor(v)
		for _, c := range []struct{ in, want int32 }{{0x23, 0x21}, {0x24, 0x22}, {0x25, 0x23}} {
			body := []byte{1, 2, 3, 4, 5, 6, 7, 8, 1}
			id, out, drop := tr.Serverbound(StatePlay, c.in, body)
			if drop || id != c.want || !bytes.Equal(out, []byte{1, 2, 3, 4, 5, 6, 7, 8, 1}) {
				t.Errorf("v%d 0x%02x → 0x%02x %v drop=%v, want 0x%02x unchanged", v, c.in, id, out, drop, c.want)
			}
		}
	}
}

// F3+F4 on 26.2/26.3 (change_game_mode, 0x05, a GameType VarInt) arrives as
// the /gamemode command it stands for.
func TestChangeGameModeBecomesTheCommand(t *testing.T) {
	for _, v := range []int32{776, 777} {
		id, out, drop := TranslatorFor(v).Serverbound(StatePlay, 0x05, AppendVarInt(nil, 1))
		if drop || id != canonChatCommand || !bytes.Equal(out, AppendString(nil, "gamemode creative")) {
			t.Errorf("v%d: id 0x%02x drop %v body %q", v, id, drop, out)
		}
		if _, _, drop := TranslatorFor(v).Serverbound(StatePlay, 0x05, AppendVarInt(nil, 9)); !drop {
			t.Errorf("v%d: a bad mode was not dropped", v)
		}
	}
}
