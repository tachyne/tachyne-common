package protocol

import "strings"

// blockEntityTypeNames is the canonical block_entity_type registry in its
// order (1.21.11's, the ids blockEntityTypeFor maps; 26.2 and 26.3 drop bed
// and append potent_sulfur). Item components name block entity types —
// block_entity_data's TypedEntityData — so the copier needs the names.
var blockEntityTypeNames = []string{
	"furnace", "chest", "trapped_chest", "ender_chest", "jukebox", "dispenser",
	"dropper", "sign", "hanging_sign", "mob_spawner", "creaking_heart", "piston",
	"brewing_stand", "enchanting_table", "end_portal", "beacon", "skull",
	"daylight_detector", "hopper", "comparator", "banner", "structure_block",
	"end_gateway", "command_block", "shulker_box", "bed", "conduit", "barrel",
	"smoker", "blast_furnace", "lectern", "bell", "jigsaw", "campfire", "beehive",
	"sculk_sensor", "calibrated_sculk_sensor", "sculk_catalyst", "sculk_shrieker",
	"chiseled_bookshelf", "shelf", "brushable_block", "decorated_pot", "crafter",
	"trial_spawner", "vault", "test_block", "test_instance_block",
	"copper_golem_statue",
}

// potentSulfur26x is the 26.x-only potent_sulfur type, the last entry of
// those clients' registry (after bed's removal, index 48).
const (
	potentSulfurName = "potent_sulfur"
	potentSulfur26x  = 48
)

// BlockEntityTypeID is the named block entity type's registry id at a
// client version ("minecraft:" optional); false when the client lacks it.
func BlockEntityTypeID(version int32, name string) (int32, bool) {
	name = strings.TrimPrefix(name, "minecraft:")
	if name == potentSulfurName {
		return potentSulfur26x, version >= 776
	}
	for i, n := range blockEntityTypeNames {
		if n == name {
			return blockEntityTypeFor(version, int32(i))
		}
	}
	return 0, false
}

// blockEntityTypeName is the "minecraft:"-prefixed name of a client's block
// entity type id.
func blockEntityTypeName(version, id int32) (string, bool) {
	if version >= 776 && id == potentSulfur26x {
		return "minecraft:" + potentSulfurName, true
	}
	for i, n := range blockEntityTypeNames {
		if c, ok := blockEntityTypeFor(version, int32(i)); ok && c == id {
			return "minecraft:" + n, true
		}
	}
	return "", false
}
