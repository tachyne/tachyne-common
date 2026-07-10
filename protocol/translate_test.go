package protocol

import "testing"

// stubTranslator swaps packet id 1<->2 clientbound and tags bodies, to prove the
// seam actually routes through the Translator (identity wouldn't show a change).
type stubTranslator struct{ ver int32 }

func (s stubTranslator) Version() int32 { return s.ver }
func (s stubTranslator) Clientbound(_ State, id int32, b []byte) (int32, []byte, bool) {
	if id == 99 {
		return id, b, true // drop
	}
	return id + 1000, append([]byte{0xC0}, b...), false
}
func (s stubTranslator) Serverbound(_ State, id int32, b []byte) (int32, []byte, bool) {
	return id - 1000, b, false
}

func TestIdentityPassesThrough(t *testing.T) {
	body := []byte{1, 2, 3}
	id, out, drop := Identity.Clientbound(StatePlay, 0x2b, body)
	if id != 0x2b || drop || &out[0] != &body[0] {
		t.Errorf("identity Clientbound altered the packet: id=%d drop=%v", id, drop)
	}
	id, out, drop = Identity.Serverbound(StateLogin, 0x00, body)
	if id != 0x00 || drop || &out[0] != &body[0] {
		t.Errorf("identity Serverbound altered the packet: id=%d drop=%v", id, drop)
	}
}

func TestTranslatorForTarget(t *testing.T) {
	// Target(770) is the LAYOUT base but no longer Identity: the engine's content
	// is 1.21.11(774), so even a 770 client gets the id-remap chain (0 layout steps).
	if tr := TranslatorFor(Target); tr == nil || tr == Identity || tr.Version() != Target {
		t.Errorf("TranslatorFor(Target) = %v, want a non-identity 770 id-remap chain", tr)
	}
	if got := TranslatorFor(1); got != nil { // protocol 1 is ancient & unregistered
		t.Errorf("TranslatorFor(unknown) = %v, want nil", got)
	}
}

func TestRegisterAndLookup(t *testing.T) {
	const v = 999001 // a sentinel version no real client uses
	register(stubTranslator{ver: v})
	defer delete(translators, v)

	got := TranslatorFor(v)
	if got == nil || got.Version() != v {
		t.Fatalf("TranslatorFor(%d) did not return the registered stub", v)
	}
	id, out, drop := got.Clientbound(StatePlay, 5, []byte{0xAA})
	if id != 1005 || drop || len(out) != 2 || out[0] != 0xC0 {
		t.Errorf("stub Clientbound = (%d,%v,%v)", id, out, drop)
	}
	if _, _, drop := got.Clientbound(StatePlay, 99, nil); !drop {
		t.Error("stub should drop id 99")
	}

	found := false
	for _, sv := range SupportedVersions() {
		if sv == v {
			found = true
		}
	}
	if !found {
		t.Error("SupportedVersions did not include the registered version")
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	const v = 999002
	register(stubTranslator{ver: v})
	defer delete(translators, v)
	defer func() {
		if recover() == nil {
			t.Error("registering a duplicate version should panic")
		}
	}()
	register(stubTranslator{ver: v})
}

func TestItoa(t *testing.T) {
	for _, c := range []struct {
		n    int32
		want string
	}{{0, "0"}, {770, "770"}, {-5, "-5"}, {999001, "999001"}} {
		if got := itoa(c.n); got != c.want {
			t.Errorf("itoa(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}
