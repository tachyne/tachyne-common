package gwsession

import (
	"bytes"
	"testing"

	"github.com/tachyne/tachyne-common/protocol"
)

func TestResourcePackPushAndAnswers(t *testing.T) {
	p := NewResourcePack("https://example.org/pack.zip", "abc", true, "Please")
	if p == nil || p.ID[6]>>4 != 3 {
		t.Fatalf("pack %+v: want a name-based (v3) UUID", p)
	}
	b := packPush(p)
	if !bytes.Equal(b[:16], p.ID[:]) {
		t.Fatal("the push starts with the pack's id")
	}
	ans := func(act int32) []byte { return protocol.AppendVarInt(append([]byte(nil), p.ID[:]...), act) }
	if term, dec := packResponseTerminal(ans(3)); term || dec {
		t.Fatal("ACCEPTED is not final")
	}
	if term, dec := packResponseTerminal(ans(1)); !term || !dec {
		t.Fatal("DECLINED is final and a decline")
	}
	if term, dec := packResponseTerminal(ans(0)); !term || dec {
		t.Fatal("SUCCESSFULLY_LOADED is final")
	}
	if NewResourcePack("", "", false, "") != nil {
		t.Fatal("no URL, no pack")
	}
}
