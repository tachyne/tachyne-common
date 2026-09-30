package attach

// Secure (signed) chat frames. The per-connection half of vanilla's secure
// chat — the player's message chain, the last-seen window it acknowledges,
// the signature cache that packs what it is sent — lives in the Java gateway,
// which is where ServerGamePacketListenerImpl keeps it. The world holds only
// what PlayerList does: each player's validated chat session (RemoteChatSession,
// which every client needs to verify that player's messages) and the relay of
// a signed message to every recipient (PlayerList.broadcastChatMessage with
// ChatType.bind(CHAT, sender)).

// FeaturePlayerChat is the Hello feature a gateway advertises when it renders
// MsgPlayerChat and MsgPlayerInfoChat. A session without it (the Bedrock
// gateway, an older Java gateway) is sent player chat as a Chat frame with a
// Sender, exactly as before.
const FeaturePlayerChat = "player_chat"

// MsgChatSession (gw→w): the player's chat session, validated by the gateway
// against the Mojang services key (ServerGamePacketListenerImpl
// handleChatSessionUpdate → resetPlayerChatState). The world stores it on the
// player and broadcasts it (player_info_update INITIALIZE_CHAT).
const MsgChatSession = 0xa0

// ChatSession is vanilla RemoteChatSession.Data: the session id and the
// profile public key (ProfilePublicKey.Data).
type ChatSession struct {
	SessionID [16]byte `json:"session_id"`
	ExpiresAt int64    `json:"expires_at"` // unix millis (ByteBufCodecs.INSTANT)
	Key       []byte   `json:"key"`        // X.509 SubjectPublicKeyInfo, as the client sent it
	KeySig    []byte   `json:"key_sig"`    // Mojang's signature over the key
}

// MsgPlayerInfoChat (w→gw): one listed player's chat session changed —
// player_info_update with INITIALIZE_CHAT alone, sent to everyone.
const MsgPlayerInfoChat = 0xa1

// PlayerInfoChat carries the player and the session (nil clears it).
type PlayerInfoChat struct {
	UUID    [16]byte     `json:"uuid"`
	Session *ChatSession `json:"session,omitempty"`
}

// SignedChat is the signed half of a player's chat message, riding the gw→w
// Chat frame (Chat.Text is the signed content). It is what the gateway's
// SignedMessageChain decoder produced: the message's link index in the
// sender's chain, its signature, and the signed body (timestamp, salt and
// the last-seen signatures the gateway resolved from the client's
// acknowledgement update).
type SignedChat struct {
	Index     int32    `json:"index"`
	Signature []byte   `json:"signature"`
	Timestamp int64    `json:"timestamp"` // unix millis
	Salt      int64    `json:"salt"`
	LastSeen  [][]byte `json:"last_seen,omitempty"` // 256-byte signatures, oldest first
}

// MsgPlayerChat (w→gw): a player's chat message for one recipient —
// ServerGamePacketListenerImpl.sendPlayerChatMessage. The recipient's gateway
// assigns the global index and packs the signatures against its own
// MessageSignatureCache, then tracks the signature as pending in the
// recipient's last-seen window.
const MsgPlayerChat = 0xa2

// PlayerChat is vanilla PlayerChatMessage plus its bound chat type. An
// unsigned message (no Signature) is what PlayerChatMessage.unsigned makes.
type PlayerChat struct {
	Sender     [16]byte `json:"sender"`
	SenderName string   `json:"sender_name"`
	Index      int32    `json:"index,omitempty"`
	Signature  []byte   `json:"signature,omitempty"`
	Content    string   `json:"content"`
	Timestamp  int64    `json:"timestamp"`
	Salt       int64    `json:"salt,omitempty"`
	LastSeen   [][]byte `json:"last_seen,omitempty"`
	// Unsigned is the decorated content when it differs from the signed
	// one (PlayerChatMessage.withUnsignedContent): a plugin rewrote it.
	Unsigned string `json:"unsigned,omitempty"`
	// ChatType is the chat_type registry name ("" = minecraft:chat).
	ChatType string `json:"chat_type,omitempty"`
	// Target is the bound type's target name (msg_command_*), "" = none.
	Target string `json:"target,omitempty"`
}

// MsgDeleteChat (w→gw): remove a signed message from the recipient's chat
// (ClientboundDeleteChatPacket). The recipient's gateway packs the signature
// against its cache.
const MsgDeleteChat = 0xa6

// DeleteChat names the message by its signature.
type DeleteChat struct {
	Signature []byte `json:"signature"`
}
