package render770

import (
	"bytes"
	"encoding/json"
	"testing"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

func TestDialogIDs(t *testing.T) {
	if ShowDialogID(776) != 0x8c || ClearDialogID(776) != 0x8b || ShowDialogID(777) != 0x8f || ClearDialogID(777) != 0x8e {
		t.Fatal("play dialog ids")
	}
	if ConfigShowDialogID(776) != 0x12 || ConfigClearDialogID(776) != 0x11 || ConfigShowDialogID(777) != 0x13 || ConfigClearDialogID(777) != 0x12 {
		t.Fatal("configuration dialog ids")
	}
}

// show_dialog's holder: a registry reference is id+1 (server_links is the
// third declared entry), a direct dialog 0 then its network NBT.
func TestShowDialogBody(t *testing.T) {
	for ref, want := range map[string]byte{"minecraft:custom_options": 1, "quick_actions": 2, "minecraft:server_links": 3} {
		body, ok := ShowDialogBody(attach.ShowDialog{Ref: ref})
		if !ok || !bytes.Equal(body, []byte{want}) {
			t.Errorf("%s = % x (%v), want %d", ref, body, ok, want)
		}
	}
	if _, ok := ShowDialogBody(attach.ShowDialog{Ref: "minecraft:nope"}); ok {
		t.Error("an unknown dialog rendered")
	}
	d := json.RawMessage(`{"type":"minecraft:notice","title":"Hello"}`)
	body, ok := ShowDialogBody(attach.ShowDialog{Dialog: d})
	nbt, _ := protocol.JSONToNetworkNBT(d)
	if !ok || body[0] != 0 || !bytes.Equal(body[1:], nbt) {
		t.Fatalf("direct dialog = % x", body)
	}
	// Strict re-parse: the NBT after the holder byte reads back as the dialog.
	js, err := protocol.NetworkNBTToJSON(bytes.NewReader(body[1:]))
	if err != nil || string(js) != `{"title":"Hello","type":"minecraft:notice"}` {
		t.Fatalf("re-read %s (%v)", js, err)
	}
	if cfg, ok := ConfigShowDialogBody(d); !ok || !bytes.Equal(cfg, nbt) {
		t.Fatalf("configuration form = % x", cfg)
	}
}

func TestParseCustomClickAction(t *testing.T) {
	tag := []byte{10, 8, 0, 4, 'n', 'a', 'm', 'e', 0, 3, 'B', 'o', 'b', 1, 0, 2, 'o', 'k', 1, 0}
	b := protocol.AppendString(nil, "myplugin:submit")
	b = protocol.AppendVarInt(b, int32(len(tag)))
	b = append(b, tag...)
	e, ok := ParseCustomClickAction(b)
	if !ok || e.ID != "myplugin:submit" || string(e.Payload) != `{"name":"Bob","ok":1}` {
		t.Fatalf("parsed %+v (%s) ok=%v", e, e.Payload, ok)
	}
	// No payload: a lone End tag; a bare path takes the minecraft namespace.
	none := append(protocol.AppendString(nil, "hello"), 1, 0)
	if e, ok := ParseCustomClickAction(none); !ok || e.ID != "minecraft:hello" || e.Payload != nil {
		t.Fatalf("no payload: %+v ok=%v", e, ok)
	}
	if _, ok := ParseCustomClickAction(append(protocol.AppendString(nil, "Bad Id"), 1, 0)); ok {
		t.Fatal("an invalid identifier parsed")
	}
	if _, ok := ParseCustomClickAction(none[:len(none)-1]); ok {
		t.Fatal("a truncated action parsed")
	}
}
