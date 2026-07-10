package protocol

// Known Java Edition protocol version numbers. The handshake carries the
// client's protocol version; a ViaVersion-style translation layer maps a
// client version onto the core's canonical version (Target). We currently
// implement Target natively and translate other versions toward it.
const (
	V1_21_10 = 773
	V1_21_5  = 770 // current canonical core version
	V1_21    = 767
	V1_20_1  = 763
	V1_8_9   = 47
)

// Target is the canonical protocol version the game core implements natively.
// Connections on other versions are translated to/from this by per-connection
// adapters (see translate.go: the Translator seam). We implement 1.21.5 (770)
// because it is the newest version with a complete, verified registry +
// packet-layout dump available to source from.
//
// Adding support for another client version (the goal-#3 path):
//  1. Create internal/protocol/v<ver>.go defining a Translator whose Version()
//     returns that client protocol number.
//  2. Implement Clientbound/Serverbound: rewrite the packets that DIFFER from
//     Target between that version and 770 (IDs and/or field layouts), state by
//     state. Identical packets pass through unchanged. Source the diff from
//     minecraft-data (covers up to 774) or, for versions it lacks, the
//     minecraft.wiki protocol pages / ViaVersion's public diff packages as a
//     REFERENCE (read, don't copy — they are GPL; protocol facts are not).
//  3. register() it from the file's init(). TranslatorFor then serves that
//     version automatically; unregistered versions are rejected at login with a
//     clear message instead of a cryptic client-side decode error.
const Target = V1_21_5

// MCVersion is the human-readable game version matching Target. It is sent as
// the minecraft:core known-pack version so the client uses its built-in
// registry content during Configuration.
const MCVersion = "1.21.5"
