package protocol

import (
	"bytes"
	"strings"
	"testing"
)

// TestConfigRegistryPacketsWellFormed strictly re-parses every composed
// registry packet for every supported client version: registry id, entry
// count, then per entry a NAME string, has_data bool, and (when set) a
// well-formed nameless NBT compound. This is the test that would have caught
// the refactor dropping the entry-name string (real 26.2 clients caught it
// instead: "failed to decode clientbound/minecraft:registry_data").
func TestConfigRegistryPacketsWellFormed(t *testing.T) {
	for _, v := range []int32{Target, 772, 775, MaxTranslated} {
		packets := ConfigRegistryPackets(v)
		if len(packets) < 18 {
			t.Fatalf("v%d: only %d registry packets", v, len(packets))
		}
		for _, data := range packets {
			r := bytes.NewReader(data)
			regID, err := ReadString(r)
			if err != nil || !strings.HasPrefix(regID, "minecraft:") {
				t.Fatalf("v%d: bad registry id %q: %v", v, regID, err)
			}
			n, err := ReadVarInt(r)
			if err != nil || n <= 0 {
				t.Fatalf("v%d %s: bad entry count %d: %v", v, regID, n, err)
			}
			for i := int32(0); i < n; i++ {
				name, err := ReadString(r)
				if err != nil || !strings.Contains(name, ":") {
					t.Fatalf("v%d %s entry %d: bad name %q: %v", v, regID, i, name, err)
				}
				hasData, err := r.ReadByte()
				if err != nil || hasData > 1 {
					t.Fatalf("v%d %s %s: bad has_data %d: %v", v, regID, name, hasData, err)
				}
				if hasData == 1 {
					rest := data[len(data)-r.Len():]
					if rest[0] != nbtCompound {
						t.Fatalf("v%d %s %s: NBT root tag %#x", v, regID, name, rest[0])
					}
					consumed := 1 + walkNBT(t, rest[1:], nbtCompound) // nameless root
					r.Seek(int64(consumed), 1)
				}
			}
			if r.Len() != 0 {
				t.Fatalf("v%d %s: %d trailing bytes", v, regID, r.Len())
			}
		}
	}
}
