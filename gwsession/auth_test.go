package gwsession

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/tachyne/tachyne-common/attach"
	"github.com/tachyne/tachyne-common/protocol"
)

// fakeSessions is a session service that answers hasJoined for one expected
// server hash, as Mojang's does, or with a fixed status.
type fakeSessions struct {
	mu     sync.Mutex
	want   string // the server hash a genuine client registered
	status int    // non-zero: answer with this status instead
	asked  string // the username last asked about
}

func (f *fakeSessions) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.URL.Path != "/session/minecraft/hasJoined" {
		http.NotFound(w, r)
		return
	}
	f.asked = r.URL.Query().Get("username")
	if f.status != 0 {
		w.WriteHeader(f.status)
		return
	}
	if r.URL.Query().Get("serverId") != f.want {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{
		"id":   "069a79f444e94726a5befca90e38aaf5",
		"name": "Notch", // the account's own spelling, whatever the client typed
		"properties": []map[string]string{
			{"name": "textures", "value": "dGV4dHVyZXM=", "signature": "c2ln"},
		},
	})
}

func (f *fakeSessions) expect(hash string) {
	f.mu.Lock()
	f.want = hash
	f.mu.Unlock()
}

func onlineServer(t *testing.T, sessions http.Handler) string {
	t.Helper()
	hs := httptest.NewServer(sessions)
	t.Cleanup(hs.Close)
	auth, err := NewAuthenticator(hs.URL)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Name: "gw-test", VersionName: "26.3", Proto: 777, MinProto: 776, MaxProto: 777, Auth: auth}
	go s.Serve(ln)
	t.Cleanup(func() { ln.Close() })
	return ln.Addr().String()
}

// clientLogin plays a vanilla client through Login Start and the encryption
// handshake, then returns the first packet the server sends after the
// switch, read through the cipher. register is called with the server hash
// the client would send to Mojang's joinServer.
func clientLogin(t *testing.T, addr, name string, register func(hash string)) protocol.Packet {
	t.Helper()
	c, br := dialHandshake(t, addr, 777, intentLogin)
	start := protocol.AppendString(nil, name)
	start = append(start, make([]byte, 16)...) // the client's own uuid: ignored
	if err := protocol.WritePacket(c, loginPktStart, start); err != nil {
		t.Fatal(err)
	}
	pkt, err := protocol.ReadPacket(br)
	if err != nil {
		t.Fatal(err)
	}
	if pkt.ID != loginPktEncryptionRequest {
		t.Fatalf("expected the encryption request, got %#x", pkt.ID)
	}
	body := bytes.NewReader(pkt.Data)
	serverID, _ := protocol.ReadString(body)
	pubDER, _ := readByteArray(body)
	challenge, _ := readByteArray(body)
	should, _ := body.ReadByte()
	if serverID != "" || len(challenge) != 4 || should != 1 {
		t.Fatalf("encryption request serverId=%q challenge=%d bytes authenticate=%d", serverID, len(challenge), should)
	}
	pubAny, err := x509.ParsePKIXPublicKey(pubDER)
	if err != nil {
		t.Fatalf("the public key is not X.509: %v", err)
	}
	pub := pubAny.(*rsa.PublicKey)
	if pub.N.BitLen() != 1024 {
		t.Errorf("key is %d bits, vanilla's is 1024", pub.N.BitLen())
	}
	secret := make([]byte, 16)
	rand.Read(secret)
	register(protocol.AuthDigest("", secret, pubDER))
	encSecret, _ := rsa.EncryptPKCS1v15(rand.Reader, pub, secret)
	encChallenge, _ := rsa.EncryptPKCS1v15(rand.Reader, pub, challenge)
	resp := appendByteArray(nil, encSecret)
	resp = appendByteArray(resp, encChallenge)
	if err := protocol.WritePacket(c, loginPktEncryptionResponse, resp); err != nil {
		t.Fatal(err)
	}
	ec, err := protocol.NewEncryptedConn(c, secret)
	if err != nil {
		t.Fatal(err)
	}
	out, err := protocol.ReadPacket(bufio.NewReader(ec))
	if err != nil {
		t.Fatalf("reading through the cipher: %v", err)
	}
	return *out
}

func disconnectText(t *testing.T, pkt protocol.Packet) string {
	t.Helper()
	if pkt.ID != loginPktDisconnect {
		t.Fatalf("expected a login disconnect, got %#x", pkt.ID)
	}
	s, err := protocol.ReadString(pkt.Body())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// A genuine client is authenticated: the session service's profile — its
// spelling of the name — is who logs in. With no world wired the gateway
// says so by that name, over the encrypted stream.
func TestOnlineLoginAuthenticates(t *testing.T) {
	f := &fakeSessions{}
	addr := onlineServer(t, f)
	pkt := clientLogin(t, addr, "notch", f.expect)
	if text := disconnectText(t, pkt); !strings.Contains(text, "Notch") {
		t.Errorf("after authentication the gateway said %s, want it to greet the account name Notch", text)
	}
	if f.asked != "notch" {
		t.Errorf("hasJoined asked about %q, want the name the client sent", f.asked)
	}
}

// A client that did not register this join with the session service (a
// cracked client, or someone using another player's name) is refused.
func TestOnlineLoginRefusesUnverified(t *testing.T) {
	f := &fakeSessions{}
	addr := onlineServer(t, f)
	pkt := clientLogin(t, addr, "EdgeZA", func(string) {}) // never registered
	if text := disconnectText(t, pkt); !strings.Contains(text, msgUnverifiedUsername) {
		t.Errorf("an unverified client was told %s", text)
	}
}

// With the session service down every login fails, as vanilla's does.
func TestOnlineLoginAuthServersDown(t *testing.T) {
	f := &fakeSessions{status: http.StatusServiceUnavailable}
	addr := onlineServer(t, f)
	pkt := clientLogin(t, addr, "EdgeZA", f.expect)
	if text := disconnectText(t, pkt); !strings.Contains(text, msgAuthServersDown) {
		t.Errorf("with the session service down the client was told %s", text)
	}
}

// A mangled challenge is a protocol error: the connection just closes.
func TestOnlineLoginBadChallenge(t *testing.T) {
	addr := onlineServer(t, &fakeSessions{})
	c, br := dialHandshake(t, addr, 777, intentLogin)
	protocol.WritePacket(c, loginPktStart, append(protocol.AppendString(nil, "EdgeZA"), make([]byte, 16)...))
	pkt, err := protocol.ReadPacket(br)
	if err != nil {
		t.Fatal(err)
	}
	body := bytes.NewReader(pkt.Data)
	protocol.ReadString(body)
	pubDER, _ := readByteArray(body)
	pubAny, _ := x509.ParsePKIXPublicKey(pubDER)
	pub := pubAny.(*rsa.PublicKey)
	secret := make([]byte, 16)
	encSecret, _ := rsa.EncryptPKCS1v15(rand.Reader, pub, secret)
	encWrong, _ := rsa.EncryptPKCS1v15(rand.Reader, pub, []byte{9, 9, 9, 9})
	protocol.WritePacket(c, loginPktEncryptionResponse, appendByteArray(appendByteArray(nil, encSecret), encWrong))
	if _, err := protocol.ReadPacket(br); err == nil {
		t.Error("a wrong challenge got an answer instead of a closed connection")
	}
}

// Login Success carries the profile's properties (the skin).
func TestProfilePropsOnTheWire(t *testing.T) {
	b := appendProfileProps(nil, []attach.Property{{Name: "textures", Value: "v", Signature: "s"}, {Name: "x", Value: "y"}})
	r := bytes.NewReader(b)
	n, _ := protocol.ReadVarInt(r)
	name, _ := protocol.ReadString(r)
	val, _ := protocol.ReadString(r)
	has, _ := r.ReadByte()
	sig, _ := protocol.ReadString(r)
	name2, _ := protocol.ReadString(r)
	protocol.ReadString(r)
	has2, _ := r.ReadByte()
	if n != 2 || name != "textures" || val != "v" || has != 1 || sig != "s" || name2 != "x" || has2 != 0 || r.Len() != 0 {
		t.Errorf("props encoded as %x", b)
	}
}

func TestValidPlayerName(t *testing.T) {
	for name, want := range map[string]bool{"EdgeZA": true, "a_b": true, "": false, "seventeen_chars_x": false, "has space": false, "naïve": false} {
		if got := validPlayerName(name); got != want {
			t.Errorf("validPlayerName(%q) = %v", name, got)
		}
	}
}
