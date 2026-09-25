package gwsession

import (
	"testing"

	"github.com/tachyne/tachyne-common/protocol"
)

// Every AbstractHorse carries its flags byte at 17, 18 on 26.2+: the whole
// family must be in the ageable shift, or a donkey's flags land on 26.x's
// AGE_LOCKED Boolean and disconnect the client.
func TestHorseFamilyTakesTheAgeableShift(t *testing.T) {
	for _, name := range []string{"horse", "donkey", "mule", "skeleton_horse", "zombie_horse",
		"llama", "trader_llama", "camel", "camel_husk"} {
		if !ageableIntMetaTypes[protocol.CanonicalEntity(name)] {
			t.Errorf("%s is not enrolled in the 26.2 ageable index shift", name)
		}
	}
}
