package gwsession

import (
	"bufio"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tachyne/tachyne-common/access"
	"github.com/tachyne/tachyne-common/protocol"
)

func startTestServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{MOTD: "test motd", Name: "gw-test", VersionName: "26.2", Proto: 776, MinProto: 776, MaxProto: 776}
	go s.Serve(ln)
	t.Cleanup(func() { ln.Close() })
	return ln.Addr().String()
}

// dialHandshake opens a client connection and sends a handshake.
func dialHandshake(t *testing.T, addr string, proto int32, intent int32) (net.Conn, *bufio.Reader) {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	body := protocol.AppendVarInt(nil, proto)
	body = protocol.AppendString(body, "localhost")
	body = protocol.AppendU16(body, 25565)
	body = protocol.AppendVarInt(body, intent)
	if err := protocol.WritePacket(c, 0x00, body); err != nil {
		t.Fatal(err)
	}
	return c, bufio.NewReader(c)
}

func TestStatusPing(t *testing.T) {
	addr := startTestServer(t)
	c, br := dialHandshake(t, addr, 776, intentStatus)

	// Status request → JSON response pinned to this gateway's protocol.
	if err := protocol.WritePacket(c, statusPktResponse, nil); err != nil {
		t.Fatal(err)
	}
	pkt, err := protocol.ReadPacket(br)
	if err != nil {
		t.Fatal(err)
	}
	if pkt.ID != statusPktResponse {
		t.Fatalf("status response id: want %#x got %#x", statusPktResponse, pkt.ID)
	}
	payload, err := protocol.ReadString(pkt.Body())
	if err != nil {
		t.Fatal(err)
	}
	var st statusJSON
	if err := json.Unmarshal([]byte(payload), &st); err != nil {
		t.Fatalf("status JSON: %v\n%s", err, payload)
	}
	if st.Version.Protocol != 776 || st.Version.Name != "26.2" {
		t.Errorf("status version: got %s/%d want 26.2/776", st.Version.Name, st.Version.Protocol)
	}
	if st.Description.Text != "test motd" {
		t.Errorf("status motd: got %q", st.Description.Text)
	}

	// Ping → pong echoes the 8-byte payload.
	ping := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	if err := protocol.WritePacket(c, statusPktPong, ping); err != nil {
		t.Fatal(err)
	}
	pkt, err = protocol.ReadPacket(br)
	if err != nil {
		t.Fatal(err)
	}
	if pkt.ID != statusPktPong || len(pkt.Data) != 8 {
		t.Fatalf("pong: id=%#x len=%d", pkt.ID, len(pkt.Data))
	}
	got := pkt.Data
	for i := range ping {
		if got[i] != ping[i] {
			t.Fatalf("pong payload: want %v got %v", ping, got)
		}
	}
}

// readDisconnectReason reads a Login Disconnect and returns its text.
func readDisconnectReason(t *testing.T, br *bufio.Reader) string {
	t.Helper()
	pkt, err := protocol.ReadPacket(br)
	if err != nil {
		t.Fatal(err)
	}
	if pkt.ID != loginPktDisconnect {
		t.Fatalf("disconnect id: want %#x got %#x", loginPktDisconnect, pkt.ID)
	}
	payload, err := protocol.ReadString(pkt.Body())
	if err != nil {
		t.Fatal(err)
	}
	var reason struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(payload), &reason); err != nil {
		t.Fatalf("disconnect JSON: %v\n%s", err, payload)
	}
	return reason.Text
}

func TestLoginWrongProtocolIsRejected(t *testing.T) {
	addr := startTestServer(t)
	_, br := dialHandshake(t, addr, 770, intentLogin)
	reason := readDisconnectReason(t, br)
	if !strings.Contains(reason, "protocol 770") {
		t.Errorf("reason should name the client's protocol: %q", reason)
	}
}

func TestLoginPinnedProtocolPlaceholder(t *testing.T) {
	addr := startTestServer(t)
	c, br := dialHandshake(t, addr, 776, intentLogin)

	// Login Start: name + 16 uuid bytes.
	body := protocol.AppendString(nil, "wesley")
	body = append(body, make([]byte, 16)...)
	if err := protocol.WritePacket(c, loginPktStart, body); err != nil {
		t.Fatal(err)
	}
	reason := readDisconnectReason(t, br)
	if !strings.Contains(reason, "wesley") {
		t.Errorf("reason should greet the player by name: %q", reason)
	}
}

func TestProbeConnectionIsSilent(t *testing.T) {
	addr := startTestServer(t)
	// A kubelet-style TCP probe: connect, send nothing, close. Must not
	// wedge the server (subsequent real connections still work).
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	_, br := dialHandshake(t, addr, 770, intentLogin)
	if reason := readDisconnectReason(t, br); !strings.Contains(reason, "protocol 770") {
		t.Errorf("server broken after probe connection: %q", reason)
	}
}

func TestOfflineUUIDMatchesVanilla(t *testing.T) {
	// Known vanilla offline UUID for "Notch".
	if got := OfflineUUID("Notch"); got != "b50ad385-829d-3141-a216-7e7d7539ba7f" {
		t.Errorf("OfflineUUID(Notch) = %s", got)
	}
}

func TestLoginConsultsAccess(t *testing.T) {
	as := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Name, UUID, IP string }
		json.NewDecoder(r.Body).Decode(&req)
		if req.UUID != OfflineUUID(req.Name) {
			t.Errorf("gateway sent uuid %q for %q", req.UUID, req.Name)
		}
		if strings.EqualFold(req.Name, "griefer") {
			json.NewEncoder(w).Encode(map[string]any{"allow": false, "reason": "Banned: griefing", "roles": []string{}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"allow": true, "roles": []string{"op"}})
	}))
	t.Cleanup(as.Close)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{MOTD: "test", Name: "gw-test", VersionName: "26.2", Proto: 776, MinProto: 776, MaxProto: 776, Access: access.New(as.URL, "tok", time.Minute)}
	go s.Serve(ln)
	t.Cleanup(func() { ln.Close() })
	addr := ln.Addr().String()

	// Denied player gets the access reason.
	c, br := dialHandshake(t, addr, 776, intentLogin)
	body := protocol.AppendString(nil, "griefer")
	body = append(body, make([]byte, 16)...)
	protocol.WritePacket(c, loginPktStart, body)
	if reason := readDisconnectReason(t, br); !strings.Contains(reason, "griefing") {
		t.Errorf("denied login should carry the access reason: %q", reason)
	}

	// Allowed player still reaches the under-construction farewell.
	c, br = dialHandshake(t, addr, 776, intentLogin)
	body = protocol.AppendString(nil, "wesley")
	body = append(body, make([]byte, 16)...)
	protocol.WritePacket(c, loginPktStart, body)
	if reason := readDisconnectReason(t, br); !strings.Contains(reason, "under construction") {
		t.Errorf("allowed login should reach the placeholder: %q", reason)
	}
}

func TestLoginFailsClosedWhenAccessDown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{MOTD: "test", Name: "gw-test", VersionName: "26.2", Proto: 776, MinProto: 776, MaxProto: 776, Access: access.New("http://127.0.0.1:1", "tok", time.Minute)}
	go s.Serve(ln)
	t.Cleanup(func() { ln.Close() })

	c, br := dialHandshake(t, ln.Addr().String(), 776, intentLogin)
	body := protocol.AppendString(nil, "wesley")
	body = append(body, make([]byte, 16)...)
	protocol.WritePacket(c, loginPktStart, body)
	if reason := readDisconnectReason(t, br); !strings.Contains(reason, "Unable to verify") {
		t.Errorf("access outage must fail closed: %q", reason)
	}
}

// A range-pinned front door (the gw-java-770 shape: 770-772) must accept any
// protocol in the range and name the whole range in a rejection.
func TestLoginProtocolRange(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{MOTD: "test", Name: "gw-test", VersionName: "1.21.5", Proto: 770, MinProto: 770, MaxProto: 772}
	go s.Serve(ln)
	t.Cleanup(func() { ln.Close() })
	addr := ln.Addr().String()

	// 771 is inside the range: with no Backend it reaches the placeholder.
	c, br := dialHandshake(t, addr, 771, intentLogin)
	body := protocol.AppendString(nil, "wesley")
	body = append(body, make([]byte, 16)...)
	protocol.WritePacket(c, loginPktStart, body)
	if reason := readDisconnectReason(t, br); !strings.Contains(reason, "under construction") {
		t.Errorf("in-range login should pass the gate: %q", reason)
	}

	// 769 is outside: the rejection names the accepted range.
	_, br = dialHandshake(t, addr, 769, intentLogin)
	if reason := readDisconnectReason(t, br); !strings.Contains(reason, "protocols 770-772") {
		t.Errorf("out-of-range rejection should name the range: %q", reason)
	}
}
