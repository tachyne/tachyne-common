package gwsession

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// auth.go is online mode: vanilla's login handshake against Mojang's session
// service (ServerLoginPacketListenerImpl.handleHello/handleKey). The gateway
// sends its RSA public key and a random challenge (Encryption Request), the
// client answers with the shared secret and the challenge, both encrypted to
// that key (Encryption Response), the stream switches to AES/CFB-8 under the
// secret, and the session service is asked whether this player really joined
// with that server hash. Its answer is the account's own UUID, the name as
// the account spells it, and the textures (the skin).

// Login-state packet ids of the handshake.
const (
	loginPktEncryptionRequest  = 0x01 // clientbound ClientboundHelloPacket
	loginPktEncryptionResponse = 0x01 // serverbound ServerboundKeyPacket
)

// DefaultSessionServer is Mojang's session service.
const DefaultSessionServer = "https://sessionserver.mojang.com"

// The vanilla disconnect texts (multiplayer.disconnect.*).
const (
	msgUnverifiedUsername = "Failed to verify username!"
	msgAuthServersDown    = "Authentication servers are down. Please try again later. Sorry!"
)

// authTimeout bounds the session-service round trip.
const authTimeout = 10 * time.Second

// Profile is an authenticated game profile.
type Profile struct {
	UUID  [16]byte
	Name  string
	Props []attach.Property // "textures" = the skin and cape, signed by Mojang
}

// UUIDString is the dashed form of the profile's id.
func (p Profile) UUIDString() string {
	u := p.UUID
	return fmt.Sprintf("%x-%x-%x-%x-%x", u[0:4], u[4:6], u[6:8], u[8:10], u[10:16])
}

// Authenticator holds the gateway's key pair (vanilla makes one 1024-bit
// pair at startup) and the session service it asks.
type Authenticator struct {
	key           *rsa.PrivateKey
	pub           []byte // X.509 SubjectPublicKeyInfo, as the client expects it
	SessionServer string
	HTTP          *http.Client
}

// NewAuthenticator makes the key pair. sessionServer "" = Mojang's.
func NewAuthenticator(sessionServer string) (*Authenticator, error) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		return nil, err
	}
	pub, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, err
	}
	if sessionServer == "" {
		sessionServer = DefaultSessionServer
	}
	return &Authenticator{key: key, pub: pub, SessionServer: sessionServer, HTTP: &http.Client{Timeout: authTimeout}}, nil
}

// errAuthDown marks a session service that could not be reached, as opposed
// to one that said no.
var errAuthDown = errors.New("session service unavailable")

// authError carries the text the player is disconnected with.
type authError struct {
	reason string
	err    error
}

func (e *authError) Error() string { return e.reason + ": " + e.err.Error() }

// cryptConn is the client connection once the handshake has switched it to
// AES/CFB-8: reads and writes go through the cipher, everything else
// (deadlines, addresses, Close) is the plain connection's.
type cryptConn struct {
	net.Conn
	enc *protocol.EncryptedConn
}

func (c *cryptConn) Read(p []byte) (int, error)  { return c.enc.Read(p) }
func (c *cryptConn) Write(p []byte) (int, error) { return c.enc.Write(p) }

// Login runs the encryption handshake and the session check for a player who
// sent Login Start as name. It returns the encrypted connection and a reader
// over it — every later packet, a disconnect included, must go through them
// — and the authenticated profile. On failure the returned connection is the
// one a disconnect must be written to (encrypted once the switch happened),
// and the error is an *authError naming the vanilla reason, or a plain error
// for a protocol violation (no reason: vanilla drops the connection).
func (a *Authenticator) Login(br *bufio.Reader, c net.Conn, name string) (net.Conn, *bufio.Reader, Profile, error) {
	challenge := make([]byte, 4)
	if _, err := rand.Read(challenge); err != nil {
		return c, br, Profile{}, err
	}
	req := protocol.AppendString(nil, "") // serverId: empty since 1.7
	req = appendByteArray(req, a.pub)
	req = appendByteArray(req, challenge)
	req = protocol.AppendBool(req, true) // shouldAuthenticate
	if err := protocol.WritePacket(c, loginPktEncryptionRequest, req); err != nil {
		return c, br, Profile{}, err
	}
	pkt, err := protocol.ReadPacket(br)
	if err != nil {
		return c, br, Profile{}, err
	}
	if pkt.ID != loginPktEncryptionResponse {
		return c, br, Profile{}, fmt.Errorf("expected the encryption response, got packet %#x", pkt.ID)
	}
	body := bytes.NewReader(pkt.Data)
	keyBytes, err1 := readByteArray(body)
	encChallenge, err2 := readByteArray(body)
	if err1 != nil || err2 != nil {
		return c, br, Profile{}, errors.New("malformed encryption response")
	}
	got, err := rsa.DecryptPKCS1v15(nil, a.key, encChallenge)
	if err != nil || !bytes.Equal(got, challenge) {
		return c, br, Profile{}, errors.New("encryption response: challenge mismatch") // vanilla: "Protocol error"
	}
	secret, err := rsa.DecryptPKCS1v15(nil, a.key, keyBytes)
	if err != nil || len(secret) != 16 {
		return c, br, Profile{}, errors.New("encryption response: bad shared secret")
	}
	if br.Buffered() > 0 {
		// The client sends nothing more until it hears back; plaintext bytes
		// already buffered would be read as ciphertext.
		return c, br, Profile{}, errors.New("client sent data before the encryption switch")
	}
	enc, err := protocol.NewEncryptedConn(c, secret)
	if err != nil {
		return c, br, Profile{}, err
	}
	ec := &cryptConn{Conn: c, enc: enc}
	ebr := bufio.NewReader(ec)

	prof, err := a.hasJoined(name, protocol.AuthDigest("", secret, a.pub))
	switch {
	case errors.Is(err, errAuthDown):
		return ec, ebr, Profile{}, &authError{msgAuthServersDown, err}
	case err != nil:
		return ec, ebr, Profile{}, &authError{msgUnverifiedUsername, err}
	}
	return ec, ebr, prof, nil
}

// hasJoined is the session service's /session/minecraft/hasJoined: a
// profile when the player joined with this server hash, 204 when not.
func (a *Authenticator) hasJoined(name, serverHash string) (Profile, error) {
	u := a.SessionServer + "/session/minecraft/hasJoined?username=" + url.QueryEscape(name) +
		"&serverId=" + url.QueryEscape(serverHash)
	ctx, cancel := context.WithTimeout(context.Background(), authTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Profile{}, err
	}
	resp, err := a.HTTP.Do(req)
	if err != nil {
		return Profile{}, fmt.Errorf("%w: %v", errAuthDown, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusOK:
	case resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests:
		return Profile{}, fmt.Errorf("%w: HTTP %d", errAuthDown, resp.StatusCode)
	default: // 204 No Content: this player did not join with this hash
		return Profile{}, fmt.Errorf("session service: HTTP %d", resp.StatusCode)
	}
	var out struct {
		ID         string            `json:"id"`
		Name       string            `json:"name"`
		Properties []attach.Property `json:"properties"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&out); err != nil {
		return Profile{}, fmt.Errorf("session service: %v", err)
	}
	id, err := hex.DecodeString(out.ID)
	if err != nil || len(id) != 16 || out.Name == "" {
		return Profile{}, fmt.Errorf("session service: bad profile %q/%q", out.ID, out.Name)
	}
	var p Profile
	copy(p.UUID[:], id)
	p.Name, p.Props = out.Name, out.Properties
	return p, nil
}

func appendByteArray(b, data []byte) []byte {
	b = protocol.AppendVarInt(b, int32(len(data)))
	return append(b, data...)
}

func readByteArray(r *bytes.Reader) ([]byte, error) {
	n, err := protocol.ReadVarInt(r)
	if err != nil {
		return nil, err
	}
	if n < 0 || int(n) > r.Len() {
		return nil, errors.New("byte array overruns the packet")
	}
	out := make([]byte, n)
	_, err = io.ReadFull(r, out)
	return out, err
}
