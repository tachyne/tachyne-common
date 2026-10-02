package gwsession

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net"
	"testing"
	"time"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
	"github.com/tachyne/tachyne-common/render770"
)

// moonConfig is a world with a fourth dimension of its own type and a data
// pack's painting with a tag naming it.
func moonConfig() attach.ConfigData {
	dims := []attach.DimensionInfo{
		{ID: 0, Key: "minecraft:overworld", SkyLight: true, Clock: "minecraft:overworld"},
		{ID: 1, Key: "minecraft:the_nether"},
		{ID: 2, Key: "minecraft:the_end", Clock: "minecraft:the_end"},
		{ID: 3, Key: "tachyne:moon", Type: "tachyne:moon", TypeData: json.RawMessage(`{"height":384,"min_y":-64,"has_skylight":true}`), SkyLight: true},
	}
	return attach.ConfigData{
		Dimensions: dims,
		Registries: []attach.RegistryEntries{{Registry: "minecraft:painting_variant", Entries: []attach.RegistryEntry{
			{Name: "tachyne:sunset", Data: json.RawMessage(`{"asset_id":"tachyne:sunset","width":2,"height":1}`)},
		}}},
		Tags: []attach.TagSet{{Registry: "minecraft:painting_variant", Tags: []attach.Tag{{Name: "minecraft:placeable", Entries: []string{"tachyne:sunset"}}}}},
	}
}

// The login packet lists the world's levels in its table's order and spawns
// in Welcome.Dim with that dimension's type id.
func TestJoinPacketDimensionTable(t *testing.T) {
	c := moonConfig()
	b := joinPacket(5, 0, 8, &attach.DeathPos{Dim: 3, X: 1, Y: 2, Z: 3}, false, attach.Welcome{Dim: 3, Config: &c})
	r := bytes.NewReader(b)
	r.Seek(5, 0) // eid, hardcore
	n, _ := protocol.ReadVarInt(r)
	var keys []string
	for i := int32(0); i < n; i++ {
		k, _ := protocol.ReadString(r)
		keys = append(keys, k)
	}
	if len(keys) != 4 || keys[3] != "tachyne:moon" || keys[1] != "minecraft:the_nether" {
		t.Fatalf("levels %v", keys)
	}
	protocol.ReadVarInt(r) // max players
	protocol.ReadVarInt(r) // view
	protocol.ReadVarInt(r) // simulation
	r.Seek(3, 1)           // the three flags
	if id, _ := protocol.ReadVarInt(r); id != 4 {
		t.Fatalf("dimension type id %d, want 4 (appended after the four built-in types)", id)
	}
	if k, _ := protocol.ReadString(r); k != "tachyne:moon" {
		t.Fatalf("spawn level %q", k)
	}
	r.Seek(8+1+1+1+1, 1) // seed, gamemode, previous, debug, flat
	if has, _ := r.ReadByte(); has != 1 {
		t.Fatal("no death location")
	}
	if k, _ := protocol.ReadString(r); k != "tachyne:moon" {
		t.Fatalf("death level %q", k)
	}
	// The default table keeps the historical order.
	if got := joinLevelKeys(nil); len(got) != 3 || got[1] != "minecraft:the_end" {
		t.Fatalf("default levels %v", got)
	}
	// Respawn into the moon: its type id and key.
	p := render770.RespawnIn(attach.Dimension{Dim: 3}, welcomeDims(attach.Welcome{Config: &c}))
	rr := bytes.NewReader(p.Body)
	if id, _ := protocol.ReadVarInt(rr); id != 4 {
		t.Fatalf("respawn type id %d", id)
	}
	if k, _ := protocol.ReadString(rr); k != "tachyne:moon" {
		t.Fatalf("respawn level %q", k)
	}
}

// readCfg reads one configuration packet the gateway wrote.
func readCfg(t *testing.T, br *bufio.Reader) *protocol.Packet {
	t.Helper()
	p, err := protocol.ReadCompressed(br)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// The reconfiguration round trip over a pipe, as the client sees it:
// start_configuration, then no play packet; the configuration phase again
// with the world's data (the fourth dimension's type inline, the pack's
// painting, its tag); after finish_configuration still nothing until the
// rejoin's login, which reopens play.
func TestReconfigurationRoundTrip(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	server.SetDeadline(time.Now().Add(20 * time.Second))
	client.SetDeadline(time.Now().Add(20 * time.Second))
	tr := protocol.TranslatorFor(776)
	cc := &clientConn{c: server, tr: tr, entTypes: map[int32]int32{}, menus: map[int32]int32{}}
	cbr := bufio.NewReader(client)

	raw, _ := json.Marshal(attach.StartConfiguration{ConfigData: moonConfig()})
	var e attach.StartConfiguration
	if err := json.Unmarshal(raw, &e); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cc.startConfiguration(e) }()
	p := readCfg(t, cbr)
	if want, _, _ := tr.Clientbound(protocol.StatePlay, playClientStartConfig, nil); p.ID != want || len(p.Data) != 0 {
		t.Fatalf("start_configuration: id 0x%x len %d", p.ID, len(p.Data))
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := cc.send(playClientGameEvent, []byte{13, 0, 0, 0, 0}); err != nil || !cc.configuring() {
		t.Fatal("a play packet was not held back")
	}
	if cc.startConfiguration(e) != nil {
		t.Fatal("a second request")
	}

	sc := cc.takeConfig()
	if sc == nil || len(sc.Dimensions) != 4 {
		t.Fatalf("pending configuration %+v", sc)
	}
	if cc.takeConfig() != nil {
		t.Fatal("the configuration was handed out twice")
	}
	sbr := bufio.NewReader(server)
	go func() {
		_, err := configure(Config{}, sbr, server, tr, 384, 776, configExtras(&sc.ConfigData))
		done <- err
	}()
	if p := readCfg(t, cbr); p.ID != cfgClientKnownPacks {
		t.Fatalf("first configuration packet 0x%x", p.ID)
	}
	kp := protocol.AppendVarInt(nil, 0)
	if err := protocol.WriteCompressed(client, cfgServerKnownPacks, kp, compressThreshold); err != nil {
		t.Fatal(err)
	}
	var sawMoon, sawSunset, sawTags bool
	for {
		p := readCfg(t, cbr)
		if p.ID == cfgClientFinish {
			break
		}
		switch p.ID {
		case cfgClientRegistryData:
			r := bytes.NewReader(p.Data)
			id, _ := protocol.ReadString(r)
			if id == "minecraft:dimension_type" && bytes.Contains(p.Data, []byte("tachyne:moon")) {
				sawMoon = true
			}
			if id == "minecraft:painting_variant" && bytes.Contains(p.Data, []byte("tachyne:sunset")) {
				sawSunset = true
			}
		case cfgClientUpdateTags:
			sawTags = true
		}
	}
	if !sawMoon || !sawSunset || !sawTags {
		t.Fatalf("configuration data: moon %v sunset %v tags %v", sawMoon, sawSunset, sawTags)
	}
	if err := protocol.WriteCompressed(client, cfgServerFinish, nil, compressThreshold); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	cc.configured()
	if cc.send(playClientGameEvent, []byte{13, 0, 0, 0, 0}) != nil || !cc.configuring() {
		t.Fatal("a play packet went before the rejoin")
	}

	go func() { done <- cc.rejoin(joinPacket(9, 0, 8, nil, false, attach.Welcome{Config: &sc.ConfigData})) }()
	p = readCfg(t, cbr)
	if want, _, _ := tr.Clientbound(protocol.StatePlay, playClientLogin, nil); p.ID != want {
		t.Fatalf("rejoin wrote 0x%x first, want the login 0x%x", p.ID, want)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if cc.configuring() {
		t.Fatal("still between phases after the login")
	}
	if cc.rejoin(nil) != errEarlyRejoin {
		t.Fatal("a rejoin outside a reconfiguration")
	}
}

// update_tags in play: the world's tags over the configured registries.
func TestPlayUpdateTagsResolvesConfiguredRegistries(t *testing.T) {
	c := moonConfig()
	body := protocol.UpdateTagsPacketWith(777, configExtras(&c))
	if bytes.Equal(body, protocol.UpdateTagsPacket(777)) {
		t.Fatal("the data pack's tag changed nothing")
	}
	if !bytes.Contains(body, []byte("minecraft:placeable")) {
		t.Fatal("placeable missing")
	}
}

// The engine names tag registries by their data-pack folder; the gateway
// resolves them as the registry keys. A vanilla tag that no longer loads
// arrives empty and goes out empty.
func TestTagExtrasFolderRegistries(t *testing.T) {
	x := configExtras(&attach.ConfigData{Tags: []attach.TagSet{
		{Registry: "block", Tags: []attach.Tag{{Name: "minecraft:logs", Entries: []string{}}}},
		{Registry: "worldgen/biome", Tags: []attach.Tag{{Name: "minecraft:is_forest", Entries: []string{"minecraft:plains"}}}},
	}})
	if len(x.Tags) != 2 || x.Tags[0].Registry != "minecraft:block" || x.Tags[1].Registry != "minecraft:worldgen/biome" {
		t.Fatalf("registries %+v", x.Tags)
	}
	body := protocol.UpdateTagsPacketWith(777, x)
	r := bytes.NewReader(body)
	n, _ := protocol.ReadVarInt(r)
	for i := int32(0); i < n; i++ {
		reg, _ := protocol.ReadString(r)
		c, _ := protocol.ReadVarInt(r)
		for j := int32(0); j < c; j++ {
			name, _ := protocol.ReadString(r)
			k, _ := protocol.ReadVarInt(r)
			for l := int32(0); l < k; l++ {
				protocol.ReadVarInt(r)
			}
			if reg == "minecraft:block" && name == "minecraft:logs" && k != 0 {
				t.Fatalf("logs kept %d members", k)
			}
		}
	}
	if r.Len() != 0 {
		t.Fatalf("%d trailing bytes", r.Len())
	}
}
