package protocol

// ItemTag263 is a 26.3 item tag's members, as registry names
// ("minecraft:tnt"), resolved the way the tag is sent to a 26.3 client
// (tags263_gen.go: nested tags already flattened). An unknown tag is nil.
//
// A gateway that has to render something the Java client derives from a
// tag — the Bedrock gateway picks a sulfur cube's archetype look from the
// #sulfur_cube_archetype/* tag its swallowed block is in — reads it here,
// so there is one copy of the vanilla tag data.
func ItemTag263(tag string) []string {
	for _, reg := range tags263Data {
		if reg.registry != "minecraft:item" {
			continue
		}
		for _, t := range reg.tags {
			if t.name == tag {
				out := make([]string, len(t.entries))
				copy(out, t.entries)
				return out
			}
		}
	}
	return nil
}
