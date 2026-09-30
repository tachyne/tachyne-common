package render770

// securechat.go renders signed player chat and parses the secure-chat
// serverbound packets. Every layout here is the same at canonical 770 and at
// 26.2/26.3 (the chain only renumbers the ids), and follows the vanilla
// stream codecs: ClientboundPlayerChatPacket, ClientboundDeleteChatPacket,
// ClientboundPlayerInfoUpdatePacket INITIALIZE_CHAT, ServerboundChatPacket,
// ServerboundChatAckPacket, ServerboundChatCommandSignedPacket and
// ServerboundChatSessionUpdatePacket.

import (
	"bytes"
	"io"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// Canonical-770 ids of the secure-chat packets.
const (
	IDDeleteChat = 0x1b // clientbound delete_chat
	IDPlayerChat = 0x3a // clientbound player_chat

	SIDChatAck           = 0x04 // serverbound chat_ack
	SIDChatCommandSigned = 0x06 // serverbound chat_command_signed
	SIDChat              = 0x07 // serverbound chat
	SIDChatSessionUpdate = 0x08 // serverbound chat_session_update
)

// SignatureBytes is MessageSignature.BYTES.
const SignatureBytes = 256

// LastSeenMax is LastSeenMessages.LAST_SEEN_MESSAGES_MAX_LENGTH: the window a
// client acknowledges and the most entries a packed last-seen list carries.
const LastSeenMax = 20

// chatTypeIDs is the chat_type registry order (protocol.SyncedRegistries).
var chatTypeIDs = map[string]int32{
	"":                                    0,
	"minecraft:chat":                      0,
	"minecraft:emote_command":             1,
	"minecraft:msg_command_incoming":      2,
	"minecraft:msg_command_outgoing":      3,
	"minecraft:say_command":               4,
	"minecraft:team_msg_command_incoming": 5,
	"minecraft:team_msg_command_outgoing": 6,
}

// PackedSignature is MessageSignature.Packed: a recipient's cache index, or
// (ID -1) the full signature.
type PackedSignature struct {
	ID   int32
	Full []byte
}

func appendPackedSignature(b []byte, p PackedSignature) []byte {
	b = protocol.AppendVarInt(b, p.ID+1)
	if p.ID == -1 {
		b = append(b, p.Full...)
	}
	return b
}

// PlayerChat renders player_chat for one recipient. globalIndex is the
// recipient's running chat index (ServerGamePacketListenerImpl.nextChatIndex)
// and lastSeen the message's last-seen signatures packed against the
// recipient's signature cache (SignedMessageBody.pack).
//
// Layout: globalIndex VarInt · sender UUID · index VarInt · optional
// signature (bool + 256 bytes) · body (content string, timestamp i64 millis,
// salt i64, last-seen VarInt count + packed entries) · optional unsigned
// content (bool + component) · filter mask (VarInt 0 = pass-through) · bound
// chat type (registry ref index+1, name component, optional target).
func PlayerChat(globalIndex int32, e attach.PlayerChat, lastSeen []PackedSignature) Packet {
	b := protocol.AppendVarInt(nil, globalIndex)
	b = append(b, e.Sender[:]...)
	b = protocol.AppendVarInt(b, e.Index)
	signed := len(e.Signature) == SignatureBytes
	b = protocol.AppendBool(b, signed)
	if signed {
		b = append(b, e.Signature...)
	}
	b = protocol.AppendString(b, e.Content)
	b = protocol.AppendI64(b, e.Timestamp)
	b = protocol.AppendI64(b, e.Salt)
	b = protocol.AppendVarInt(b, int32(len(lastSeen)))
	for _, p := range lastSeen {
		b = appendPackedSignature(b, p)
	}
	b = protocol.AppendBool(b, e.Unsigned != "")
	if e.Unsigned != "" {
		b = append(b, chatNBT(e.Unsigned)...)
	}
	b = protocol.AppendVarInt(b, 0) // FilterMask PASS_THROUGH
	return Packet{IDPlayerChat, appendBoundChatType(b, e.ChatType, e.SenderName, e.Target)}
}

// appendBoundChatType writes ChatType.Bound: the type as a registry
// reference, the sender's name and the optional target name.
func appendBoundChatType(b []byte, chatType, name, target string) []byte {
	id, ok := chatTypeIDs[chatType]
	if !ok {
		id = 0
	}
	b = protocol.AppendVarInt(b, id+1)
	b = append(b, chatNBT(name)...)
	b = protocol.AppendBool(b, target != "")
	if target != "" {
		b = append(b, chatNBT(target)...)
	}
	return b
}

// DeleteChat renders delete_chat: the message's packed signature.
func DeleteChat(p PackedSignature) Packet {
	return Packet{IDDeleteChat, appendPackedSignature(nil, p)}
}

// appendChatSession writes RemoteChatSession.Data as a nullable field (the
// INITIALIZE_CHAT entry): present flag, session UUID, expiry (i64 millis),
// the X.509 key and Mojang's key signature, each a VarInt-prefixed array.
func appendChatSession(b []byte, s *attach.ChatSession) []byte {
	b = protocol.AppendBool(b, s != nil)
	if s == nil {
		return b
	}
	b = append(b, s.SessionID[:]...)
	b = protocol.AppendI64(b, s.ExpiresAt)
	b = protocol.AppendVarInt(b, int32(len(s.Key)))
	b = append(b, s.Key...)
	b = protocol.AppendVarInt(b, int32(len(s.KeySig)))
	return append(b, s.KeySig...)
}

// PlayerInfoChat renders player_info_update with INITIALIZE_CHAT alone
// (ServerGamePacketListenerImpl.resetPlayerChatState's broadcast).
func PlayerInfoChat(e attach.PlayerInfoChat) Packet {
	b := protocol.AppendU8(nil, 0x02)
	b = protocol.AppendVarInt(b, 1) // one entry
	b = append(b, e.UUID[:]...)
	return Packet{IDPlayerInfo, appendChatSession(b, e.Session)}
}

// LastSeenUpdate is LastSeenMessages.Update: how far the client advanced its
// window, which of the 20 slots it acknowledges (bit i = slot i, as
// BitSet.valueOf reads the three bytes) and the checksum of the resulting
// list (0 = ignore).
type LastSeenUpdate struct {
	Offset       int32
	Acknowledged uint32
	Checksum     byte
}

func readLastSeenUpdate(r *bytes.Reader) (LastSeenUpdate, error) {
	var u LastSeenUpdate
	off, err := protocol.ReadVarInt(r)
	if err != nil {
		return u, err
	}
	var bits [3]byte // ByteBufCodecs.fixedBitSet(20): ceil(20/8) bytes
	if _, err := io.ReadFull(r, bits[:]); err != nil {
		return u, err
	}
	sum, err := r.ReadByte()
	if err != nil {
		return u, err
	}
	u.Offset = off
	u.Acknowledged = uint32(bits[0]) | uint32(bits[1])<<8 | uint32(bits[2])<<16
	u.Checksum = sum
	return u, nil
}

// ChatMessage is ServerboundChatPacket.
type ChatMessage struct {
	Text      string
	Timestamp int64  // unix millis
	Salt      int64  // the signature salt
	Signature []byte // nil = unsigned
	LastSeen  LastSeenUpdate
}

// maxChatLength is ServerboundChatPacket's stringUtf8(256) bound.
const maxChatLength = 256

// ParseChatMessage reads serverbound chat.
func ParseChatMessage(data []byte) (ChatMessage, bool) {
	r := bytes.NewReader(data)
	var m ChatMessage
	text, err := protocol.ReadString(r)
	if err != nil || len([]rune(text)) > maxChatLength {
		return m, false
	}
	m.Text = text
	var n [16]byte
	if _, err := io.ReadFull(r, n[:]); err != nil {
		return m, false
	}
	m.Timestamp = int64(be64(n[:8]))
	m.Salt = int64(be64(n[8:]))
	has, err := r.ReadByte()
	if err != nil {
		return m, false
	}
	if has != 0 {
		sig := make([]byte, SignatureBytes)
		if _, err := io.ReadFull(r, sig); err != nil {
			return m, false
		}
		m.Signature = sig
	}
	if m.LastSeen, err = readLastSeenUpdate(r); err != nil {
		return m, false
	}
	return m, r.Len() == 0
}

// ArgumentSignature is one ArgumentSignatures.Entry: a signed command
// argument's name and its signature.
type ArgumentSignature struct {
	Name      string
	Signature []byte
}

// ChatCommandSigned is ServerboundChatCommandSignedPacket.
type ChatCommandSigned struct {
	Command   string
	Timestamp int64
	Salt      int64
	Arguments []ArgumentSignature
	LastSeen  LastSeenUpdate
}

// maxArgumentSignatures is ArgumentSignatures.MAX_ARGUMENT_COUNT.
const maxArgumentSignatures = 8

// ParseChatCommandSigned reads serverbound chat_command_signed.
func ParseChatCommandSigned(data []byte) (ChatCommandSigned, bool) {
	r := bytes.NewReader(data)
	var c ChatCommandSigned
	cmd, err := protocol.ReadString(r)
	if err != nil {
		return c, false
	}
	c.Command = cmd
	var n [16]byte
	if _, err := io.ReadFull(r, n[:]); err != nil {
		return c, false
	}
	c.Timestamp = int64(be64(n[:8]))
	c.Salt = int64(be64(n[8:]))
	count, err := protocol.ReadVarInt(r)
	if err != nil || count < 0 || count > maxArgumentSignatures {
		return c, false
	}
	for i := int32(0); i < count; i++ {
		name, err := protocol.ReadString(r)
		if err != nil {
			return c, false
		}
		sig := make([]byte, SignatureBytes)
		if _, err := io.ReadFull(r, sig); err != nil {
			return c, false
		}
		c.Arguments = append(c.Arguments, ArgumentSignature{Name: name, Signature: sig})
	}
	if c.LastSeen, err = readLastSeenUpdate(r); err != nil {
		return c, false
	}
	return c, r.Len() == 0
}

// ParseChatAck reads serverbound chat_ack: the offset alone.
func ParseChatAck(data []byte) (int32, bool) {
	r := bytes.NewReader(data)
	off, err := protocol.ReadVarInt(r)
	return off, err == nil && r.Len() == 0
}

// Key-size bounds from the stream codecs: ByteBufCodecs.PUBLIC_KEY is
// byteArray(512), the key signature byteArray(4096).
const (
	maxPublicKeyBytes = 512
	maxKeySigBytes    = 4096
)

// ParseChatSessionUpdate reads serverbound chat_session_update
// (RemoteChatSession.Data).
func ParseChatSessionUpdate(data []byte) (attach.ChatSession, bool) {
	r := bytes.NewReader(data)
	var s attach.ChatSession
	if _, err := io.ReadFull(r, s.SessionID[:]); err != nil {
		return s, false
	}
	var t [8]byte
	if _, err := io.ReadFull(r, t[:]); err != nil {
		return s, false
	}
	s.ExpiresAt = int64(be64(t[:]))
	key, ok := readBoundedArray(r, maxPublicKeyBytes)
	if !ok {
		return s, false
	}
	sig, ok := readBoundedArray(r, maxKeySigBytes)
	if !ok {
		return s, false
	}
	s.Key, s.KeySig = key, sig
	return s, r.Len() == 0
}

func readBoundedArray(r *bytes.Reader, max int) ([]byte, bool) {
	n, err := protocol.ReadVarInt(r)
	if err != nil || n < 0 || int(n) > max || int(n) > r.Len() {
		return nil, false
	}
	out := make([]byte, n)
	_, err = io.ReadFull(r, out)
	return out, err == nil
}

func be64(b []byte) uint64 {
	return uint64(b[0])<<56 | uint64(b[1])<<48 | uint64(b[2])<<40 | uint64(b[3])<<32 |
		uint64(b[4])<<24 | uint64(b[5])<<16 | uint64(b[6])<<8 | uint64(b[7])
}
