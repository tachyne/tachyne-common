package gwsession

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
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
		_, err := configure(Config{}, sbr, server, tr, 384, 776, configExtras(&sc.ConfigData), nil)
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
	if want, _, _ := tr.Clientbound(protocol.StatePlay, playClientLogin, []byte{0}); p.ID != want { // the join rewriter reads the body's last byte
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

// dialogPack is a data pack with a dialog of its own, a replacement for a
// built-in one, and the two dialog tags the client reads (the pause
// screen's additions and the quick-actions key), named as the engine names
// them: the registry by folder, members by id.
func dialogPack() attach.ConfigData {
	welcome := `{"type":"minecraft:notice","title":{"text":"Welcome","bold":true},"can_close_with_escape":false,
		"body":[{"type":"minecraft:plain_message","contents":["Read ",{"text":"this","color":"gold"}],"width":250}],
		"action":{"label":"OK","action":{"type":"minecraft:dynamic/custom","id":"tachyne:ack","additions":{"n":1.5}}}}`
	links := `{"type":"minecraft:server_links","title":"Links","button_width":310,"columns":2}`
	return attach.ConfigData{
		Registries: []attach.RegistryEntries{{Registry: "dialog", Entries: []attach.RegistryEntry{
			{Name: "tachyne:welcome", Data: json.RawMessage(welcome)},
			{Name: "minecraft:server_links", Data: json.RawMessage(links)},
		}}},
		Tags: []attach.TagSet{{Registry: "dialog", Tags: []attach.Tag{
			{Name: "minecraft:pause_screen_additions", Entries: []string{"tachyne:welcome"}},
			{Name: "minecraft:quick_actions", Entries: []string{"tachyne:welcome", "minecraft:server_links"}},
		}}},
	}
}

// readRegistry parses one registry_data body: its entries' names and, for
// those with data, the element read back as JSON.
func readRegistry(t *testing.T, body []byte) (string, []string, map[string]json.RawMessage) {
	t.Helper()
	r := bytes.NewReader(body)
	id, _ := protocol.ReadString(r)
	n, _ := protocol.ReadVarInt(r)
	var names []string
	data := map[string]json.RawMessage{}
	for i := int32(0); i < n; i++ {
		name, err := protocol.ReadString(r)
		if err != nil {
			t.Fatalf("%s entry %d: %v", id, i, err)
		}
		names = append(names, name)
		if has, _ := r.ReadByte(); has == 1 {
			js, err := protocol.NetworkNBTToJSON(r)
			if err != nil {
				t.Fatalf("%s %s: %v", id, name, err)
			}
			data[name] = js
		}
	}
	if r.Len() != 0 {
		t.Fatalf("%s: %d trailing bytes", id, r.Len())
	}
	return id, names, data
}

// tagIDs reads update_tags and returns one tag's member ids.
func tagIDs(t *testing.T, body []byte, registry, tag string) []int32 {
	t.Helper()
	r := bytes.NewReader(body)
	n, _ := protocol.ReadVarInt(r)
	var found []int32
	for i := int32(0); i < n; i++ {
		reg, _ := protocol.ReadString(r)
		c, _ := protocol.ReadVarInt(r)
		for j := int32(0); j < c; j++ {
			name, _ := protocol.ReadString(r)
			k, _ := protocol.ReadVarInt(r)
			var ids []int32
			for l := int32(0); l < k; l++ {
				id, _ := protocol.ReadVarInt(r)
				ids = append(ids, id)
			}
			if reg == registry && name == tag {
				found = ids
			}
		}
	}
	if r.Len() != 0 {
		t.Fatalf("update_tags: %d trailing bytes", r.Len())
	}
	return found
}

// sameJSON compares two JSON values, numbers by value and booleans as the
// bytes NbtOps writes them (true = 1).
func sameJSON(t *testing.T, a, b []byte) bool {
	t.Helper()
	var norm func(v any) any
	norm = func(v any) any {
		switch x := v.(type) {
		case bool:
			if x {
				return float64(1)
			}
			return float64(0)
		case map[string]any:
			for k, e := range x {
				x[k] = norm(e)
			}
		case []any:
			for i, e := range x {
				x[i] = norm(e)
			}
		}
		return v
	}
	var va, vb any
	if json.Unmarshal(a, &va) != nil || json.Unmarshal(b, &vb) != nil {
		t.Fatalf("not JSON: %s / %s", a, b)
	}
	ja, _ := json.Marshal(norm(va))
	jb, _ := json.Marshal(norm(vb))
	return bytes.Equal(ja, jb)
}

// A data pack's dialogs round-trip: the registry keeps the built-in ids
// (the replacement in place, the new dialog appended as 3), each element
// reads back as the JSON it was, the two dialog tags resolve to those ids,
// and show_dialog finds the pack's dialog by name.
func TestDataPackDialogs(t *testing.T) {
	pack := dialogPack()
	x := configExtras(&pack)
	for _, v := range []int32{776, 777} {
		var names []string
		var data map[string]json.RawMessage
		for _, p := range protocol.ConfigRegistryPacketsWith(v, 384, x) {
			if id, n, d := readRegistry(t, p); id == "minecraft:dialog" {
				names, data = n, d
			}
		}
		want := []string{"minecraft:custom_options", "minecraft:quick_actions", "minecraft:server_links", "tachyne:welcome"}
		if fmt.Sprint(names) != fmt.Sprint(want) {
			t.Fatalf("v%d: dialog registry %v", v, names)
		}
		for _, e := range pack.Registries[0].Entries {
			if !sameJSON(t, data[e.Name], e.Data) {
				t.Errorf("v%d: %s came back as %s", v, e.Name, data[e.Name])
			}
		}
		tags := protocol.UpdateTagsPacketWith(v, x)
		if got := tagIDs(t, tags, "minecraft:dialog", "minecraft:pause_screen_additions"); fmt.Sprint(got) != "[3]" {
			t.Errorf("v%d: pause_screen_additions %v", v, got)
		}
		if got := tagIDs(t, tags, "minecraft:dialog", "minecraft:quick_actions"); fmt.Sprint(got) != "[3 2]" {
			t.Errorf("v%d: quick_actions %v", v, got)
		}
		body, ok := render770.ShowDialogBodyWith(attach.ShowDialog{Ref: "tachyne:welcome"}, func(n string) (int32, bool) {
			return protocol.DialogRegistryIDWith(v, x, n)
		})
		if !ok || !bytes.Equal(body, protocol.AppendVarInt(nil, 4)) {
			t.Errorf("v%d: show_dialog %x %v", v, body, ok)
		}
	}
	// Without a pack the built-in ids answer, and an unknown name none.
	if id, ok := protocol.DialogRegistryIDWith(777, protocol.ConfigExtras{}, "minecraft:server_links"); !ok || id != 2 {
		t.Errorf("built-in server_links %d %v", id, ok)
	}
	if _, ok := protocol.DialogRegistryIDWith(777, x, "tachyne:nope"); ok {
		t.Error("an unknown dialog resolved")
	}
}

// A reconfiguration carrying new dialogs, end to end over the pipe: the
// StartConfiguration frame's JSON, the configuration phase the client sees
// (the dialog registry with the pack's entry, the tags naming it).
func TestReconfigurationCarriesDialogs(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	server.SetDeadline(time.Now().Add(20 * time.Second))
	client.SetDeadline(time.Now().Add(20 * time.Second))
	tr := protocol.TranslatorFor(777)
	cc := &clientConn{c: server, tr: tr, entTypes: map[int32]int32{}, menus: map[int32]int32{}}
	cbr := bufio.NewReader(client)

	raw, _ := json.Marshal(attach.StartConfiguration{ConfigData: dialogPack()})
	var e attach.StartConfiguration
	if err := json.Unmarshal(raw, &e); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cc.startConfiguration(e) }()
	readCfg(t, cbr) // start_configuration
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	sc := cc.takeConfig()
	go func() {
		_, err := configure(Config{}, bufio.NewReader(server), server, tr, 384, 777, configExtras(&sc.ConfigData), nil)
		done <- err
	}()
	readCfg(t, cbr) // known packs
	protocol.WriteCompressed(client, cfgServerKnownPacks, protocol.AppendVarInt(nil, 0), compressThreshold)
	var sawDialog bool
	var pause []int32
	tagsID, _, _ := tr.Clientbound(protocol.StateConfiguration, cfgClientUpdateTags, nil) // 26.3 moved it
	for {
		p := readCfg(t, cbr)
		if p.ID == cfgClientFinish {
			break
		}
		switch p.ID {
		case cfgClientRegistryData:
			if id, names, data := readRegistry(t, p.Data); id == "minecraft:dialog" {
				sawDialog = len(names) == 4 && names[3] == "tachyne:welcome" && data["tachyne:welcome"] != nil
			}
		case tagsID:
			pause = tagIDs(t, p.Data, "minecraft:dialog", "minecraft:pause_screen_additions")
		}
	}
	if !sawDialog || fmt.Sprint(pause) != "[3]" {
		t.Fatalf("dialog registry %v, pause_screen_additions %v", sawDialog, pause)
	}
	protocol.WriteCompressed(client, cfgServerFinish, nil, compressThreshold)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
