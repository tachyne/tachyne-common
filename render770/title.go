package render770

import (
	"github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// Titles: the big words across the middle of the screen, their subtitle, and
// the fade/stay/fade the pair is shown with.
//
// The ids come from the packets either side of them, which this renderer
// already pins: set_passengers 0x64, set_player_team 0x66, set_score 0x67,
// set_time 0x6a and sound 0x6e. The clientbound play packets are registered
// alphabetically, so set_subtitle_text, set_title_text and
// set_titles_animation fall at 0x69, 0x6b and 0x6c between them.
const (
	IDSetSubtitleText   = 0x69
	IDSetTitleText      = 0x6b
	IDSetTitleAnimation = 0x6c
	IDClearTitles       = 0x0e
)

// TitlePackets renders one Title frame as the packets it needs: the timing
// first (vanilla sends it before the text, so the first frame of a title is
// already shown at the right speed), then the subtitle, then the title —
// which is what actually puts them on screen.
func TitlePackets(e attach.Title) []Packet {
	if e.Clear {
		return []Packet{{IDClearTitles, []byte{boolByte(e.Reset)}}}
	}
	var out []Packet
	if e.FadeIn != 0 || e.Stay != 0 || e.FadeOut != 0 {
		b := protocol.AppendI32(nil, e.FadeIn)
		b = protocol.AppendI32(b, e.Stay)
		out = append(out, Packet{IDSetTitleAnimation, protocol.AppendI32(b, e.FadeOut)})
	}
	if e.Subtitle != "" || len(e.SubtitleJSON) > 0 {
		out = append(out, Packet{IDSetSubtitleText, componentOr(e.SubtitleJSON, e.Subtitle)})
	}
	if e.Title != "" || len(e.TitleJSON) > 0 {
		out = append(out, Packet{IDSetTitleText, componentOr(e.TitleJSON, e.Title)})
	}
	return out
}

func boolByte(v bool) byte {
	if v {
		return 1
	}
	return 0
}

// componentOr is a JSON text component as network NBT, or the plain text
// when there is none (or it does not parse).
func componentOr(raw []byte, text string) []byte {
	if len(raw) > 0 {
		if nbt, ok := protocol.TextComponentNBT(raw); ok {
			return nbt
		}
	}
	return chatNBT(text)
}
