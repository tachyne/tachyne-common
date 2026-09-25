package attach

// MsgTransfer (w→gw): send the client to another server —
// ClientboundTransferPacket (/transfer).
const MsgTransfer = 0x8c

// Transfer is where to.
type Transfer struct {
	Host string `json:"host"`
	Port int32  `json:"port"`
}

// MsgCamera (w→gw): look through another entity's eyes — set_camera, which
// ServerPlayer.setCamera sends a spectator (/spectate, or a left-click on an
// entity in spectator mode). The player's own id returns the camera.
const MsgCamera = 0x8d

// Camera is the entity to look through.
type Camera struct {
	EID int32 `json:"eid"`
}

// MsgTeleportToEntity (gw→w): the spectator hotbar menu's teleport
// (ServerboundTeleportToEntityPacket).
const MsgTeleportToEntity = 0x8e

// TeleportToEntity names the entity by UUID.
type TeleportToEntity struct {
	UUID [16]byte `json:"uuid"`
}

// MsgTickingState (w→gw): the server's tick rate and whether the game is
// frozen (ClientboundTickingStatePacket, from /tick).
const MsgTickingState = 0x8f

// TickingState is /tick rate and /tick freeze.
type TickingState struct {
	Rate   float32 `json:"rate"`
	Frozen bool    `json:"frozen,omitempty"`
}

// MsgTickingStep (w→gw): frozen-game steps left (ClientboundTickingStepPacket,
// /tick step).
const MsgTickingStep = 0x90

// TickingStep is the number of steps.
type TickingStep struct {
	Steps int32 `json:"steps"`
}

// MsgTransientBlock (w→gw): a falling block has just landed here
// (ClientboundAddTransientBlockPacket, 26.3), sent with its block update so
// the client draws the block without a frame's gap. Older clients have no
// such packet and get the block update alone.
const MsgTransientBlock = 0x91

// TransientBlock is the landed block: position and canonical state.
type TransientBlock struct {
	X     int32 `json:"x"`
	Y     int32 `json:"y"`
	Z     int32 `json:"z"`
	State int32 `json:"state"`
}

// MsgGhostRecipe (w→gw): the player clicked a recipe they cannot make — the
// grid shows its ingredients as ghosts (ClientboundPlaceGhostRecipePacket,
// ServerGamePacketListenerImpl.handlePlaceRecipe's PLACE_GHOST_RECIPE).
// Exactly one of Shaped and Shapeless is set.
const MsgGhostRecipe = 0x92

// GhostRecipe is the window and the recipe's display.
type GhostRecipe struct {
	Window    int32            `json:"window"`
	Shaped    *ShapedRecipe    `json:"shaped,omitempty"`
	Shapeless *ShapelessRecipe `json:"shapeless,omitempty"`
}

// MsgPostEffects (w→gw): the player's post effects — the shader passes
// /posteffect puts on their screen (ClientboundPostEffectsPacket, 26.3).
const MsgPostEffects = 0x93

// PostEffects is the whole list, in order.
type PostEffects struct {
	Effects []string `json:"effects"`
}

// MsgSuggestReq (gw→w): the client wants completions for what it has typed
// (ServerboundCommandSuggestionPacket, for an ask_server argument).
const MsgSuggestReq = 0x94

// SuggestReq is the request: its transaction id and the text so far.
type SuggestReq struct {
	ID   int32  `json:"id"`
	Text string `json:"text"`
}

// MsgSuggestions (w→gw): the answer (ClientboundCommandSuggestionsPacket):
// the range of the text they replace, and the candidates.
const MsgSuggestions = 0x95

// Suggestions is the completions for one request.
type Suggestions struct {
	ID      int32    `json:"id"`
	Start   int32    `json:"start"`
	Length  int32    `json:"length"`
	Matches []string `json:"matches,omitempty"`
}
