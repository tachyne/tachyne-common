package protocol

import (
	"bytes"
	"testing"
)

// A player's shoulder parrot (26.x DATA_SHOULDER_PARROT_LEFT, index 19, an
// OptionalInt of the parrot variant: VarInt id+1, 0 = none) is emitted with
// the canonical OPTIONAL_UNSIGNED_INT serializer (20, after COMPOUND_TAG) and
// reaches a 26.x client as serializer 19 — the 26.3 registration order —
// with the payload untouched.
func TestShoulderParrotMetaTranslation(t *testing.T) {
	body := AppendVarInt(nil, 42)
	body = append(body, 19)
	body = AppendVarInt(body, metaTypeOptUInt)
	body = AppendVarInt(body, 3) // variant 2 (green) + 1
	body = append(body, 20)
	body = AppendVarInt(body, metaTypeOptUInt)
	body = AppendVarInt(body, 0) // right shoulder empty
	body = append(body, 0xff)

	want := AppendVarInt(nil, 42)
	want = append(want, 19)
	want = AppendVarInt(want, 19)
	want = AppendVarInt(want, 3)
	want = append(want, 20)
	want = AppendVarInt(want, 19)
	want = AppendVarInt(want, 0)
	want = append(want, 0xff)
	for _, v := range []int32{776, 777} {
		if got := remapEntityMeta(v, body); !bytes.Equal(got, want) {
			t.Errorf("%d: got %x want %x", v, got, want)
		}
	}
}
