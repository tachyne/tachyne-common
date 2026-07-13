package render770

// parse.go decodes canonical-770 SERVERBOUND play-packet bodies into typed
// action frames — the gateway-side twin of the renderers. Gateways for other
// versions back-translate to canonical 770 first (the translator chain), so
// one parser serves every client version.

import (
	"bytes"
	"encoding/binary"
	"math"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// Canonical-770 serverbound play packet IDs for the actions parsed here.
const (
	SIDChunkBatchReceived = 0x09 // chunk-batch ack + desired chunks-per-tick

	SIDClientCommand  = 0x0a // respawn (action 0) / stats request (action 1)
	SIDEnchantItem    = 0x0f
	SIDWindowClick    = 0x10
	SIDCloseWindow    = 0x11
	SIDUseEntity      = 0x18
	SIDVehicleMove    = 0x20
	SIDCraftRequest   = 0x25
	SIDEntityAction   = 0x28
	SIDPlayerInput    = 0x29
	SIDRecipeSettings = 0x2c // recipe_book_change_settings
	SIDRecipeSeen     = 0x2d // recipe_book_seen_recipe
	SIDNameItem       = 0x2e
	SIDSelTrade       = 0x31
	SIDEditBook       = 0x16 // edit_book (writable-book save / sign)
	SIDSetBeacon      = 0x32 // set_beacon_effect (the menu's confirm click)
	SIDCreativeSlot   = 0x36
	SIDSignUpdate     = 0x3a // update_sign (sign edit GUI result)
	SIDUseItem        = 0x3f
)

const inputSneakBit = 0x20 // player_input flags: sneak

func readI16(br *bytes.Reader) (int16, bool) {
	var b [2]byte
	if _, err := br.Read(b[:]); err != nil || br.Len() < 0 {
		return 0, false
	}
	return int16(binary.BigEndian.Uint16(b[:])), true
}

// ParseChunkBatchReceived decodes chunk_batch_received: the client's ack of a
// chunk batch, carrying how many chunks per tick it wants (float; vanilla
// derives it from its own processing time). The gateway uses it to pace chunk
// delivery — this packet is gateway-local flow control and never reaches the
// world.
func ParseChunkBatchReceived(data []byte) (float32, bool) {
	if len(data) < 4 {
		return 0, false
	}
	v := math.Float32frombits(binary.BigEndian.Uint32(data))
	if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) || v < 0 {
		return 0, false
	}
	return v, true
}

// ParseUseEntity decodes interact_entity; ok=false for interact_at (2),
// which carries no action the world handles.
func ParseUseEntity(data []byte) (attach.UseEntity, bool) {
	br := bytes.NewReader(data)
	target, err := protocol.ReadVarInt(br)
	if err != nil {
		return attach.UseEntity{}, false
	}
	mouse, err := protocol.ReadVarInt(br)
	if err != nil || mouse > 1 { // 0=interact, 1=attack, 2=interact_at (unused)
		return attach.UseEntity{}, false
	}
	return attach.UseEntity{Target: target, Attack: mouse == 1}, true
}

// ParseVehicleMove decodes move_vehicle (x, y, z doubles + yaw, pitch).
func ParseVehicleMove(data []byte) (attach.VehicleMove, bool) {
	if len(data) < 28 {
		return attach.VehicleMove{}, false
	}
	f64 := func(off int) float64 {
		return math.Float64frombits(binary.BigEndian.Uint64(data[off:]))
	}
	yaw := math.Float32frombits(binary.BigEndian.Uint32(data[24:]))
	return attach.VehicleMove{X: f64(0), Y: f64(8), Z: f64(16), Yaw: yaw}, true
}

// ParseSelTrade decodes select_trade.
func ParseSelTrade(data []byte) (attach.SelTrade, bool) {
	n, err := protocol.ReadVarInt(bytes.NewReader(data))
	if err != nil {
		return attach.SelTrade{}, false
	}
	return attach.SelTrade{Slot: n}, true
}

// ParseInput decodes player_input flags.
func ParseInput(data []byte) (attach.Input, bool) {
	if len(data) < 1 {
		return attach.Input{}, false
	}
	return attach.Input{Sneak: data[0]&inputSneakBit != 0}, true
}

// ParseWindowClick decodes container_click. Changed-slot stacks arrive in the
// hashed form: id + count are read, per-component hashes skipped.
func ParseWindowClick(data []byte) (attach.WindowClick, bool) {
	br := bytes.NewReader(data)
	win, err := protocol.ReadVarInt(br)
	if err != nil {
		return attach.WindowClick{}, false
	}
	if _, err := protocol.ReadVarInt(br); err != nil { // state id (accepted as-is)
		return attach.WindowClick{}, false
	}
	slot, ok := readI16(br)
	if !ok {
		return attach.WindowClick{}, false
	}
	if _, err := br.ReadByte(); err != nil { // mouse button (mode disambiguates)
		return attach.WindowClick{}, false
	}
	mode, err := protocol.ReadVarInt(br)
	if err != nil {
		return attach.WindowClick{}, false
	}
	n, err := protocol.ReadVarInt(br)
	if err != nil || n < 0 || n > 128 {
		return attach.WindowClick{}, false
	}
	e := attach.WindowClick{ID: win, Slot: int32(slot), Mode: mode}
	for i := 0; i < int(n); i++ {
		s, ok := readI16(br)
		if !ok {
			return attach.WindowClick{}, false
		}
		st, ok := readHashedSlot(br)
		if !ok {
			return attach.WindowClick{}, false
		}
		e.Changed = append(e.Changed, attach.ClickChange{Slot: int32(s), Item: st})
	}
	cur, ok := readHashedSlot(br)
	if !ok {
		return attach.WindowClick{}, false
	}
	e.Cursor = cur
	return e, true
}

// readHashedSlot reads a hashed Slot: id + count kept, component hashes
// skipped (the world revalidates against its authoritative inventory).
func readHashedSlot(br *bytes.Reader) (attach.ItemStack, bool) {
	has, err := br.ReadByte()
	if err != nil {
		return attach.ItemStack{}, false
	}
	if has == 0 {
		return attach.ItemStack{}, true
	}
	item, err := protocol.ReadVarInt(br)
	if err != nil {
		return attach.ItemStack{}, false
	}
	count, err := protocol.ReadVarInt(br)
	if err != nil {
		return attach.ItemStack{}, false
	}
	nAdd, err := protocol.ReadVarInt(br)
	if err != nil || nAdd < 0 || nAdd > 64 {
		return attach.ItemStack{}, false
	}
	for i := 0; i < int(nAdd); i++ {
		if _, err := protocol.ReadVarInt(br); err != nil { // component type
			return attach.ItemStack{}, false
		}
		var hash [4]byte
		if _, err := br.Read(hash[:]); err != nil { // component hash (i32)
			return attach.ItemStack{}, false
		}
	}
	nRm, err := protocol.ReadVarInt(br)
	if err != nil || nRm < 0 || nRm > 64 {
		return attach.ItemStack{}, false
	}
	for i := 0; i < int(nRm); i++ {
		if _, err := protocol.ReadVarInt(br); err != nil {
			return attach.ItemStack{}, false
		}
	}
	return attach.ItemStack{ID: item, Count: count}, true
}

// ParseCraft decodes craft_recipe_request (makeAll ignored — fills one).
func ParseCraft(data []byte) (attach.Craft, bool) {
	br := bytes.NewReader(data)
	win, err := protocol.ReadVarInt(br)
	if err != nil {
		return attach.Craft{}, false
	}
	id, err := protocol.ReadVarInt(br)
	if err != nil {
		return attach.Craft{}, false
	}
	return attach.Craft{Window: win, Recipe: id}, true
}

// ParseNameItem decodes the anvil rename box.
func ParseNameItem(data []byte) (attach.NameItem, bool) {
	name, err := protocol.ReadString(bytes.NewReader(data))
	if err != nil {
		return attach.NameItem{}, false
	}
	return attach.NameItem{Name: name}, true
}

// ParseEnchant decodes enchant_item (windowId + option index).
func ParseEnchant(data []byte) (attach.Enchant, bool) {
	br := bytes.NewReader(data)
	if _, err := protocol.ReadVarInt(br); err != nil {
		return attach.Enchant{}, false
	}
	btn, err := protocol.ReadVarInt(br)
	if err != nil {
		return attach.Enchant{}, false
	}
	return attach.Enchant{Button: btn}, true
}

// ParseSetBeacon decodes set_beacon_effect: two Optional<Holder<MobEffect>>
// (presence bool + mob_effect registry id each). The attach frame carries
// them in the beacon menu's property encoding: id + 1, 0 = none.
func ParseSetBeacon(data []byte) (attach.SetBeacon, bool) {
	br := bytes.NewReader(data)
	var out attach.SetBeacon
	for _, dst := range []*int32{&out.Primary, &out.Secondary} {
		has, err := br.ReadByte()
		if err != nil {
			return attach.SetBeacon{}, false
		}
		if has != 0 {
			id, err := protocol.ReadVarInt(br)
			if err != nil {
				return attach.SetBeacon{}, false
			}
			*dst = id + 1
		}
	}
	return out, true
}

// ParseEditBook decodes edit_book: slot VarInt, page strings (≤100 of
// ≤1024), optional title (≤32).
func ParseEditBook(data []byte) (attach.EditBook, bool) {
	br := bytes.NewReader(data)
	slot, err := protocol.ReadVarInt(br)
	if err != nil {
		return attach.EditBook{}, false
	}
	n, err := protocol.ReadVarInt(br)
	if err != nil || n < 0 || n > 100 {
		return attach.EditBook{}, false
	}
	out := attach.EditBook{Slot: slot}
	for i := int32(0); i < n; i++ {
		s, err := protocol.ReadString(br)
		if err != nil || len(s) > 1024 {
			return attach.EditBook{}, false
		}
		out.Pages = append(out.Pages, s)
	}
	has, err := br.ReadByte()
	if err != nil {
		return attach.EditBook{}, false
	}
	if has != 0 {
		title, err := protocol.ReadString(br)
		if err != nil || len(title) > 32 {
			return attach.EditBook{}, false
		}
		out.Title, out.HasTitle = title, true
	}
	return out, true
}

// ParsePlayerAction decodes player_command (entityId + action).
func ParsePlayerAction(data []byte) (attach.PlayerAction, bool) {
	br := bytes.NewReader(data)
	if _, err := protocol.ReadVarInt(br); err != nil { // entityId (self)
		return attach.PlayerAction{}, false
	}
	action, err := protocol.ReadVarInt(br)
	if err != nil {
		return attach.PlayerAction{}, false
	}
	return attach.PlayerAction{Action: action}, true
}

// ParseRespawnReq decodes client_command; ok only for actionId 0 (respawn).
func ParseRespawnReq(data []byte) (attach.RespawnReq, bool) {
	return attach.RespawnReq{}, len(data) > 0 && data[0] == 0
}

// ParseStatsReq decodes client_command; ok only for actionId 1 (the client
// opened the Statistics screen and wants the snapshot).
func ParseStatsReq(data []byte) (attach.StatsReq, bool) {
	return attach.StatsReq{}, len(data) > 0 && data[0] == 1
}

// ParseRecipeSettingChange decodes recipe_book_change_settings: book type
// enum + open + filtering.
func ParseRecipeSettingChange(data []byte) (attach.RecipeSettingChange, bool) {
	br := bytes.NewReader(data)
	book, err := protocol.ReadVarInt(br)
	if err != nil || book < 0 || book > 3 {
		return attach.RecipeSettingChange{}, false
	}
	open, e1 := br.ReadByte()
	filter, e2 := br.ReadByte()
	if e1 != nil || e2 != nil {
		return attach.RecipeSettingChange{}, false
	}
	return attach.RecipeSettingChange{Book: book, Open: open != 0, Filter: filter != 0}, true
}

// ParseRecipeSeen decodes recipe_book_seen_recipe: one display id.
func ParseRecipeSeen(data []byte) (attach.RecipeSeen, bool) {
	id, err := protocol.ReadVarInt(bytes.NewReader(data))
	if err != nil {
		return attach.RecipeSeen{}, false
	}
	return attach.RecipeSeen{ID: id}, true
}

// ParseSignUpdate decodes update_sign: position, which side, and the four raw
// lines the player typed (wire cap 384 chars/line, vanilla's
// ServerboundSignUpdatePacket.MAX_STRING_LENGTH).
func ParseSignUpdate(data []byte) (attach.SignUpdate, bool) {
	if len(data) < 8 {
		return attach.SignUpdate{}, false
	}
	x, y, z := protocol.ReadPosition(data[:8])
	br := bytes.NewReader(data[8:])
	front, err := br.ReadByte()
	if err != nil {
		return attach.SignUpdate{}, false
	}
	e := attach.SignUpdate{X: int32(x), Y: int32(y), Z: int32(z), Front: front != 0}
	for i := range e.Lines {
		line, err := protocol.ReadString(br)
		if err != nil || len(line) > 384 {
			return attach.SignUpdate{}, false
		}
		e.Lines[i] = line
	}
	return e, true
}

// ParseCreativeSlot decodes set_creative_mode_slot: slot + a FULL Slot, of
// which id+count are kept (components not needed world-side).
func ParseCreativeSlot(data []byte, clientProto int32) (attach.CreativeSlot, bool) {
	br := bytes.NewReader(data)
	slot, ok := readI16(br)
	if !ok {
		return attach.CreativeSlot{}, false
	}
	count, err := protocol.ReadVarInt(br)
	if err != nil {
		return attach.CreativeSlot{}, false
	}
	e := attach.CreativeSlot{Slot: int32(slot)}
	if count > 0 {
		item, err := protocol.ReadVarInt(br)
		if err != nil {
			return attach.CreativeSlot{}, false
		}
		e.Item = attach.ItemStack{ID: item, Count: count}
		e.PaintingVariant = creativePaintingVariant(br, clientProto)
	}
	return e, true
}

// creativePaintingVariant extracts the painting/variant component from a
// creative slot's component list, if it is the first added component — the
// creative menu's painting presets carry exactly that one. Components are
// still in the CLIENT's id space (the back-translation renumbers only the
// item id), so the component-type id is looked up per version. Anything
// unexpected yields "" (the engine falls back to vanilla's random largest
// fit).
func creativePaintingVariant(br *bytes.Reader, clientProto int32) string {
	compID := protocol.PaintingComponentID(clientProto)
	if compID < 0 {
		return ""
	}
	nAdd, err := protocol.ReadVarInt(br)
	if err != nil || nAdd < 1 {
		return ""
	}
	if _, err := protocol.ReadVarInt(br); err != nil { // remove-count precedes the entries
		return ""
	}
	typ, err := protocol.ReadVarInt(br)
	if err != nil || typ != compID {
		return "" // a different component leads — unknown payload, stop
	}
	// The untrusted slot codec (all serverbound creative slots) wraps each
	// component value in a byte-length prefix (vanilla
	// DataComponentPatch.DELIMITED_STREAM_CODEC).
	vlen, err := protocol.ReadVarInt(br)
	if err != nil || vlen < 1 || vlen > 5 {
		return ""
	}
	holder, err := protocol.ReadVarInt(br)
	if err != nil || holder <= 0 {
		return "" // 0 would be an inline definition — not a menu preset
	}
	return protocol.PaintingVariantName(holder - 1)
}
