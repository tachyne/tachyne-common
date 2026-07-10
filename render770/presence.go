package render770

// presence.go renders the presence-UI event family: chat / system messages,
// the action-bar overlay, boss bars, and the world clock.

import (
	"strings"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// Canonical-770 clientbound play packet IDs for this family.
const (
	IDBossBar    = 0x09
	IDUpdateTime = 0x6a
	IDSystemChat = 0x72
)

const dayLengthTicks = 24000

// chatNBT encodes plain text as a network-NBT text component — a nameless
// root TAG_String (type 8), which the client reads as {"text": s} — after
// sanitizing: capped at 256 runes, NUL and astral runes replaced (they break
// the client's modified-UTF8 reader).
func chatNBT(s string) []byte {
	var sb strings.Builder
	n := 0
	for _, r := range s {
		if n >= 256 {
			break
		}
		if r == 0 || r > 0xFFFF {
			r = '?'
		}
		sb.WriteRune(r)
		n++
	}
	s = sb.String()
	b := []byte{0x08, byte(len(s) >> 8), byte(len(s))}
	return append(b, s...)
}

// Chat renders a system message — a chat line, or the action-bar overlay.
func Chat(e attach.Chat) Packet {
	return Packet{IDSystemChat, protocol.AppendBool(chatNBT(e.Text), e.ActionBar)}
}

// BossBar renders a boss-bar operation. Color/style are the house constants
// (purple, solid) every boss uses.
func BossBar(e attach.BossBar) Packet {
	b := append([]byte(nil), e.UUID[:]...)
	switch e.Op {
	case attach.BossBarAdd:
		b = protocol.AppendVarInt(b, 0)
		b = append(b, chatNBT(e.Title)...)
		b = protocol.AppendF32(b, e.Health)
		b = protocol.AppendVarInt(b, 5) // color: purple
		b = protocol.AppendVarInt(b, 0) // style: solid
		b = protocol.AppendU8(b, 0)     // flags
	case attach.BossBarHealth:
		b = protocol.AppendVarInt(b, 2)
		b = protocol.AppendF32(b, e.Health)
	default: // BossBarRemove
		b = protocol.AppendVarInt(b, 1)
	}
	return Packet{IDBossBar, b}
}

// Time renders Update Time with tickDayTime=true (the client advances its own
// clock between sends). A zero Age falls back to the day time — sessions that
// only track a clock (solo mode, gateway join) have no world age.
func Time(e attach.Time) Packet {
	age := e.Age
	if age == 0 {
		age = e.Time
	}
	b := protocol.AppendI64(nil, age)
	b = protocol.AppendI64(b, e.Time%dayLengthTicks)
	return Packet{IDUpdateTime, protocol.AppendBool(b, true)}
}
