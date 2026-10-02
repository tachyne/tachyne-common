package gwsession

// securechat.go is the per-connection half of vanilla secure chat, kept where
// ServerGamePacketListenerImpl keeps it: the player's validated chat session
// and message-chain decoder (RemoteChatSession, SignedMessageChain.Decoder),
// the last-seen window the client acknowledges (LastSeenMessagesValidator),
// the signature cache that packs what the client is sent
// (MessageSignatureCache) and the running chat index. The world relays signed
// messages to every recipient; each recipient's gateway renders them here.

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"math"
	"net/http"
	"sync"
	"time"

	attach "github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
	"github.com/tachyne/tachyne-common/render770"
)

// DefaultServicesKeysURL is where Mojang publishes the keys that sign
// players' profile public keys (authlib MinecraftServicesKeyInfo).
const DefaultServicesKeysURL = "https://api.minecraftservices.com/publickeys"

// ServicesKeys is the Minecraft Services key set's PROFILE_KEY half (the
// response's playerCertificateKeys): the keys a chat session's key signature
// is checked against, with SHA1withRSA. Fetched once and kept; a failed
// fetch is retried at most once a minute. With no keys the server cannot
// validate profile keys, so — as vanilla's enforceSecureProfile — it neither
// enforces secure chat nor accepts chat sessions.
type ServicesKeys struct {
	URL  string
	HTTP *http.Client

	mu      sync.Mutex
	keys    []*rsa.PublicKey
	lastTry time.Time
}

// servicesRetry spaces fetch attempts while the key set is still empty.
const servicesRetry = time.Minute

// NewServicesKeys makes a key set that fetches from url ("" = the default).
func NewServicesKeys(url string) *ServicesKeys {
	if url == "" {
		url = DefaultServicesKeysURL
	}
	return &ServicesKeys{URL: url, HTTP: &http.Client{Timeout: authTimeout}}
}

// StaticServicesKeys is a key set that never fetches (tests, pinned keys).
func StaticServicesKeys(keys ...*rsa.PublicKey) *ServicesKeys {
	return &ServicesKeys{keys: keys, lastTry: time.Now().Add(1000 * time.Hour)}
}

// Prefetch loads the key set in the background.
func (k *ServicesKeys) Prefetch() {
	if k != nil {
		go k.load()
	}
}

// CanValidate reports whether any profile-key signing key is known
// (Services.canValidateProfileKeys).
func (k *ServicesKeys) CanValidate() bool { return k != nil && len(k.load()) > 0 }

func (k *ServicesKeys) load() []*rsa.PublicKey {
	k.mu.Lock()
	defer k.mu.Unlock()
	if len(k.keys) > 0 || time.Since(k.lastTry) < servicesRetry || k.URL == "" {
		return k.keys
	}
	k.lastTry = time.Now()
	keys, err := fetchServicesKeys(k.HTTP, k.URL)
	if err != nil {
		log.Printf("secure chat: services keys unavailable: %v", err)
		return k.keys
	}
	k.keys = keys
	return k.keys
}

// fetchServicesKeys reads the publickeys document: base64 X.509 keys under
// playerCertificateKeys[].publicKey.
func fetchServicesKeys(client *http.Client, url string) ([]*rsa.PublicKey, error) {
	ctx, cancel := context.WithTimeout(context.Background(), authTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("services keys: " + resp.Status)
	}
	var doc struct {
		PlayerCertificateKeys []servicesKeyData `json:"playerCertificateKeys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, err
	}
	return parseServicesKeys(doc.PlayerCertificateKeys)
}

// servicesKeyData is one KeyData entry: a base64 X.509 public key.
type servicesKeyData struct {
	PublicKey string `json:"publicKey"`
}

func parseServicesKeys(list []servicesKeyData) ([]*rsa.PublicKey, error) {
	var out []*rsa.PublicKey
	for _, e := range list {
		der, err := base64.StdEncoding.DecodeString(e.PublicKey)
		if err != nil {
			return nil, err
		}
		pub, err := parseRSAKey(der)
		if err != nil {
			return nil, err
		}
		out = append(out, pub)
	}
	return out, nil
}

var errNotRSA = errors.New("not an RSA key")

func parseRSAKey(der []byte) (*rsa.PublicKey, error) {
	k, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, err
	}
	pub, ok := k.(*rsa.PublicKey)
	if !ok {
		return nil, errNotRSA
	}
	return pub, nil
}

// validProfileKey is ProfilePublicKey.Data.validateSignature: Mojang's
// signature over the profile id, the expiry in millis and the key's X.509
// bytes, verified with SHA1withRSA against any services key.
func (k *ServicesKeys) validProfileKey(profile [16]byte, s attach.ChatSession) bool {
	payload := make([]byte, 0, 24+len(s.Key))
	payload = append(payload, profile[:]...)
	payload = protocol.AppendI64(payload, s.ExpiresAt)
	payload = append(payload, s.Key...)
	sum := sha1.Sum(payload)
	for _, pub := range k.load() {
		if rsa.VerifyPKCS1v15(pub, crypto.SHA1, sum[:], s.KeySig) == nil {
			return true
		}
	}
	return false
}

// Translation keys of the secure-chat refusals, as vanilla words them.
const (
	keyExpiredPublicKey    = "multiplayer.disconnect.expired_public_key"
	keyInvalidKeySignature = "multiplayer.disconnect.invalid_public_key_signature"
	keyValidationFailed    = "multiplayer.disconnect.chat_validation_failed"
	keyIllegalCharacters   = "multiplayer.disconnect.illegal_characters"
	keyTooManyPending      = "multiplayer.disconnect.too_many_pending_chats"
	keyMissingProfileKey   = "chat.disabled.missingProfileKey"
	keyChainBroken         = "chat.disabled.chain_broken"
	keyExpiredProfileKey   = "chat.disabled.expiredProfileKey"
	keyInvalidSignature    = "chat.disabled.invalid_signature"
	keyOutOfOrderChat      = "chat.disabled.out_of_order_chat"
	keyInvalidCommandSig   = "chat.disabled.invalid_command_signature"
)

// maxPendingChats is sendPlayerChatMessage's cap on tracked, unacknowledged
// messages before the client is disconnected.
const maxPendingChats = 4096

// chatChain is SignedMessageChain with its key-based decoder: the next link
// the sender must sign (broken = no link, until a new session) and the
// newest timestamp seen.
type chatChain struct {
	sessionID [16]byte
	pub       *rsa.PublicKey
	expiresAt int64 // unix millis
	next      int32
	broken    bool
	lastTS    int64 // unix millis (Instant.EPOCH to start)
}

// unpack is SignedMessageChain.Decoder.unpack: it returns the link index the
// message was signed at, or the refusal's translation key.
func (c *chatChain) unpack(sender [16]byte, sig []byte, content string, ts, salt int64, lastSeen [][]byte, now time.Time) (int32, string) {
	index, _, refuse := c.unpackAny(sender, sig, []string{content}, ts, salt, lastSeen, now)
	return index, refuse
}

// unpackAny is unpack for a body whose content is one of several candidates:
// a signed command argument, whose value the gateway knows only as a suffix
// of the command line (see render770.MessageCandidates). The signature
// verifies against at most one of them; that one is the signed content.
// Every candidate is checked at the same link, which advances once.
func (c *chatChain) unpackAny(sender [16]byte, sig []byte, contents []string, ts, salt int64, lastSeen [][]byte, now time.Time) (int32, string, string) {
	if len(sig) != render770.SignatureBytes {
		return 0, "", keyMissingProfileKey
	}
	if c.expiresAt < now.UnixMilli() {
		return 0, "", keyExpiredProfileKey
	}
	if c.broken {
		return 0, "", keyChainBroken
	}
	if ts < c.lastTS {
		c.broken = true
		return 0, "", keyOutOfOrderChat
	}
	c.lastTS = ts
	index := c.next
	content, ok := "", false
	for _, cand := range contents {
		if verifyMessage(c.pub, sender, c.sessionID, index, cand, ts, salt, lastSeen, sig) {
			content, ok = cand, true
			break
		}
	}
	if !ok {
		c.broken = true
		return 0, "", keyInvalidSignature
	}
	if index == math.MaxInt32 { // SignedMessageLink.advance: the chain ends
		c.broken = true
	} else {
		c.next = index + 1
	}
	return index, content, ""
}

// messagePayload is what a chat signature covers (PlayerChatMessage.
// updateSignature): the version int 1, the link (sender, session, index),
// then the body — salt, the timestamp in whole seconds, the content's UTF-8
// length and bytes, the last-seen count and signatures.
func messagePayload(sender, session [16]byte, index int32, content string, ts, salt int64, lastSeen [][]byte) []byte {
	b := protocol.AppendI32(nil, 1)
	b = append(b, sender[:]...)
	b = append(b, session[:]...)
	b = protocol.AppendI32(b, index)
	b = protocol.AppendI64(b, salt)
	b = protocol.AppendI64(b, floorDiv(ts, 1000)) // Instant.getEpochSecond
	b = protocol.AppendI32(b, int32(len(content)))
	b = append(b, content...)
	b = protocol.AppendI32(b, int32(len(lastSeen)))
	for _, s := range lastSeen {
		b = append(b, s...)
	}
	return b
}

func floorDiv(a, b int64) int64 {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

// verifyMessage checks a message signature with SHA256withRSA.
func verifyMessage(pub *rsa.PublicKey, sender, session [16]byte, index int32, content string, ts, salt int64, lastSeen [][]byte, sig []byte) bool {
	if pub == nil {
		return false
	}
	sum := sha256.Sum256(messagePayload(sender, session, index, content, ts, salt, lastSeen))
	return rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig) == nil
}

// trackedSig is LastSeenTrackedEntry.
type trackedSig struct {
	sig     []byte
	pending bool
}

// lastSeenValidator is LastSeenMessagesValidator: every signed message the
// client was sent, in order, as a window the client's acknowledgements move
// through.
type lastSeenValidator struct {
	count       int
	tracked     []*trackedSig
	lastPending []byte
}

func newLastSeenValidator(count int) *lastSeenValidator {
	return &lastSeenValidator{count: count, tracked: make([]*trackedSig, count)}
}

func (v *lastSeenValidator) addPending(sig []byte) {
	if !bytes.Equal(sig, v.lastPending) {
		v.tracked = append(v.tracked, &trackedSig{sig: sig, pending: true})
		v.lastPending = sig
	}
}

var errLastSeen = errors.New("last seen validation failed")

func (v *lastSeenValidator) applyOffset(offset int32) error {
	max := len(v.tracked) - v.count
	if offset < 0 || int(offset) > max {
		return errLastSeen
	}
	v.tracked = append([]*trackedSig(nil), v.tracked[offset:]...)
	return nil
}

// applyUpdate is LastSeenMessagesValidator.applyUpdate: the acknowledged
// signatures, oldest first, once the window and checksum agree.
func (v *lastSeenValidator) applyUpdate(u render770.LastSeenUpdate) ([][]byte, error) {
	if err := v.applyOffset(u.Offset); err != nil {
		return nil, err
	}
	if u.Acknowledged>>uint(v.count) != 0 { // BitSet.length() > lastSeenCount
		return nil, errLastSeen
	}
	var seen [][]byte
	for i := 0; i < v.count; i++ {
		m := v.tracked[i]
		if u.Acknowledged&(1<<uint(i)) != 0 {
			if m == nil {
				return nil, errLastSeen
			}
			v.tracked[i] = &trackedSig{sig: m.sig}
			seen = append(seen, m.sig)
		} else {
			if m != nil && !m.pending {
				return nil, errLastSeen
			}
			v.tracked[i] = nil
		}
	}
	if u.Checksum != 0 && u.Checksum != lastSeenChecksum(seen) {
		return nil, errLastSeen
	}
	return seen, nil
}

// sigChecksum is MessageSignature.checksum: Arrays.hashCode over the bytes.
func sigChecksum(sig []byte) int32 {
	h := int32(1)
	for _, b := range sig {
		h = 31*h + int32(int8(b))
	}
	return h
}

// lastSeenChecksum is LastSeenMessages.computeChecksum (0 is reserved for
// "ignore", so it becomes 1).
func lastSeenChecksum(sigs [][]byte) byte {
	c := int32(1)
	for _, s := range sigs {
		c = 31*c + sigChecksum(s)
	}
	if b := byte(c); b != 0 {
		return b
	}
	return 1
}

// sigCache is MessageSignatureCache: the 128 signatures most recently sent
// to the client, which later packets name by index.
type sigCache struct {
	entries [128][]byte
}

func (c *sigCache) pack(sig []byte) render770.PackedSignature {
	for i, e := range c.entries {
		if e != nil && bytes.Equal(e, sig) {
			return render770.PackedSignature{ID: int32(i)}
		}
	}
	return render770.PackedSignature{ID: -1, Full: sig}
}

// push is MessageSignatureCache.push(body, signature): the body's last-seen
// entries then the signature go to the front, and what they displace shifts
// back unless it is among them.
func (c *sigCache) push(lastSeen [][]byte, sig []byte) {
	queue := make([][]byte, 0, len(lastSeen)+1)
	queue = append(queue, lastSeen...)
	if sig != nil {
		queue = append(queue, sig)
	}
	fresh := make(map[string]bool, len(queue))
	for _, s := range queue {
		fresh[string(s)] = true
	}
	for i := 0; len(queue) > 0 && i < len(c.entries); i++ {
		old := c.entries[i]
		c.entries[i] = queue[len(queue)-1] // removeLast
		queue = queue[:len(queue)-1]
		if old != nil && !fresh[string(old)] {
			queue = append([][]byte{old}, queue...) // addFirst
		}
	}
}

// chatState is one connection's secure-chat state. Its methods lock: the
// client reader applies acknowledgements while the world reader renders the
// messages that create them.
type chatState struct {
	mu        sync.Mutex
	profile   [16]byte
	keys      *ServicesKeys
	online    bool
	session   *attach.ChatSession
	chain     *chatChain
	lastSeen  *lastSeenValidator
	cache     sigCache
	nextIndex int32
	signable  render770.SignableIndex // the command tree's message arguments
}

func newChatState(profile [16]byte, online bool, keys *ServicesKeys) *chatState {
	return &chatState{profile: profile, online: online, keys: keys, lastSeen: newLastSeenValidator(render770.LastSeenMax)}
}

// reset is the fresh state of a new game listener (a rejoin after
// reconfiguration builds a new ServerGamePacketListenerImpl): no session or
// chain until the client sends its session again, an empty last-seen
// window, cache and index. The command tree's index stays until the world
// sends the tree again.
func (s *chatState) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.session = nil
	s.chain = nil
	s.lastSeen = newLastSeenValidator(render770.LastSeenMax)
	s.cache = sigCache{}
	s.nextIndex = 0
}

// enforce is MinecraftServer.enforceSecureProfile: online, and able to
// validate profile keys (enforce-secure-profile is always on here).
func (s *chatState) enforce() bool { return s.online && s.keys.CanValidate() }

func sameSession(a, b *attach.ChatSession) bool {
	return a != nil && b != nil && a.ExpiresAt == b.ExpiresAt &&
		bytes.Equal(a.Key, b.Key) && bytes.Equal(a.KeySig, b.KeySig)
}

// sessionUpdate is handleChatSessionUpdate. forward is the validated session
// the world should broadcast (nil = nothing changed or it was ignored); kick
// is a disconnect reason's translation key.
func (s *chatState) sessionUpdate(in attach.ChatSession) (forward *attach.ChatSession, kick string) {
	can := s.enforce() // may fetch the key set: before the lock
	s.mu.Lock()
	defer s.mu.Unlock()
	if sameSession(s.session, &in) {
		return nil, ""
	}
	if s.session != nil && in.ExpiresAt < s.session.ExpiresAt {
		return nil, keyExpiredPublicKey
	}
	if !can {
		log.Printf("secure chat: ignoring a chat session: no services public key")
		return nil, ""
	}
	if !s.keys.validProfileKey(s.profile, in) {
		return nil, keyInvalidKeySignature
	}
	pub, err := parseRSAKey(in.Key)
	if err != nil {
		return nil, keyInvalidKeySignature
	}
	sess := in
	s.session = &sess
	s.chain = &chatChain{sessionID: in.SessionID, pub: pub, expiresAt: in.ExpiresAt}
	return &sess, ""
}

// allowedChat is the inverse of isChatMessageIllegal
// (StringUtil.isAllowedChatCharacter on every UTF-16 unit).
func allowedChat(s string) bool {
	for _, r := range s {
		if r == 167 || r < 32 || r == 127 {
			return false
		}
	}
	return true
}

// chatResult is what one serverbound chat or signed command came to.
type chatResult struct {
	kick    string             // disconnect with this translation key
	refuse  string             // tell the player this (red) and drop the message
	signed  *attach.SignedChat // the signed half, for a signed message
	lastSee [][]byte           // the acknowledged signatures
	args    []attach.SignedArgument
}

// receiveChat is handleChat up to broadcastChatMessage: apply the last-seen
// update, check the characters, decode through the chain.
func (s *chatState) receiveChat(m render770.ChatMessage, now time.Time) chatResult {
	enforce := s.enforce() // may fetch the key set: before the lock
	s.mu.Lock()
	defer s.mu.Unlock()
	seen, err := s.lastSeen.applyUpdate(m.LastSeen)
	if err != nil {
		return chatResult{kick: keyValidationFailed}
	}
	if !allowedChat(m.Text) {
		return chatResult{kick: keyIllegalCharacters}
	}
	if s.chain == nil { // SignedMessageChain.Decoder.unsigned
		if enforce {
			return chatResult{refuse: keyMissingProfileKey}
		}
		return chatResult{lastSee: seen}
	}
	index, refuse := s.chain.unpack(s.profile, m.Signature, m.Text, m.Timestamp, m.Salt, seen, now)
	if refuse != "" {
		return chatResult{refuse: refuse}
	}
	return chatResult{lastSee: seen, signed: &attach.SignedChat{
		Index: index, Signature: m.Signature, Timestamp: m.Timestamp, Salt: m.Salt, LastSeen: seen,
	}}
}

// receiveSignedCommand is handleSignedChatCommand up to performCommand:
// the acknowledgements, the characters, then collectSignedArguments. The
// world owns the parser, so which arguments are signable comes from the
// command tree it sent (render770.SignableArguments, held in signable): a
// signature for an argument the command does not have breaks the chain, as
// vanilla's mismatch does; each named argument is unpacked through the
// chain against the command line's possible message values. With no
// signatures the command's message arguments are unsigned
// (collectUnsignedArguments), which a key-holding player or an enforcing
// server refuses.
func (s *chatState) receiveSignedCommand(c render770.ChatCommandSigned, now time.Time) chatResult {
	enforce := s.enforce() // may fetch the key set: before the lock
	s.mu.Lock()
	defer s.mu.Unlock()
	seen, err := s.lastSeen.applyUpdate(c.LastSeen)
	if err != nil {
		return chatResult{kick: keyValidationFailed}
	}
	if !allowedChat(c.Command) {
		return chatResult{kick: keyIllegalCharacters}
	}
	expected := s.signable.For(c.Command)
	if len(c.Arguments) == 0 {
		if len(expected) > 0 && (s.chain != nil || enforce) {
			return chatResult{refuse: keyMissingProfileKey}
		}
		return chatResult{lastSee: seen}
	}
	byName := map[string]render770.SignableArg{}
	for _, a := range expected {
		byName[a.Name] = a
	}
	var args []attach.SignedArgument
	done := map[string]bool{}
	for _, in := range c.Arguments {
		want, ok := byName[in.Name]
		if !ok || done[in.Name] {
			if s.chain != nil {
				s.chain.broken = true
			}
			return chatResult{refuse: keyInvalidCommandSig}
		}
		done[in.Name] = true
		if s.chain == nil { // SignedMessageChain.Decoder.unsigned
			if enforce {
				return chatResult{refuse: keyMissingProfileKey}
			}
			continue
		}
		cands := render770.MessageCandidates(c.Command)
		if skip := want.Depth - 1; skip > 0 && skip <= len(cands) {
			cands = cands[skip:] // the words before the message are other arguments
		}
		index, content, refuse := s.chain.unpackAny(s.profile, in.Signature, cands, c.Timestamp, c.Salt, seen, now)
		if refuse != "" {
			return chatResult{refuse: refuse}
		}
		args = append(args, attach.SignedArgument{Name: in.Name, Content: content, Chat: attach.SignedChat{
			Index: index, Signature: in.Signature, Timestamp: c.Timestamp, Salt: c.Salt, LastSeen: seen,
		}})
	}
	return chatResult{lastSee: seen, args: args}
}

// unsignedCommandRefused is performUnsignedChatCommand's check: an
// enforcing server refuses a plain chat_command whose parse reaches a
// signable argument (the client should have signed it).
func (s *chatState) unsignedCommandRefused(cmd string) bool {
	enforce := s.enforce()
	s.mu.Lock()
	defer s.mu.Unlock()
	return enforce && len(s.signable.For(cmd)) > 0
}

// setCommandTree indexes the command tree the world sent for its signable
// arguments. An unreadable tree leaves nothing signable.
func (s *chatState) setCommandTree(tree []byte) {
	idx, ok := render770.SignableArguments(tree)
	if !ok {
		log.Printf("secure chat: command tree unreadable: no signable arguments")
		idx = nil
	}
	s.mu.Lock()
	s.signable = idx
	s.mu.Unlock()
}

// ack is handleChatAck.
func (s *chatState) ack(offset int32) (kick string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastSeen.applyOffset(offset) != nil {
		return keyValidationFailed
	}
	return ""
}

// render is sendPlayerChatMessage: the recipient's next global index, the
// body packed against its cache, then the signature cached and tracked.
func (s *chatState) render(e attach.PlayerChat) (render770.Packet, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var packed []render770.PackedSignature
	for _, sig := range e.LastSeen {
		packed = append(packed, s.cache.pack(sig))
	}
	p := render770.PlayerChat(s.nextIndex, e, packed)
	s.nextIndex++
	if len(e.Signature) == render770.SignatureBytes {
		s.cache.push(e.LastSeen, e.Signature)
		s.lastSeen.addPending(e.Signature)
		if len(s.lastSeen.tracked) > maxPendingChats {
			return p, keyTooManyPending
		}
	}
	return p, ""
}

// renderDelete packs a delete_chat against the recipient's cache.
func (s *chatState) renderDelete(sig []byte) render770.Packet {
	s.mu.Lock()
	defer s.mu.Unlock()
	return render770.DeleteChat(s.cache.pack(sig))
}

// translated is a translatable text component's network NBT.
func translated(key string, red bool) []byte {
	raw := `{"translate":"` + key + `"}`
	if red {
		raw = `{"translate":"` + key + `","color":"red"}`
	}
	nbt, _ := protocol.TextComponentNBT([]byte(raw))
	return nbt
}

// kickTranslated sends the play disconnect with a translated reason.
func kickTranslated(cc *clientConn, key string) {
	cc.send(render770.IDDisconnect, translated(key, false))
}

// refuseChat tells the player, in red, why the message went nowhere
// (handleMessageDecodeFailure → sendSystemMessage).
func refuseChat(cc *clientConn, key string) {
	cc.send(render770.IDSystemChat, protocol.AppendBool(translated(key, true), false))
}
