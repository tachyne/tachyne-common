package protocol

// Multi-version translation seam (ViaVersion-style). The game core implements ONE
// canonical protocol version (Target); a per-connection Translator adapts every
// packet between the client's actual version and Target, so the rest of the server
// only ever speaks canonical packets. This is the plug-in point for goal #3
// (multi-version client support): each newer/older version registers a Translator
// describing how its packets differ from Target's.
//
// Why here: translation is pure wire-layer work (it rewrites packet IDs and field
// bytes), so it belongs in protocol, the leaf package. The core (worldgen/world)
// stays version-agnostic; only the connection boundary translates.

// State identifies the connection state that scopes packet IDs. The same numeric
// packet ID means different things in Login vs Play, so translation is state-aware.
type State uint8

const (
	StateHandshake State = iota
	StateStatus
	StateLogin
	StateConfiguration
	StatePlay
)

func (s State) String() string {
	switch s {
	case StateHandshake:
		return "handshake"
	case StateStatus:
		return "status"
	case StateLogin:
		return "login"
	case StateConfiguration:
		return "configuration"
	case StatePlay:
		return "play"
	default:
		return "unknown"
	}
}

// Translator adapts packets between a client's protocol version and the core's
// canonical Target version. A connection holds exactly one for its lifetime.
//
//   - Clientbound rewrites a canonical (Target) packet the server produced into
//     the wire form the client expects.
//   - Serverbound rewrites a packet the client sent into canonical form.
//
// Each returns the (possibly changed) packet ID and body, plus drop=true when the
// packet has no equivalent on the other side and should be silently discarded.
// Implementations must not retain the input slice; return a fresh slice when the
// body changes, or the same slice unchanged when it does not.
type Translator interface {
	Version() int32
	Clientbound(state State, id int32, body []byte) (outID int32, out []byte, drop bool)
	Serverbound(state State, id int32, body []byte) (outID int32, out []byte, drop bool)
}

// identity passes packets through unchanged — used when the client already speaks
// the canonical Target version, which is the common case today.
type identity struct{}

func (identity) Version() int32 { return Target }
func (identity) Clientbound(_ State, id int32, b []byte) (int32, []byte, bool) {
	return id, b, false
}
func (identity) Serverbound(_ State, id int32, b []byte) (int32, []byte, bool) {
	return id, b, false
}

// Identity is the shared pass-through translator for canonical-version clients.
var Identity Translator = identity{}

// translators holds the registered per-version Translators, keyed by the client
// protocol version they handle. Per-version files populate this in their init().
var translators = map[int32]Translator{}

// register installs a Translator for its Version(). Intended for per-version files'
// init(); panics on a duplicate so two files can't silently claim one version.
func register(t Translator) {
	v := t.Version()
	if _, dup := translators[v]; dup {
		panic("protocol: duplicate translator for version " + itoa(v))
	}
	translators[v] = t
}

// TranslatorFor returns the Translator for a client protocol version, or nil if
// the version is unsupported (the caller should reject the connection with a clear
// message rather than let it fail later on a malformed packet). The canonical
// Target is always supported via the Identity translator.
func TranslatorFor(version int32) Translator {
	if t, ok := translators[version]; ok {
		return t // hand-written override wins over the generated chain
	}
	// NOTE: even a Target(770)-layout client goes through the chain now. The
	// engine's CONTENT is 1.21.11 (proto 774) while the wire LAYOUT canonical is
	// still 770 (render770), so every client — including 770 — needs the id-remap
	// (1.21.11→client) that chainFor applies; a 770 client just gets zero layout
	// steps. There is no pure-Identity served version (770 needs ids, 774 would
	// need layout), so Identity is only a fallback for exact-match hand overrides.
	return chainFor(version) // id-remap (+ layout steps for >770), or nil if out of range
}

// SupportedVersions lists every protocol version the server can serve: Target, the
// translated chain range (Target+1..MaxTranslated), and any hand-registered
// override. Useful for diagnostics / the login rejection message.
func SupportedVersions() []int32 {
	vs := []int32{Target}
	for v := int32(Target) + 1; v <= MaxTranslated; v++ {
		vs = append(vs, v)
	}
	for v := range translators {
		vs = append(vs, v)
	}
	return vs
}

// itoa is a tiny stdlib-free int→string for the panic message above (protocol is a
// leaf and avoids pulling strconv into its import set unnecessarily).
func itoa(n int32) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
