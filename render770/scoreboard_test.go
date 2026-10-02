package render770

// Strict re-parse of the scoreboard packets, mirroring the vanilla read
// paths — including set_player_team in BOTH forms (the ≤26.1 layout and the
// 26.2 reorder with Optional<TeamColor>).

import (
	"bytes"
	"encoding/json"
	"testing"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

func TestObjectiveScoreDisplayReparse(t *testing.T) {
	p := Objective(attach.Objective{Name: "kills", Method: attach.ObjAdd, Title: "Kills", Hearts: false})
	r := bytes.NewReader(p.Body)
	if s := rdString(t, r); s != "kills" {
		t.Fatalf("name %q", s)
	}
	if m, _ := r.ReadByte(); m != 0 {
		t.Fatalf("method %d", m)
	}
	if err := protocol.SkipNetworkNBT(r); err != nil {
		t.Fatalf("title: %v", err)
	}
	if rdVarInt(t, r) != 0 || rdBool(t, r) || r.Len() != 0 {
		t.Fatal("render type / number format / trailing")
	}

	p = DisplaySlot(attach.DisplaySlot{Slot: attach.SlotSidebar, Objective: "kills"})
	r = bytes.NewReader(p.Body)
	if rdVarInt(t, r) != 1 || rdString(t, r) != "kills" || r.Len() != 0 {
		t.Fatal("display slot wire")
	}

	p = Score(attach.Score{Owner: "wesley", Objective: "kills", Value: 7})
	r = bytes.NewReader(p.Body)
	if rdString(t, r) != "wesley" || rdString(t, r) != "kills" || rdVarInt(t, r) != 7 {
		t.Fatal("score fields")
	}
	if rdBool(t, r) || rdBool(t, r) || r.Len() != 0 {
		t.Fatal("score optionals should be absent")
	}

	p = Score(attach.Score{Owner: "wesley", Objective: "kills", Reset: true})
	if p.ID != IDResetScore {
		t.Fatalf("reset id 0x%x", p.ID)
	}
	r = bytes.NewReader(p.Body)
	if rdString(t, r) != "wesley" || !rdBool(t, r) || rdString(t, r) != "kills" || r.Len() != 0 {
		t.Fatal("reset wire")
	}
}

func teamFixture(method int32) attach.Team {
	return attach.Team{Name: "red", Method: method, Title: "Red Team",
		Prefix: "[R] ", Color: 12, FriendlyFire: true, Visibility: 0,
		Collision: 1, Players: []string{"wesley", "probe"}}
}

func TestPlayerTeamOldForm(t *testing.T) {
	p := PlayerTeam(teamFixture(attach.TeamAdd), 770)
	r := bytes.NewReader(p.Body)
	if rdString(t, r) != "red" {
		t.Fatal("name")
	}
	if m, _ := r.ReadByte(); m != 0 {
		t.Fatal("method")
	}
	protocol.SkipNetworkNBT(r)        // display
	if o, _ := r.ReadByte(); o != 1 { // friendly fire
		t.Fatalf("options %d", o)
	}
	if rdVarInt(t, r) != 0 || rdVarInt(t, r) != 1 {
		t.Fatal("visibility/collision")
	}
	if rdVarInt(t, r) != 12 { // ChatFormatting.RED ordinal
		t.Fatal("color")
	}
	protocol.SkipNetworkNBT(r) // prefix
	protocol.SkipNetworkNBT(r) // suffix
	if rdVarInt(t, r) != 2 || rdString(t, r) != "wesley" || rdString(t, r) != "probe" || r.Len() != 0 {
		t.Fatal("players")
	}
}

func TestPlayerTeamNewForm776(t *testing.T) {
	p := PlayerTeam(teamFixture(attach.TeamAdd), 776)
	r := bytes.NewReader(p.Body)
	rdString(t, r)
	r.ReadByte()
	protocol.SkipNetworkNBT(r) // display
	protocol.SkipNetworkNBT(r) // prefix
	protocol.SkipNetworkNBT(r) // suffix
	if rdVarInt(t, r) != 0 || rdVarInt(t, r) != 1 {
		t.Fatal("visibility/collision")
	}
	if !rdBool(t, r) || rdVarInt(t, r) != 12 { // Optional<TeamColor> present, RED
		t.Fatal("team color")
	}
	if o, _ := r.ReadByte(); o != 1 {
		t.Fatalf("options %d", o)
	}
	if rdVarInt(t, r) != 2 {
		t.Fatal("players count")
	}
	rdString(t, r)
	rdString(t, r)
	if r.Len() != 0 {
		t.Fatal("trailing")
	}
	// membership-only method carries no parameters in either form
	p = PlayerTeam(attach.Team{Name: "red", Method: attach.TeamAddPlayers,
		Players: []string{"x"}}, 776)
	r = bytes.NewReader(p.Body)
	rdString(t, r)
	if m, _ := r.ReadByte(); m != 3 {
		t.Fatal("method")
	}
	if rdVarInt(t, r) != 1 || rdString(t, r) != "x" || r.Len() != 0 {
		t.Fatal("players-only wire")
	}
}

// Number formats (NumberFormatTypes.OPTIONAL_STREAM_CODEC): present flag,
// type id (blank 0, styled 1, fixed 2), payload — none, a Style compound, a
// text component. Written out by hand as the byte oracle.
func TestNumberFormats(t *testing.T) {
	head := protocol.AppendString(nil, "w")
	head = protocol.AppendString(head, "kills")
	head = protocol.AppendVarInt(head, 7)
	head = protocol.AppendBool(head, false) // no display name
	for _, tc := range []struct {
		name string
		f    *attach.NumberFormat
		want []byte
	}{
		{"none", nil, []byte{0}},
		{"blank", &attach.NumberFormat{Kind: attach.NumberFormatBlank}, []byte{1, 0}},
		{"styled", &attach.NumberFormat{Kind: attach.NumberFormatStyled, Color: "red", Bold: true},
			[]byte{1, 1, 10, 8, 0, 5, 'c', 'o', 'l', 'o', 'r', 0, 3, 'r', 'e', 'd', 1, 0, 4, 'b', 'o', 'l', 'd', 1, 0}},
		{"fixed", &attach.NumberFormat{Kind: attach.NumberFormatFixed, Fixed: "MVP"}, []byte{1, 2, 8, 0, 3, 'M', 'V', 'P'}},
		{"unknown", &attach.NumberFormat{Kind: "sparkly"}, []byte{0}},
	} {
		eq(t, "score "+tc.name, Score(attach.Score{Owner: "w", Objective: "kills", Value: 7, Format: tc.f}),
			IDSetScore, append(append([]byte(nil), head...), tc.want...))
	}
	obj := protocol.AppendString(nil, "kills")
	obj = append(obj, 0) // add
	obj = append(obj, chatNBT("Kills")...)
	obj = protocol.AppendVarInt(obj, 0)
	eq(t, "objective fixed", Objective(attach.Objective{Name: "kills", Method: attach.ObjAdd, Title: "Kills",
		Format: &attach.NumberFormat{Kind: attach.NumberFormatBlank}}), IDSetObjective, append(obj, 1, 0))
}

// TestScoreDisplayName: set_score's display override
// (ComponentSerialization.TRUSTED_OPTIONAL_STREAM_CODEC) is a presence flag
// and the component as network NBT, before the number format. The frame
// goes through its JSON payload first — the gateway's entry path.
func TestScoreDisplayName(t *testing.T) {
	raw, err := json.Marshal(attach.Score{Owner: "w", Objective: "kills", Value: 7,
		Display: &attach.Text{Text: "Hi", Color: "red"}})
	if err != nil {
		t.Fatal(err)
	}
	var e attach.Score
	if err := json.Unmarshal(raw, &e); err != nil {
		t.Fatal(err)
	}
	want := protocol.AppendString(nil, "w")
	want = protocol.AppendString(want, "kills")
	want = protocol.AppendVarInt(want, 7)
	want = append(want, 1, // display present
		10,                                          // TAG_Compound (nameless root)
		8, 0, 4, 't', 'e', 'x', 't', 0, 2, 'H', 'i', // "text": "Hi"
		8, 0, 5, 'c', 'o', 'l', 'o', 'r', 0, 3, 'r', 'e', 'd', // "color": "red"
		0, // TAG_End
		0) // no number format
	eq(t, "score display", Score(e), IDSetScore, want)

	// The translated form, with a number format after it.
	e = attach.Score{Owner: "w", Objective: "kills", Value: 7,
		Display: &attach.Text{Translate: "a.b"}, Format: &attach.NumberFormat{Kind: attach.NumberFormatBlank}}
	want = protocol.AppendString(nil, "w")
	want = protocol.AppendString(want, "kills")
	want = protocol.AppendVarInt(want, 7)
	want = append(want, 1, 10, 8, 0, 9, 't', 'r', 'a', 'n', 's', 'l', 'a', 't', 'e', 0, 3, 'a', '.', 'b', 0, 1, 0)
	eq(t, "score display translate", Score(e), IDSetScore, want)
}
