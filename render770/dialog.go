package render770

// dialog.go renders the dialog packets and parses custom_click_action. None
// of them exists at canonical 770 (dialogs are 1.21.6+), so each is composed
// at the client's own id and sent raw, and the serverbound action is read
// before the chain (which has nothing to lower it to). The layouts are the
// same on 26.2 and 26.3; only the ids moved.

import (
	"bytes"
	"io"
	"strings"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// Dialog packet ids on 26.2 (776) and 26.3 (777), from the servers' packet
// reports.
const (
	IDClearDialog776 = 0x8b
	IDShowDialog776  = 0x8c
	IDClearDialog777 = 0x8e
	IDShowDialog777  = 0x8f

	IDConfigClearDialog776 = 0x11
	IDConfigShowDialog776  = 0x12
	IDConfigClearDialog777 = 0x12
	IDConfigShowDialog777  = 0x13

	// SIDCustomClickAction26x is serverbound custom_click_action in play on
	// 26.2 and 26.3; SIDConfigCustomClickAction26x the same in configuration.
	SIDCustomClickAction26x       = 0x44
	SIDConfigCustomClickAction26x = 0x08
)

// DialogMinProto is the first protocol with dialogs this server serves.
const DialogMinProto = 776

// ShowDialogID is play-state show_dialog at a client version.
func ShowDialogID(version int32) int32 {
	if version >= 777 {
		return IDShowDialog777
	}
	return IDShowDialog776
}

// ClearDialogID is play-state clear_dialog at a client version.
func ClearDialogID(version int32) int32 {
	if version >= 777 {
		return IDClearDialog777
	}
	return IDClearDialog776
}

// ConfigShowDialogID and ConfigClearDialogID are the configuration-state ids.
func ConfigShowDialogID(version int32) int32 {
	if version >= 777 {
		return IDConfigShowDialog777
	}
	return IDConfigShowDialog776
}

func ConfigClearDialogID(version int32) int32 {
	if version >= 777 {
		return IDConfigClearDialog777
	}
	return IDConfigClearDialog776
}

// ShowDialogBody is play-state show_dialog's body: Dialog.STREAM_CODEC, a
// holder — a registry reference as its id+1, or 0 then the dialog as network
// NBT (fromCodecWithRegistriesTrusted). ok is false for an unknown reference
// or a dialog that is not JSON.
func ShowDialogBody(e attach.ShowDialog) ([]byte, bool) {
	return ShowDialogBodyWith(e, protocol.DialogRegistryID)
}

// ShowDialogBodyWith is ShowDialogBody resolving a reference with the
// dialog registry the client was configured with (a data pack's dialogs
// included: protocol.DialogRegistryIDWith).
func ShowDialogBodyWith(e attach.ShowDialog, registryID func(string) (int32, bool)) ([]byte, bool) {
	if e.Ref != "" {
		id, ok := registryID(qualify(e.Ref))
		if !ok {
			return nil, false
		}
		return protocol.AppendVarInt(nil, id+1), true
	}
	nbt, ok := protocol.JSONToNetworkNBT(e.Dialog)
	if !ok {
		return nil, false
	}
	return append(protocol.AppendVarInt(nil, 0), nbt...), true
}

// ConfigShowDialogBody is the configuration-state form
// (CONTEXT_FREE_STREAM_CODEC): always direct, the NBT alone.
func ConfigShowDialogBody(dialog []byte) ([]byte, bool) {
	return protocol.JSONToNetworkNBT(dialog)
}

// qualify gives an identifier its default namespace.
func qualify(id string) string {
	if strings.Contains(id, ":") {
		return id
	}
	return "minecraft:" + id
}

// validIdentifier is Identifier's character rule: [a-z0-9_.-] in the
// namespace, and '/' also in the path.
func validIdentifier(id string) bool {
	ns, path, ok := strings.Cut(id, ":")
	if !ok || ns == "" || path == "" {
		return false
	}
	for _, c := range ns {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '.' || c == '-') {
			return false
		}
	}
	for _, c := range path {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '.' || c == '-' || c == '/') {
			return false
		}
	}
	return true
}

// maxClickPayload is the payload's lengthPrefixed(65536) bound.
const maxClickPayload = 65536

// ParseCustomClickAction reads custom_click_action: an identifier, then the
// payload as a length-prefixed optional tag (a lone End byte = none).
func ParseCustomClickAction(data []byte) (attach.CustomClickAction, bool) {
	r := bytes.NewReader(data)
	var e attach.CustomClickAction
	id, err := protocol.ReadString(r)
	if err != nil {
		return e, false
	}
	e.ID = qualify(id)
	if !validIdentifier(e.ID) {
		return e, false
	}
	n, err := protocol.ReadVarInt(r)
	if err != nil || n < 1 || n > maxClickPayload || int(n) > r.Len() {
		return e, false
	}
	tag := make([]byte, n)
	if _, err := io.ReadFull(r, tag); err != nil || r.Len() != 0 {
		return e, false
	}
	if tag[0] != 0 {
		js, err := protocol.NetworkNBTToJSON(bytes.NewReader(tag))
		if err != nil {
			return e, false
		}
		e.Payload = js
	}
	return e, true
}
