package attach

// Recipe data the client holds (vanilla RecipeManager's synchronized parts,
// sent by PlayerList.placeNewPlayer and, after a /reload, reloadResources as
// ClientboundUpdateRecipesPacket): the recipe property sets — the items each
// station slot accepts (RecipeManager.getSynchronizedItemProperties) — and
// the stonecutter's selectable recipes in their fixed order
// (getSynchronizedStonecutterRecipes; the menu's button index is a position
// in this list filtered by the input item, so the engine's menu must use the
// same order).

// MsgUpdateRecipes (w→gw): the recipes changed (a /reload). The gateway
// keeps them for the session (a rejoin and a shard handover send them
// again) and sends update_recipes now.
const MsgUpdateRecipes = 0xab

// UpdateRecipes is ClientboundUpdateRecipesPacket's content. Each half is
// optional on its own: a nil slice (JSON null or absent) means the
// gateway's generated vanilla table; an empty, non-nil one ([]) means none.
// Item ids are canonical item registry ids.
type UpdateRecipes struct {
	ItemSets    []RecipePropertySet `json:"item_sets"`
	Stonecutter []StonecutterRecipe `json:"stonecutter"`
}

// RecipePropertySet is one RecipePropertySet: its key (minecraft:
// smithing_base, smithing_template, smithing_addition, furnace_input,
// blast_furnace_input, smoker_input, campfire_input, brewing_input,
// brewing_reagent) and the items it accepts.
type RecipePropertySet struct {
	Key   string  `json:"key"`
	Items []int32 `json:"items"`
}

// RecipeIngredient is an Ingredient: a holder set of items, either a tag
// (Tag, e.g. "minecraft:planks"; the client resolves it from its tags) or
// an explicit item list.
type RecipeIngredient struct {
	Items []int32 `json:"items,omitempty"`
	Tag   string  `json:"tag,omitempty"`
}

// StonecutterRecipe is one SelectableRecipe.SingleInputEntry: the input
// ingredient and the option display — the result stack (item and count).
type StonecutterRecipe struct {
	Input  RecipeIngredient `json:"input"`
	Result int32            `json:"result"`
	Count  int32            `json:"count"`
}
