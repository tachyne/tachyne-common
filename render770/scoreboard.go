package render770

// scoreboard.go renders the five vanilla scoreboard packets from the domain
// frames. Layouts are identical from 1.21.5 through 26.2 EXCEPT
// set_player_team: 26.2 reordered the parameters (displayName, prefix,
// suffix, visibility, collision, Optional<TeamColor>, options) from the
// older (displayName, options, visibility, collision, color enum, prefix,
// suffix) — verified against the 26.1 codec, which still uses the old form.
// PlayerTeam therefore composes at the client's real version, RecipeBook-
// style; the other four ride the ordinary chain (ids renumber, bodies pass).

import (
	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// Canonical-770 clientbound ids.
const (
	IDSetObjective     = 0x63
	IDDisplayObjective = 0x5b
	IDSetScore         = 0x67
	IDResetScore       = 0x48
	IDSetPlayerTeam    = 0x66
)

// teamParamsNewForm is the first protocol using the 26.2 parameter layout.
const teamParamsNewForm = 776

// Objective renders set_objective. Number format is always absent (default
// styling), matching vanilla servers without format overrides.
func Objective(e attach.Objective) Packet {
	b := protocol.AppendString(nil, e.Name)
	b = protocol.AppendU8(b, uint8(e.Method))
	if e.Method == attach.ObjAdd || e.Method == attach.ObjUpdate {
		b = append(b, chatNBT(e.Title)...)
		render := int32(0) // integer
		if e.Hearts {
			render = 1
		}
		b = protocol.AppendVarInt(b, render)
		b = protocol.AppendBool(b, false) // no number format override
	}
	return Packet{IDSetObjective, b}
}

// DisplaySlot renders set_display_objective ("" objective clears the slot).
func DisplaySlot(e attach.DisplaySlot) Packet {
	b := protocol.AppendVarInt(nil, e.Slot)
	b = protocol.AppendString(b, e.Objective)
	return Packet{IDDisplayObjective, b}
}

// Score renders set_score, or reset_score when the frame says so.
func Score(e attach.Score) Packet {
	if e.Reset {
		b := protocol.AppendString(nil, e.Owner)
		b = protocol.AppendBool(b, e.Objective != "")
		if e.Objective != "" {
			b = protocol.AppendString(b, e.Objective)
		}
		return Packet{IDResetScore, b}
	}
	b := protocol.AppendString(nil, e.Owner)
	b = protocol.AppendString(b, e.Objective)
	b = protocol.AppendVarInt(b, e.Value)
	b = protocol.AppendBool(b, false) // no display-name override
	b = protocol.AppendBool(b, false) // no number format override
	return Packet{IDSetScore, b}
}

// PlayerTeam renders set_player_team for the client's protocol version.
func PlayerTeam(e attach.Team, version int32) Packet {
	b := protocol.AppendString(nil, e.Name)
	b = protocol.AppendU8(b, uint8(e.Method))
	if e.Method == attach.TeamAdd || e.Method == attach.TeamUpdate {
		var options uint8
		if e.FriendlyFire {
			options |= 1
		}
		if e.SeeInvisible {
			options |= 2
		}
		if version >= teamParamsNewForm {
			b = append(b, chatNBT(e.Title)...)
			b = append(b, chatNBT(e.Prefix)...)
			b = append(b, chatNBT(e.Suffix)...)
			b = protocol.AppendVarInt(b, e.Visibility)
			b = protocol.AppendVarInt(b, e.Collision)
			hasColor := e.Color >= 0 && e.Color <= 15
			b = protocol.AppendBool(b, hasColor)
			if hasColor {
				b = protocol.AppendVarInt(b, e.Color) // TeamColor ids = color ordinals
			}
			b = protocol.AppendU8(b, options)
		} else {
			b = append(b, chatNBT(e.Title)...)
			b = protocol.AppendU8(b, options)
			b = protocol.AppendVarInt(b, e.Visibility)
			b = protocol.AppendVarInt(b, e.Collision)
			color := e.Color
			if color < 0 || color > 15 {
				color = 21 // ChatFormatting.RESET = no team color
			}
			b = protocol.AppendVarInt(b, color)
			b = append(b, chatNBT(e.Prefix)...)
			b = append(b, chatNBT(e.Suffix)...)
		}
	}
	if e.Method == attach.TeamAdd || e.Method == attach.TeamAddPlayers || e.Method == attach.TeamRemovePlayers {
		b = protocol.AppendVarInt(b, int32(len(e.Players)))
		for _, p := range e.Players {
			b = protocol.AppendString(b, p)
		}
	}
	return Packet{IDSetPlayerTeam, b}
}
