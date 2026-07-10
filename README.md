# tachyne-common

> tachyne is an unofficial fan project, not affiliated with Mojang,
> Microsoft, or Minecraft's developer/publisher in any way. See the
> Disclaimer at the bottom.

## Project status

**Work in progress.** tachyne is young and moving fast: a full survival game
runs today, but expect rough edges, missing vanilla features, and breaking
changes between updates. **Bug reports are genuinely useful** — please open a
GitHub Issue with your client version/edition and what you saw. Contributions
are welcome too: see [CONTRIBUTING.md](CONTRIBUTING.md).



Shared pure-Go module for the tachyne Minecraft cluster. This module is where
the Minecraft wire format lives — the world engine (`tachyne-world`) has no
wire code at all ("worlds are versionless"); gateways compose these packages
to terminate real clients.

## Packages

- **`attach/`** — the domain attach protocol between gateways and world pods:
  the engine's ONLY external interface. Framing `u32be len | u8 type |
  payload`; JSON payloads except Chunk (JSON header + zlib binary of raw
  block-state/light arrays) and Ping/Pong. The frame catalog is typed end to
  end: core session frames 0x01–0x09 (Hello/Welcome/Want/Chunk/Move/Time/
  Ping/Pong/Bye), clientbound domain events (entities 0x0a–0x10, dig/place/
  held/blockset 0x11–0x14, dimension/teleport/command 0x15–0x17, bossbar
  0x1a, survival 0x1b–0x1f, items/windows 0x20–0x27, sound/particles/worldfx
  0x28–0x2a, misc 0x2b–0x33, entity-status/swing 0x40/0x41), and serverbound
  typed actions 0x34–0x3f. `entities.go` (consts + struct docs) is the
  protocol reference — keep it current when adding frames. Events carry
  ABSOLUTE positions; per-viewer delta math is the renderer's job. Codec
  rules that bit us once: one json tag PER field, no custom marshalers on
  embedded structs (a promoted MarshalJSON silently dropped `on_ground`).

- **`render770/`** — the shared canonical-770 (Minecraft 1.21.5) renderer +
  serverbound parsers for the attach events. `EntityView` is the per-viewer
  entity tracker: relative i16-delta moves against what THIS viewer actually
  rendered, absolute resync on first sight / ≥7.5-block jumps / every 40th
  move, `NoSync` for entities that must never get `sync_entity_position`
  (the 776 dragon constraint). Family renderers: presence (sanitizing chat
  NBT, boss bars, time), survival (health/XP/effects/hurt/death), items
  (ItemStack with opaque canonical components, equipment, windows,
  entity metadata), effects (inline-by-name sounds, particles, world FX,
  block set), misc (game events, abilities, passengers, vehicles, velocity,
  trades, difficulty, command tree, respawn). `parse.go` holds the `SID*`
  serverbound packet-id consts + `Parse*` decoders gateways use to lift 770
  serverbound packets into typed action frames. **Tests are byte oracles**:
  the engine's deleted packet builders were copied in verbatim as
  expected-output generators — a renderer change that shifts bytes fails
  loudly. Add an oracle (or strict re-parser) test with every new renderer.

- **`protocol/`** — the 770 wire/encoding substrate: VarInt/String, packet
  framing (plain + zlib-compressed), network NBT writer, paletted containers,
  positions, generated registries/damage-types/items tables, config-phase
  composition shared by all gateways (`ConfigRegistryPackets(proto)`,
  `UpdateTagsPacket(proto)` — full 1.21.5 AND 26.x tag data —
  `BrandPayload()`, `FeatureFlags()`), and the chained multi-version
  translation layer (`Translator`, `TranslatorFor(proto)`,
  `translate_chain.go`, generated protomaps incl. 26.x): gateways above 770
  render canonical bytes then translate at the client boundary. Generators
  for the `*_gen.go` files live in `tachyne-world`'s `scripts/` and write
  here.

- **`access/`** — client for the tachyne-access policy service: fail-closed
  login verdicts (`Check`), 30 s verdict cache.

- **`proxyproto/`** — PROXY protocol v1 reader (ingress → gateway real
  client IPs).

## Consumers & workflow

Consumers: `tachyne-world` (engine: attach types only), `tachyne-gw-java-770`
(renders 770 for 1.21.5–1.21.8), `tachyne-gw-java-776` (render770 +
`TranslatorFor(776)` for 26.2), `tachyne-gw-bedrock` (Bedrock render),
`tachyne-ingress` (proxyproto). Fetch with `GOPRIVATE=<your-git-host>`
(anonymous HTTPS read works on LAN).

Change protocol here → pin the new sha in every consumer
(`GOFLAGS=-mod=mod go get
tachyne-common@<sha>`), `go test -race` each,
deploy world before gateways. New client-visible features enter as: typed
attach frame here → renderer/parser in `render770/` (with oracle test) →
engine emission → gateway wiring.

## Credits

- **[PrismarineJS/minecraft-data](https://github.com/PrismarineJS/minecraft-data)** —
  canonical packet ids and field layouts (MIT).
- **[misode/mcmeta](https://github.com/misode/mcmeta)** — registry, tag and
  damage-type data reports.
- **[Minecraft Wiki](https://minecraft.wiki)** — protocol documentation
  (CC BY-NC-SA; factual reference).
- **[ViaVersion](https://github.com/ViaVersion/ViaVersion) / ViaBackwards** —
  cross-version packet and entity differences used as a factual reference for
  the translation chain and entity substitution (no code reused; GPL).

## Development transparency

tachyne is built by its maintainer working with an AI coding agent
(Anthropic's Claude): substantial portions of the implementation were written
by the model under human direction, and every change is reviewed, tested and
deployed by the maintainer. The project's engineering discipline is designed
for exactly this workflow — byte-oracle tests pin the wire format, full test
suites gate every image build, and real-client verification signs off
gameplay. Disclosed here for transparency; judge the code on its behavior.

## License

Licensed under the **Apache License, Version 2.0** — see [LICENSE](LICENSE)
and [NOTICE](NOTICE). Note §6: the license grants no rights to the tachyne
name or any trademarks.

## Disclaimer

tachyne is an unofficial, independent project. It is **not** affiliated with,
endorsed, sponsored, or approved by Mojang Studios, Mojang Synergies AB,
Microsoft Corporation, or any of their subsidiaries — the developer and
publisher of Minecraft have no involvement with this project. "Minecraft" is
a trademark of Mojang Synergies AB. This project contains no Minecraft game
code; all game behavior is independently reimplemented, and data tables are
built from openly licensed community datasets (see Credits).
