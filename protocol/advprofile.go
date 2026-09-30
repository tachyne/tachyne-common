package protocol

import (
	"bytes"
	"io"
	"unicode/utf8"
)

// Two item components whose payloads the slot copier has to walk field by
// field: a player head's profile (ResolvableProfile) and adventure mode's
// can_break / can_place_on (AdventureModePredicate).

// profileEither reports whether a version's profile component is 1.21.9's
// shape: ByteBufCodecs.either(GAME_PROFILE, Partial) — a Boolean, then a
// whole game profile (true) or a partial one (false) — followed by a
// PlayerSkin.Patch. 1.21.5's (the canonical form) is the partial alone:
// an optional name, an optional UUID and the property map.
func profileEither(version int32) bool { return version >= 773 }

// maxProfileProperties is GAME_PROFILE_PROPERTIES' MAX_PROPERTIES.
const maxProfileProperties = 16

// partialProfile is one profile as read off the wire: ResolvableProfile's
// Partial (name and id each optional), with the property map kept as the
// bytes it was read as.
type partialProfile struct {
	hasName bool
	name    string
	hasID   bool
	id      [16]byte
	props   []byte // GAME_PROFILE_PROPERTIES
}

// readPlayerName reads ByteBufCodecs.PLAYER_NAME (stringUtf8(16)).
func readPlayerName(r *bytes.Reader) (string, bool) {
	s, err := ReadString(r)
	if err != nil || utf8.RuneCountInString(s) > 16 {
		return "", false
	}
	return s, true
}

// readPartialProfile reads ResolvableProfile.Partial.STREAM_CODEC (and
// 1.21.5's ResolvableProfile.STREAM_CODEC, the same fields): an optional
// name, an optional UUID, the properties.
func readPartialProfile(r *bytes.Reader) (partialProfile, bool) {
	var p partialProfile
	has, err := r.ReadByte()
	if err != nil || has > 1 {
		return p, false
	}
	if has == 1 {
		if p.name, p.hasName = readPlayerName(r); !p.hasName {
			return p, false
		}
	}
	if has, err = r.ReadByte(); err != nil || has > 1 {
		return p, false
	}
	if has == 1 {
		if _, err := io.ReadFull(r, p.id[:]); err != nil {
			return p, false
		}
		p.hasID = true
	}
	return p, copyProfileProperties(r, &p.props)
}

// readGameProfile reads ByteBufCodecs.GAME_PROFILE: the UUID, the name and
// the properties, none of them optional.
func readGameProfile(r *bytes.Reader) (partialProfile, bool) {
	p := partialProfile{hasName: true, hasID: true}
	if _, err := io.ReadFull(r, p.id[:]); err != nil {
		return p, false
	}
	var ok bool
	if p.name, ok = readPlayerName(r); !ok {
		return p, false
	}
	return p, copyProfileProperties(r, &p.props)
}

func (p partialProfile) appendPartial(b []byte) []byte {
	b = AppendBool(b, p.hasName)
	if p.hasName {
		b = AppendString(b, p.name)
	}
	b = AppendBool(b, p.hasID)
	if p.hasID {
		b = append(b, p.id[:]...)
	}
	return append(b, p.props...)
}

func (p partialProfile) appendGameProfile(b []byte) []byte {
	b = append(b, p.id[:]...)
	b = AppendString(b, p.name)
	return append(b, p.props...)
}

// copyProfileProperties copies GAME_PROFILE_PROPERTIES: a count (at most
// sixteen), then each property's name, value and nullable signature.
func copyProfileProperties(r *bytes.Reader, out *[]byte) bool {
	n, err := ReadVarInt(r)
	if err != nil || n < 0 || n > maxProfileProperties {
		return false
	}
	*out = AppendVarInt(*out, n)
	for i := int32(0); i < n; i++ {
		for j := 0; j < 2; j++ { // name, value
			s, err := ReadString(r)
			if err != nil {
				return false
			}
			*out = AppendString(*out, s)
		}
		has, err := r.ReadByte()
		if err != nil || has > 1 {
			return false
		}
		*out = append(*out, has)
		if has == 1 {
			s, err := ReadString(r)
			if err != nil {
				return false
			}
			*out = AppendString(*out, s)
		}
	}
	return true
}

// skipSkinPatch reads past a PlayerSkin.Patch: three optional
// ClientAsset.ResourceTextures (an Identifier each — body, cape, elytra)
// and an optional PlayerModelType (a Boolean: slim).
func skipSkinPatch(r *bytes.Reader) bool {
	for i := 0; i < 3; i++ {
		has, err := r.ReadByte()
		if err != nil || has > 1 {
			return false
		}
		if has == 1 {
			if _, err := ReadString(r); err != nil {
				return false
			}
		}
	}
	has, err := r.ReadByte()
	if err != nil || has > 1 {
		return false
	}
	if has == 1 {
		slim, err := r.ReadByte()
		if err != nil || slim > 1 {
			return false
		}
	}
	return true
}

// copyProfile copies a profile component. Clientbound to a 1.21.9+ client
// the canonical partial goes out as vanilla's own encoding would carry it:
// a profile with both a name and an id is a whole game profile (the left
// of ResolvableProfile.CODEC's mapEither, which needs both), anything else
// a partial — which the client, as vanilla's create does, treats as a
// dynamic profile to look up when it holds exactly one of the two — and
// the skin patch is empty. Serverbound the either comes back to the
// partial; the skin patch has no canonical field and is read past.
func copyProfile(r *bytes.Reader, out *[]byte, version int32, serverbound bool) bool {
	if !profileEither(version) {
		p, ok := readPartialProfile(r)
		if !ok {
			return false
		}
		*out = p.appendPartial(*out)
		return true
	}
	if serverbound {
		full, err := r.ReadByte()
		if err != nil || full > 1 {
			return false
		}
		var p partialProfile
		var ok bool
		if full == 1 {
			p, ok = readGameProfile(r)
		} else {
			p, ok = readPartialProfile(r)
		}
		if !ok || !skipSkinPatch(r) {
			return false
		}
		*out = p.appendPartial(*out)
		return true
	}
	p, ok := readPartialProfile(r)
	if !ok {
		return false
	}
	if p.hasName && p.hasID {
		*out = append(*out, 1)
		*out = p.appendGameProfile(*out)
	} else {
		*out = append(*out, 0)
		*out = p.appendPartial(*out)
	}
	*out = append(*out, 0, 0, 0, 0) // PlayerSkin.Patch.EMPTY: four absent fields
	return true
}

// maxAdventurePredicates bounds the BlockPredicate list the copier walks
// (the codec's list is unbounded).
const maxAdventurePredicates = 64

// copyAdventurePredicate copies an AdventureModePredicate — a list of
// BlockPredicates, the same layout from 1.21.5 through 26.3. Each is an
// optional HolderSet<Block>, optional StatePropertiesPredicate, optional
// NbtPredicate (a compound tag) and a DataComponentMatchers. Block ids are
// registry ids and translate like items do; the component matchers must be
// empty (the exact list's typed values and the partial predicates are not
// walked), and anything else is refused rather than guessed at.
func copyAdventurePredicate(r *bytes.Reader, out *[]byte, version int32, serverbound bool) bool {
	n, err := ReadVarInt(r)
	if err != nil || n < 0 || n > maxAdventurePredicates {
		return false
	}
	*out = AppendVarInt(*out, n)
	for i := int32(0); i < n; i++ {
		if !copyBlockPredicate(r, out, version, serverbound) {
			return false
		}
	}
	return true
}

func copyBlockPredicate(r *bytes.Reader, out *[]byte, version int32, serverbound bool) bool {
	// blocks: Optional<HolderSet<Block>>.
	has, err := r.ReadByte()
	if err != nil || has > 1 {
		return false
	}
	*out = append(*out, has)
	if has == 1 && !copyBlockHolderSet(r, out, version, serverbound) {
		return false
	}
	// properties: Optional<StatePropertiesPredicate>.
	if has, err = r.ReadByte(); err != nil || has > 1 {
		return false
	}
	*out = append(*out, has)
	if has == 1 && !copyStateProperties(r, out) {
		return false
	}
	// nbt: Optional<NbtPredicate> — ByteBufCodecs.COMPOUND_TAG.
	if has, err = r.ReadByte(); err != nil || has > 1 {
		return false
	}
	*out = append(*out, has)
	if has == 1 {
		if t, err := r.ReadByte(); err != nil || t != nbtCompound {
			return false
		}
		r.UnreadByte()
		if !copyNBTValue(r, out) {
			return false
		}
	}
	// components: DataComponentMatchers — the exact list, then the partial
	// predicate list. Only empty ones are walked.
	for j := 0; j < 2; j++ {
		c, err := ReadVarInt(r)
		if err != nil || c != 0 {
			return false
		}
		*out = AppendVarInt(*out, 0)
	}
	return true
}

// copyBlockHolderSet copies ByteBufCodecs.holderSet(Registries.BLOCK): a
// VarInt that is 0 for a tag (its Identifier follows) or the count + 1 of
// the block registry ids that follow. A block the other side has no id for
// is left out of the list: it cannot be named there.
func copyBlockHolderSet(r *bytes.Reader, out *[]byte, version int32, serverbound bool) bool {
	k, err := ReadVarInt(r)
	if err != nil || k < 0 || k-1 > 4096 {
		return false
	}
	if k == 0 {
		tag, err := ReadString(r)
		if err != nil || len(tag) > 32767 {
			return false
		}
		*out = AppendVarInt(*out, 0)
		*out = AppendString(*out, tag)
		return true
	}
	var ids []int32
	for i := int32(0); i < k-1; i++ {
		id, err := ReadVarInt(r)
		if err != nil || id < 0 {
			return false
		}
		switch {
		case serverbound && IDAdded(RegBlock, version, id):
		case serverbound:
			ids = append(ids, UnmapID(RegBlock, version, id))
		case IDPresent(RegBlock, version, id):
			ids = append(ids, RemapID(RegBlock, version, id))
		}
	}
	*out = AppendVarInt(*out, int32(len(ids))+1)
	for _, id := range ids {
		*out = AppendVarInt(*out, id)
	}
	return true
}

// copyStateProperties copies StatePropertiesPredicate.STREAM_CODEC: a list
// of (property name, ValueMatcher), the matcher an either of an exact
// value (true, one string) or a range (false, an optional min and an
// optional max string).
func copyStateProperties(r *bytes.Reader, out *[]byte) bool {
	n, err := ReadVarInt(r)
	if err != nil || n < 0 || n > 64 {
		return false
	}
	*out = AppendVarInt(*out, n)
	str := func() bool {
		s, err := ReadString(r)
		if err != nil {
			return false
		}
		*out = AppendString(*out, s)
		return true
	}
	opt := func() bool {
		has, err := r.ReadByte()
		if err != nil || has > 1 {
			return false
		}
		*out = append(*out, has)
		return has == 0 || str()
	}
	for i := int32(0); i < n; i++ {
		if !str() { // the property's name
			return false
		}
		exact, err := r.ReadByte()
		if err != nil || exact > 1 {
			return false
		}
		*out = append(*out, exact)
		if exact == 1 {
			if !str() {
				return false
			}
		} else if !opt() || !opt() {
			return false
		}
	}
	return true
}
