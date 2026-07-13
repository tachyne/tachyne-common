package attach

// Entity/presence frames (attach v2): the hub's multiplayer state as domain
// events, so any gateway renders any player/mob for any client version.
// Positions are absolute; angles are degrees.
const (
	MsgPlayerInfo   = 0x0a // w→gw: tab-list add
	MsgPlayerGone   = 0x0b // w→gw: tab-list remove
	MsgEntityAdd    = 0x0c // w→gw: entity appears (players AND mobs)
	MsgEntityMove   = 0x0d // w→gw: absolute position/look
	MsgEntityHead   = 0x0e // w→gw: head yaw
	MsgEntityRemove = 0x0f // w→gw: entities gone
	MsgChat         = 0x10 // gw→w: player chat line; w→gw: chat to display
)

type PlayerInfo struct {
	UUID  [16]byte   `json:"uuid"`
	Name  string     `json:"name"`
	Props []Property `json:"props,omitempty"` // game-profile properties (textures = skin)
}

// Property is one game-profile property (the "textures" blob carries skins).
type Property struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Signature string `json:"signature,omitempty"`
}

type PlayerGone struct {
	UUID [16]byte `json:"uuid"`
}

type EntityAdd struct {
	EID   int32    `json:"eid"`
	UUID  [16]byte `json:"uuid"`
	Type  int32    `json:"type"` // network entity-type id (1.21.5 registry)
	X     float64  `json:"x"`
	Y     float64  `json:"y"`
	Z     float64  `json:"z"`
	Yaw   float32  `json:"yaw"`
	Pitch float32  `json:"pitch"`
	Data  int32    `json:"data,omitempty"` // spawn "object data" (e.g. shooter eid+1 for arrows)
	VX    float64  `json:"vx,omitempty"`   // initial velocity, blocks/tick — how the client
	VY    float64  `json:"vy,omitempty"`   // learns a projectile's launch arc before the
	VZ    float64  `json:"vz,omitempty"`   // first move event
}

type EntityMove struct {
	EID      int32   `json:"eid"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Z        float64 `json:"z"`
	Yaw      float32 `json:"yaw"`
	Pitch    float32 `json:"pitch"`
	OnGround bool    `json:"on_ground"`
	// NoSync marks movement that must always render as RELATIVE moves, never
	// sync_entity_position: the 770→776 translation of that packet loses the
	// entity on 26.x clients (observed with the ender dragon). Renderers
	// saturate oversized deltas and converge instead of resyncing.
	NoSync bool `json:"no_sync,omitempty"`
}

type EntityHead struct {
	EID int32   `json:"eid"`
	Yaw float32 `json:"yaw"`
}

type EntityRemove struct {
	EIDs []int32 `json:"eids"`
}

type Chat struct {
	Text string `json:"text"`
	// ActionBar renders the text as the above-hotbar overlay instead of a
	// chat line (the engine's HUD uses it).
	ActionBar bool `json:"action_bar,omitempty"`
}

// Interaction frames (attach v2.1): block breaking/placing from gateway
// players, and block changes streaming back to every session.
const (
	MsgDig      = 0x11 // gw→w: player action on a block
	MsgPlace    = 0x12 // gw→w: use item on a block face
	MsgHeldSlot = 0x13 // gw→w: hotbar slot selection
	MsgBlockSet = 0x14 // w→gw: a block changed (render + cache invalidation)
)

// Dig statuses follow vanilla semantics (0 start/creative break, 2 finish,
// 3/4 drop stack/one, 5 release use); the world validates.
type Dig struct {
	Status int32 `json:"status"`
	X      int   `json:"x"`
	Y      int   `json:"y"`
	Z      int   `json:"z"`
	Face   int32 `json:"face"`
}

type Place struct {
	Hand   int32   `json:"hand"`
	X      int     `json:"x"`
	Y      int     `json:"y"`
	Z      int     `json:"z"`
	Face   int32   `json:"face"`
	CX     float32 `json:"cx"` // cursor within the face
	CY     float32 `json:"cy"`
	CZ     float32 `json:"cz"`
	Inside bool    `json:"inside"`
}

type HeldSlot struct {
	Slot int16 `json:"slot"`
}

type BlockSet struct {
	X     int    `json:"x"`
	Y     int    `json:"y"`
	Z     int    `json:"z"`
	State uint32 `json:"state"`
}

// Dimension frames (attach v3): portal travel through gateways.
const (
	MsgDimension = 0x15 // w→gw: the player moved to another dimension
	MsgTeleport  = 0x16 // w→gw: server-authoritative position (dim switch, /tp)
)

type Dimension struct {
	Dim int32 `json:"dim"` // 0 overworld, 1 nether, 2 end
	// Gamemode rides along so the respawn packet carries the player's real
	// mode (0 survival, 1 creative, 2 adventure, 3 spectator).
	Gamemode int32 `json:"gamemode,omitempty"`
}

type Teleport struct {
	Pos
}

// MsgCommand (gw→w): a /command typed by the player (no leading slash).
const MsgCommand = 0x17

type Command struct {
	Cmd string `json:"cmd"`
}

// (0x18/0x19 were MsgRaw/MsgRawServer, the raw play-packet bridge — deleted
// with stage 6 of the domain-events refactor: every frame is typed now.)

// Serverbound action frames (stage 6b): every player intent is typed.
const (
	MsgUseItem      = 0x34 // gw→w: right-click the held item (eat/draw/throw)
	MsgUseEntity    = 0x35 // gw→w: attack / interact with an entity
	MsgSelTrade     = 0x36 // gw→w: select a merchant offer
	MsgInput        = 0x37 // gw→w: movement-input flags (sneak = dismount)
	MsgWindowClick  = 0x38 // gw→w: container slot click
	MsgCraft        = 0x39 // gw→w: recipe-book auto-fill request
	MsgWindowClose  = 0x3a // gw→w: close the open container
	MsgNameItem     = 0x3b // gw→w: anvil rename box
	MsgEnchant      = 0x3c // gw→w: enchant-table option click
	MsgPlayerAction = 0x3d // gw→w: player command (sneak/sprint/leave-bed)
	MsgRespawnReq   = 0x3e // gw→w: death screen's respawn button
	MsgCreativeSlot = 0x3f // gw→w: creative-mode slot set
)

type UseItem struct{}

type UseEntity struct {
	Target int32 `json:"target"`
	Attack bool  `json:"attack,omitempty"` // false = interact
}

type SelTrade struct {
	Slot int32 `json:"slot"`
}

type Input struct {
	Sneak bool `json:"sneak,omitempty"`
}

// ClickChange is one slot the client's click prediction changed. Item carries
// id+count only (the wire form is hashed; the world revalidates anyway).
type ClickChange struct {
	Slot int32     `json:"slot"`
	Item ItemStack `json:"item"`
}

type WindowClick struct {
	ID      int32         `json:"id"`
	Slot    int32         `json:"slot"`
	Mode    int32         `json:"mode"`
	Changed []ClickChange `json:"changed,omitempty"`
	Cursor  ItemStack     `json:"cursor"`
}

type Craft struct {
	Window int32 `json:"window"`
	Recipe int32 `json:"recipe"`
}

type WindowClose struct{}

type NameItem struct {
	Name string `json:"name"`
}

type Enchant struct {
	Button int32 `json:"button"`
}

// PlayerAction mirrors the vanilla player_command action ids
// (0 sneak, 1 unsneak, 2 leave bed, 3 sprint, 4 unsprint).
type PlayerAction struct {
	Action int32 `json:"action"`
}

type RespawnReq struct{}

type CreativeSlot struct {
	Slot int32     `json:"slot"`
	Item ItemStack `json:"item"` // id+count (most components not needed world-side)
	// PaintingVariant is the painting/variant item component when the slot
	// holds a creative-menu painting preset — vanilla places exactly that
	// variant instead of the random largest-fit selection.
	PaintingVariant string `json:"painting_variant,omitempty"`
}

// Survival-state frames (stage 3 of the domain-events refactor) — all
// player-directed: the session's own health/XP/effects/death UI.
const (
	MsgHealth = 0x1b // w→gw: health + food + saturation
	MsgXP     = 0x1c // w→gw: experience bar/level
	MsgEffect = 0x1d // w→gw: status effect applied/removed
	MsgHurt   = 0x1e // w→gw: hurt animation (red flash + directional tilt)
	MsgDeath  = 0x1f // w→gw: death screen
)

type Health struct {
	Health     float32 `json:"health"` // 0..20 half-hearts
	Food       int32   `json:"food"`   // 0..20 drumsticks
	Saturation float32 `json:"saturation"`
}

type XP struct {
	Progress float32 `json:"progress"` // 0..1 bar fill
	Level    int32   `json:"level"`
	Total    int32   `json:"total"`
}

type Effect struct {
	EID    int32 `json:"eid"`
	ID     int32 `json:"id"` // minecraft:mob_effect registry id
	Amp    int32 `json:"amp,omitempty"`
	Ticks  int32 `json:"ticks,omitempty"`
	Remove bool  `json:"remove,omitempty"` // true = effect ended
}

type Hurt struct {
	EID int32   `json:"eid"`
	Yaw float32 `json:"yaw"` // attack direction for the camera tilt
}

type Death struct {
	EID     int32  `json:"eid"`
	Message string `json:"message"`
}

// Item/container frames (stage 4 of the domain-events refactor).
const (
	MsgEquipment   = 0x20 // w→gw: an entity's worn/held loadout
	MsgEntityMeta  = 0x21 // w→gw: entity appearance metadata (opaque, typed later)
	MsgWindowOpen  = 0x22 // w→gw: open a container screen
	MsgWindowItems = 0x23 // w→gw: full window contents
	MsgWindowSlot  = 0x24 // w→gw: one window slot changed
	MsgWindowData  = 0x25 // w→gw: window property (furnace progress bars)
	MsgHeldSync    = 0x26 // w→gw: server-set hotbar selection
	MsgCollect     = 0x27 // w→gw: item-pickup fly-to-player animation
)

// ItemStack is the domain item: id + count, plus the stack's structured
// components (durability, enchantments, custom name, …) in CANONICAL (770)
// wire form — opaque scaffolding, typed later. Gateways pass the component
// bytes through their translator chain, which renumbers per client version.
// A zero Count means the empty stack.
type ItemStack struct {
	ID         int32  `json:"id,omitempty"`
	Count      int32  `json:"count,omitempty"`
	Components []byte `json:"components,omitempty"` // add-count + remove-count + entries
}

// Equipment slot order on the wire (Slots array index).
const (
	EquipMainHand = 0
	EquipOffhand  = 1
	EquipFeet     = 2
	EquipLegs     = 3
	EquipChest    = 4
	EquipHead     = 5
	EquipBody     = 6 // 1.20.5+ body slot: wolf/horse armor, happy-ghast harness
	EquipSaddle   = 7 // 1.21.5+ saddle slot (mounts; replaced the old flag bit)
)

type Equipment struct {
	EID   int32        `json:"eid"`
	Slots [8]ItemStack `json:"slots"` // indexed by the Equip* constants (incl. body/saddle)
	// SendSaddle includes slot 7 on the wire even when empty (mount
	// broadcasts need it to CLEAR a removed saddle; player loadouts omit it).
	SendSaddle bool `json:"send_saddle,omitempty"`
}

// EntityMeta carries an entity's appearance metadata (poses, fire, baby,
// sheared, slime size, saddles, the dropped-item stack, …) as the canonical
// metadata list — opaque scaffolding with the same typed-later story as
// ItemStack.Components.
type EntityMeta struct {
	EID  int32  `json:"eid"`
	Meta []byte `json:"meta"` // canonical entity-metadata list, terminator included
}

// MsgHorseScreen opens a mount's inventory screen (vanilla
// horse_screen_open): its own packet, not open_screen — the client lays the
// window out from the chest-column count and the mount's entity.
const MsgHorseScreen = 0x67 // w→gw

// HorseScreen opens the mount inventory window. Columns is the chest grid
// width (0 = no chest; llama strength or 5 for donkeys/mules).
type HorseScreen struct {
	ID      int32 `json:"id"` // window id (byte-ranged)
	Columns int32 `json:"columns"`
	EID     int32 `json:"eid"`
}

// MsgWaypoint drives the locator-bar HUD (26.2+ clientbound tracked_waypoint,
// a packet with NO canonical-770 form): players broadcast their position so
// others see a direction marker. Gateways below 776 drop it (older clients
// lack the feature). Op 0 track, 1 untrack, 2 update; only VEC3I positions
// in v1 (the client renders direction + distance from the block pos).
const MsgWaypoint = 0x6a // w→gw

// Waypoint is one transmitter's marker for the receiving player.
type Waypoint struct {
	Op       int8     `json:"op"`
	UUID     [16]byte `json:"uuid"`
	Style    string   `json:"style,omitempty"` // waypoint_style asset ("" = minecraft:default)
	Color    int32    `json:"color,omitempty"` // RGB; 0 = team/default color
	HasColor bool     `json:"has_color,omitempty"`
	X        int32    `json:"x"`
	Y        int32    `json:"y"`
	Z        int32    `json:"z"`
}

// MsgEditBook / MsgOpenBook: book editing + reading. The engine owns book
// contents (a store keyed by book id, like maps); the component bytes on
// the item stack carry the pages to clients.
const (
	MsgEditBook = 0x68 // gw→w: writable-book page save / signing
	MsgOpenBook = 0x69 // w→gw: open the held written book's reader UI
)

// EditBook is the vanilla edit_book intent: pages replace the held writable
// book's contents; a title signs it into a written book.
type EditBook struct {
	Slot     int32    `json:"slot"` // hotbar 0-8 or 40 = offhand
	Pages    []string `json:"pages,omitempty"`
	Title    string   `json:"title,omitempty"`
	HasTitle bool     `json:"has_title,omitempty"`
}

// OpenBook opens the reader for the book in a hand (0 main, 1 off).
type OpenBook struct {
	Hand int32 `json:"hand"`
}

type WindowOpen struct {
	ID    int32  `json:"id"`
	Menu  int32  `json:"menu"` // canonical minecraft:menu registry id
	Title string `json:"title"`
}

type WindowItems struct {
	ID      int32       `json:"id"`
	StateID int32       `json:"state_id"`
	Slots   []ItemStack `json:"slots"`
	Cursor  ItemStack   `json:"cursor"`
}

type WindowSlot struct {
	ID      int32     `json:"id"`
	StateID int32     `json:"state_id"`
	Slot    int32     `json:"slot"`
	Item    ItemStack `json:"item"`
}

type WindowData struct {
	ID    int32 `json:"id"`
	Prop  int32 `json:"prop"`
	Value int32 `json:"value"`
}

type HeldSync struct {
	Slot int32 `json:"slot"`
}

type Collect struct {
	Collected int32 `json:"collected"` // the item/orb entity being scooped up
	Collector int32 `json:"collector"` // the player entity collecting it
	Count     int32 `json:"count"`
}

// World-effect frames (stage 5 of the domain-events refactor).
const (
	MsgSound     = 0x28 // w→gw: named positioned sound
	MsgParticles = 0x29 // w→gw: payload-free particle burst
	MsgWorldFX   = 0x2a // w→gw: positioned world event (block-break FX, …)
)

// Sound is a positioned sound BY NAME — the version-proof holder form; the
// client ignores names it doesn't know.
type Sound struct {
	Name     string  `json:"name"`
	Category int32   `json:"category"` // soundSource enum (stable across versions)
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Z        float64 `json:"z"`
	Volume   float32 `json:"volume"`
	Pitch    float32 `json:"pitch"`
}

// Particles is a payload-free particle burst. PID is the CANONICAL (770)
// particle type id; gateway translator chains remap it per client version.
type Particles struct {
	PID    int32   `json:"pid"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Z      float64 `json:"z"`
	Spread float32 `json:"spread"`
	Speed  float32 `json:"speed"`
	Count  int32   `json:"count"`
}

// WorldFX is a positioned world event (e.g. 2001 = block-break particles +
// sound, rendered by the client from Data = the block state).
type WorldFX struct {
	Event int32 `json:"event"`
	X     int   `json:"x"`
	Y     int   `json:"y"`
	Z     int   `json:"z"`
	Data  int32 `json:"data"`
}

// Stage-6a frames: the last clientbound stragglers become typed events.
const (
	MsgGameEvent   = 0x2b // w→gw: game event (rain, gamemode change, wait-for-chunks)
	MsgAbilities   = 0x2c // w→gw: the session's own movement abilities
	MsgPassengers  = 0x2d // w→gw: who rides an entity
	MsgVehicleMove = 0x2e // w→gw: authoritative vehicle position (snap-back)
	MsgVelocity    = 0x2f // w→gw: entity velocity impulse (knockback, jumps)
	MsgTrades      = 0x30 // w→gw: merchant offers (opaque, typed later)
	MsgCursorItem  = 0x31 // w→gw: the stack carried on the mouse cursor
	MsgDifficulty  = 0x32 // w→gw: world difficulty
	MsgCommandTree = 0x33 // w→gw: brigadier command tree (opaque, typed later)
)

// GameEvent mirrors the vanilla game_event packet (a stable enum + value).
type GameEvent struct {
	Event int32   `json:"event"`
	Value float32 `json:"value,omitempty"`
}

type Abilities struct {
	Invulnerable bool `json:"invulnerable,omitempty"`
	Flying       bool `json:"flying,omitempty"`
	MayFly       bool `json:"may_fly,omitempty"`
	Creative     bool `json:"creative,omitempty"` // instabuild
}

type Passengers struct {
	Vehicle int32   `json:"vehicle"`
	Riders  []int32 `json:"riders"` // empty = everyone dismounted
}

type VehicleMove struct {
	X   float64 `json:"x"`
	Y   float64 `json:"y"`
	Z   float64 `json:"z"`
	Yaw float32 `json:"yaw"`
}

// Velocity is an impulse in blocks/tick (renderers scale to wire units).
type Velocity struct {
	EID int32   `json:"eid"`
	VX  float64 `json:"vx"`
	VY  float64 `json:"vy"`
	VZ  float64 `json:"vz"`
}

// Trades carries the canonical merchant_offers body — opaque, typed later.
type Trades struct {
	Data []byte `json:"data"`
}

type CursorItem struct {
	Item ItemStack `json:"item"`
}

type Difficulty struct {
	Level  int32 `json:"level"`
	Locked bool  `json:"locked,omitempty"`
}

// CommandTree carries the canonical brigadier tree — opaque, typed later.
type CommandTree struct {
	Data []byte `json:"data"`
}

// Entity animation frames (stage 6c sweep).
const (
	MsgEntityStatus = 0x40 // w→gw: entity status event (death animation, love, tame)
	MsgSwing        = 0x41 // w→gw: arm swing
)

// EntityStatus mirrors the vanilla entity_event byte (a stable enum: 3 death
// animation, 6/7 tame fail/ok, 18 love hearts, …).
type EntityStatus struct {
	EID    int32 `json:"eid"`
	Status int32 `json:"status"`
}

type Swing struct {
	EID int32 `json:"eid"`
}

// MsgRecipeBook (w→gw): the full recipe list for the green crafting book, sent
// once at join. Item ids are CANONICAL (770); the renderer remaps them into
// the client's id space per version (recipe_book_add carries raw item ids and
// has no body rewriter in the translation chain, so it must be built at the
// client's real version).
const MsgRecipeBook = 0x42

// RecipeBook lists every craftable recipe as a book display entry. Display id
// = index: shaped entries first (0..len(Shaped)-1), then shapeless — the same
// ids the client echoes back in craft_recipe_request.
type RecipeBook struct {
	// Replace true = the player's whole known book (join); false = newly
	// unlocked entries appended (vanilla ServerRecipeBook.addRecipes).
	Replace   bool              `json:"replace,omitempty"`
	Shaped    []ShapedRecipe    `json:"shaped,omitempty"`
	Shapeless []ShapelessRecipe `json:"shapeless,omitempty"`
}

// ShapedRecipe is a WxH row-major pattern (Cells has W*H entries, 0 = empty).
// ID is the engine-assigned display id (stable across increments); Notify and
// Highlight mirror the vanilla add-packet entry flags (toast / book badge).
type ShapedRecipe struct {
	ID        int32   `json:"id"`
	W         int32   `json:"w"`
	H         int32   `json:"h"`
	Cells     []int32 `json:"cells"`
	Result    int32   `json:"result"`
	Count     int32   `json:"count"`
	Notify    bool    `json:"notify,omitempty"`
	Highlight bool    `json:"hl,omitempty"`
}

// ShapelessRecipe is an unordered ingredient list → result.
type ShapelessRecipe struct {
	ID          int32   `json:"id"`
	Ingredients []int32 `json:"ing"`
	Result      int32   `json:"result"`
	Count       int32   `json:"count"`
	Notify      bool    `json:"notify,omitempty"`
	Highlight   bool    `json:"hl,omitempty"`
}

// MsgResync (w→gw): re-request the current chunk window with Force set — the
// world side of /refresh (fixes client-side render loss). Carries no fields;
// the gateway already knows its window.
const MsgResync = 0x43

// Resync is the (empty) MsgResync payload.
type Resync struct{}

// MsgBossBar (w→gw): boss-fight health bar (the dragon, the wither).
const MsgBossBar = 0x1a

// BossBar operations (mirroring the vanilla packet's action ids).
const (
	BossBarAdd    = 0 // show the bar: Title + Health used
	BossBarRemove = 1 // hide the bar
	BossBarHealth = 2 // update the fill fraction: Health used
)

type BossBar struct {
	UUID   [16]byte `json:"uuid"`
	Op     int32    `json:"op"`
	Title  string   `json:"title,omitempty"`
	Health float32  `json:"health,omitempty"` // 0..1 fill fraction
}

// Advancement frames (w→gw). The engine owns the tree (canonical 1.21.11
// data), criteria evaluation, and per-player grant state; the gateway renders
// update_advancements per client version. MsgAdvTree crosses once per join —
// the tree is static per engine build; MsgAdvProgress follows with the
// player's full snapshot (Reset) and then streams single-grant increments.
const (
	MsgAdvTree     = 0x44 // w→gw: the advancement tree (join-time, static)
	MsgAdvProgress = 0x45 // w→gw: per-player criteria progress
)

// AdvNode is one advancement as the client sees it. Requirements is the
// wire's OR-of-ANDs of criterion names (the union of names is also the
// criteria set — the wire never carries criteria separately). Display fields
// are zero for invisible helper nodes (HasDisplay false).
type AdvNode struct {
	ID     string     `json:"id"`
	Parent string     `json:"parent,omitempty"`
	Reqs   [][]string `json:"reqs"`

	HasDisplay bool      `json:"has_display,omitempty"`
	Title      string    `json:"title,omitempty"` // translate key (vanilla) or literal
	Desc       string    `json:"desc,omitempty"`
	Icon       ItemStack `json:"icon,omitempty"`  // same id space as every other ItemStack
	Frame      int32     `json:"frame,omitempty"` // 0 task, 1 challenge, 2 goal
	Background string    `json:"bg,omitempty"`    // root tabs only (texture id, no namespace)
	ShowToast  bool      `json:"toast,omitempty"`
	Announce   bool      `json:"announce,omitempty"`
	Hidden     bool      `json:"hidden,omitempty"`
	X          float32   `json:"x,omitempty"` // vanilla tidy-tree layout (depth)
	Y          float32   `json:"y,omitempty"` // (row)
}

type AdvTree struct {
	Nodes []AdvNode `json:"nodes"`
}

// AdvProgressEntry is one advancement's obtained criteria → unix millis.
// Criteria required but absent from Done are rendered as not-yet-obtained.
type AdvProgressEntry struct {
	ID   string           `json:"id"`
	Done map[string]int64 `json:"done"`
}

// AdvProgress carries progress for one player. Reset marks the join-time
// full snapshot (the renderer pairs it with the tree in one reset packet);
// increments follow with just the newly-obtained criteria.
type AdvProgress struct {
	Reset   bool               `json:"reset,omitempty"`
	Entries []AdvProgressEntry `json:"entries"`
}

// Statistics frames. The client's Statistics screen is request-driven
// (client_command action 1): the gateway forwards the request and the world
// replies with the player's full snapshot. Entries are (stat type, key,
// value) in canonical 774 ids — key registry depends on the type (mined:
// block registry; crafted/used/broken/picked_up/dropped: items; killed/
// killed_by: entities; custom: the custom_stat registry).
const (
	MsgStats    = 0x46 // w→gw: full statistics snapshot
	MsgStatsReq = 0x47 // gw→w: the client opened the Statistics screen
)

// Stat type ids (the vanilla stat_type registry order — identical on every
// version the chain serves).
const (
	StatMined    = 0
	StatCrafted  = 1
	StatUsed     = 2
	StatBroken   = 3
	StatPickedUp = 4
	StatDropped  = 5
	StatKilled   = 6
	StatKilledBy = 7
	StatCustom   = 8
)

type StatEntry struct {
	T int32 `json:"t"` // stat type (Stat* consts)
	K int32 `json:"k"` // key id in the type's registry (canonical 774)
	V int32 `json:"v"`
}

type Stats struct {
	Entries []StatEntry `json:"entries"`
}

type StatsReq struct{}

// Recipe-book progression frames, mirroring the vanilla ServerRecipeBook
// model: a per-player KNOWN set (sent filtered at join, grown by unlock
// increments riding MsgRecipeBook), a HIGHLIGHT set (the "new" badge,
// cleared by MsgRecipeSeen), and the per-book-type open/filter settings.
const (
	MsgRecipeSettings      = 0x48 // w→gw: the player's book settings (join)
	MsgRecipeSettingChange = 0x49 // gw→w: the client toggled open/filter
	MsgRecipeSeen          = 0x4a // gw→w: a highlighted recipe was viewed
)

// Recipe book types, in the vanilla enum/wire order.
const (
	RecipeBookCrafting     = 0
	RecipeBookFurnace      = 1
	RecipeBookBlastFurnace = 2
	RecipeBookSmoker       = 3
)

// RecipeSettings carries all four book types' (open, filtering) pairs,
// indexed by the RecipeBook* constants.
type RecipeSettings struct {
	Open   [4]bool `json:"open"`
	Filter [4]bool `json:"filter"`
}

type RecipeSettingChange struct {
	Book   int32 `json:"book"`
	Open   bool  `json:"open"`
	Filter bool  `json:"filter"`
}

type RecipeSeen struct {
	ID int32 `json:"id"`
}

// Scoreboard frames (w→gw), mirroring the vanilla ServerScoreboard model:
// objectives (with display text + render type), the three display slots,
// per-owner scores, and teams (display/prefix/suffix, color, flags,
// membership). The engine owns all state and the /scoreboard + /team
// commands; gateways render the five vanilla packets per client version.
const (
	MsgObjective   = 0x4b // objective add / remove / update
	MsgDisplaySlot = 0x4c // bind an objective to list/sidebar/below_name
	MsgScore       = 0x4d // one owner's score in one objective (or its reset)
	MsgTeam        = 0x4e // team add / remove / update / membership change
)

// Objective methods and team methods use the vanilla packet method ids.
const (
	ObjAdd    = 0
	ObjRemove = 1
	ObjUpdate = 2

	TeamAdd           = 0
	TeamRemove        = 1
	TeamUpdate        = 2
	TeamAddPlayers    = 3
	TeamRemovePlayers = 4
)

// Display slots (vanilla ids).
const (
	SlotList      = 0
	SlotSidebar   = 1
	SlotBelowName = 2
)

type Objective struct {
	Name   string `json:"name"`
	Method int32  `json:"method"`
	Title  string `json:"title,omitempty"`  // literal display text
	Hearts bool   `json:"hearts,omitempty"` // render type: hearts vs integer
}

type DisplaySlot struct {
	Slot      int32  `json:"slot"`
	Objective string `json:"objective,omitempty"` // "" clears the slot
}

type Score struct {
	Owner     string `json:"owner"`
	Objective string `json:"objective"`
	Value     int32  `json:"value,omitempty"`
	Reset     bool   `json:"reset,omitempty"` // true = reset_score instead
}

type Team struct {
	Name         string   `json:"name"`
	Method       int32    `json:"method"`
	Title        string   `json:"title,omitempty"`
	Prefix       string   `json:"prefix,omitempty"`
	Suffix       string   `json:"suffix,omitempty"`
	Color        int32    `json:"color"` // ChatFormatting color ordinal 0-15; -1 = none
	FriendlyFire bool     `json:"ff,omitempty"`
	SeeInvisible bool     `json:"seeinvis,omitempty"`
	Visibility   int32    `json:"vis,omitempty"`  // nametag: 0 always … 3 hideForOwnTeam
	Collision    int32    `json:"coll,omitempty"` // 0 always, 1 never, 2/3 push rules
	Players      []string `json:"players,omitempty"`
}

// Sign frames, mirroring the vanilla SignBlockEntity model: two SignText
// sides (4 message lines, a dye color, a glow flag) plus is_waxed. The
// editing lock (vanilla playerWhoMayEdit) is transient engine state and
// never crosses the protocol; the engine enforces it on SignUpdate.
// (0x4f is the last free byte of the 0x44–0x4f event range and 0x50–0x5f is
// the sharding block — ordinary event growth continues at 0x60.)
const (
	MsgSignText   = 0x60 // w→gw: a sign's full text state (block_entity_data)
	MsgSignEditor = 0x61 // w→gw: open the sign edit GUI (open_sign_editor)
	MsgSignUpdate = 0x62 // gw→w: player submitted 4 raw lines for one side
)

// MsgMapData mirrors the vanilla map update flow (map_item_data): per-map
// color patches (the dirty rectangle a holder's tracker accumulated) plus
// the live decoration set (player markers, frames, banners). The engine
// owns MapItemSavedData-shaped state and per-holder dirty tracking; the
// frame is one holder's update.
const MsgMapData = 0x63 // w→gw: map color patch + decorations for one viewer

// MapDecoration is one marker on a map. Type is the map_decoration_type
// registry id (player=0, frame=1, the 16 banner colors, …); X/Z are map
// pixel coords ×2 (-128..127, vanilla's packed form); Rot is 0-15.
type MapDecoration struct {
	Type int32  `json:"type"`
	X    int8   `json:"x"`
	Z    int8   `json:"z"`
	Rot  uint8  `json:"rot"`
	Name string `json:"name,omitempty"`
}

// MapData updates one map for one viewer. When Width is 0 there is no color
// patch (decorations-only update); Colors is the patch rectangle row-major
// (Width×Height packed color bytes, vanilla id<<2|brightness). Decorations
// nil means "no decoration change"; an empty non-nil slice clears them.
type MapData struct {
	EID      int32           `json:"eid"` // the viewing player
	MapID    int32           `json:"map_id"`
	Scale    int8            `json:"scale"`
	Locked   bool            `json:"locked,omitempty"`
	Decor    []MapDecoration `json:"decor,omitempty"`
	HasDecor bool            `json:"has_decor,omitempty"`
	X        uint8           `json:"x,omitempty"`
	Y        uint8           `json:"y,omitempty"`
	Width    uint8           `json:"width,omitempty"`
	Height   uint8           `json:"height,omitempty"`
	Colors   []byte          `json:"colors,omitempty"`
}

// MsgSetBeacon is the beacon menu's confirm click (serverbound
// set_beacon_effect): the chosen powers, encoded like the beacon menu's
// container properties — mob_effect registry id + 1, 0 = none.
const MsgSetBeacon = 0x64 // gw→w: beacon effect choice

// SetBeacon carries the player's beacon power selection.
type SetBeacon struct {
	Primary   int32 `json:"primary"`             // mob_effect id + 1; 0 = none
	Secondary int32 `json:"secondary,omitempty"` // mob_effect id + 1; 0 = none
}

// MsgCampfireItems syncs a campfire's four cook slots to viewers (vanilla's
// update tag carries only Items) — the client renders the food on the fire.
const MsgCampfireItems = 0x65 // w→gw

// CampfireItems is one campfire's visible contents in the receiving
// player's dimension. Items are item registry names ("" = empty slot).
type CampfireItems struct {
	X     int32     `json:"x"`
	Y     int32     `json:"y"`
	Z     int32     `json:"z"`
	Items [4]string `json:"items"`
}

// MsgBannerPatterns syncs a placed banner's pattern layers to viewers (the
// vanilla update tag; base color is the block's own).
const MsgBannerPatterns = 0x66 // w→gw

// BannerLayer is one pattern layer: registry name + dye color name.
type BannerLayer struct {
	Pattern string `json:"pattern"`
	Color   string `json:"color"`
}

// BannerPatterns is one placed banner's layers in the receiving player's
// dimension (≤6; empty clears).
type BannerPatterns struct {
	X      int32         `json:"x"`
	Y      int32         `json:"y"`
	Z      int32         `json:"z"`
	Layers []BannerLayer `json:"layers,omitempty"`
}

// SignSide mirrors vanilla SignText: four plain-text message lines, the
// applied dye color (name; "" = black, the default) and the glow-ink flag.
type SignSide struct {
	Lines [4]string `json:"lines"`
	Color string    `json:"color,omitempty"`
	Glow  bool      `json:"glow,omitempty"`
}

// SignText is a sign block entity's full client-visible state at a position
// in the receiving player's dimension. Hanging selects the hanging_sign
// block-entity type for rendering.
type SignText struct {
	X       int32    `json:"x"`
	Y       int32    `json:"y"`
	Z       int32    `json:"z"`
	Front   SignSide `json:"front"`
	Back    SignSide `json:"back"`
	Waxed   bool     `json:"waxed,omitempty"`
	Hanging bool     `json:"hanging,omitempty"`
}

// SignEditor opens the sign editing GUI on one side of a sign the engine has
// granted this player permission to edit.
type SignEditor struct {
	X     int32 `json:"x"`
	Y     int32 `json:"y"`
	Z     int32 `json:"z"`
	Front bool  `json:"front"`
}

// SignUpdate is the serverbound edit result: the four raw lines the player
// typed for one side. The engine strips legacy §-format codes, applies the
// edit-permission and waxed checks, and rebroadcasts SignText.
type SignUpdate struct {
	X     int32     `json:"x"`
	Y     int32     `json:"y"`
	Z     int32     `json:"z"`
	Front bool      `json:"front"`
	Lines [4]string `json:"lines"`
}
