package attach

import "encoding/json"

// Dialog frames (1.21.6+ dialogs; served to 26.2 and 26.3). The world opens
// and closes a player's dialog (ServerPlayer.openDialog, /dialog clear) and
// hears the custom click actions a dialog or a text component's click event
// sends back (MinecraftServer.handleCustomClickAction).

// FeatureDialog is the Hello feature of a gateway that renders
// MsgShowDialog/MsgClearDialog. A session without it is not sent them.
const FeatureDialog = "dialog"

// MsgShowDialog (w→gw): open a dialog on the player's screen
// (ClientboundShowDialogPacket).
const MsgShowDialog = 0xa3

// ShowDialog is a Holder<Dialog>: a registry dialog by id (Ref, e.g.
// "minecraft:server_links"), or a direct one as its JSON (the Dialog codec's
// data: {"type": "minecraft:notice", "title": …}).
type ShowDialog struct {
	Ref    string          `json:"ref,omitempty"`
	Dialog json.RawMessage `json:"dialog,omitempty"`
}

// MsgClearDialog (w→gw): close whatever dialog is open
// (ClientboundClearDialogPacket).
const MsgClearDialog = 0xa4

// ClearDialog carries nothing.
type ClearDialog struct{}

// MsgCustomClickAction (gw→w): the client ran a custom click action
// (ServerboundCustomClickActionPacket): a dialog action of type
// minecraft:custom / dynamic/custom, or a text component's custom click
// event.
const MsgCustomClickAction = 0xa5

// CustomClickAction is the action's id and its payload tag as JSON (null or
// absent = none; a dialog's dynamic/custom action sends its inputs as a
// compound).
type CustomClickAction struct {
	ID      string          `json:"id"`
	Payload json.RawMessage `json:"payload,omitempty"`
}
