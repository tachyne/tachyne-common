package render770

// Strict re-parse of the scoreboard packets, mirroring the vanilla read
// paths — including set_player_team in BOTH forms (the ≤26.1 layout and the
// 26.2 reorder with Optional<TeamColor>).

import (
	"bytes"
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
