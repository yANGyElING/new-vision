package siptest

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/new-vision-lab/new-vision/internal/nodeapp/device"
)

func TestDigestAuthorizationKnownVector(t *testing.T) {
	d := device.Device{
		DeviceAccessID: "34020000001320000001",
		SIPUsername:    "34020000001320000001",
		SIPRealm:       "3402000000",
		DigestHA1:      "1c6f224fc84be6a9be7d13b1c6a29e2f",
	}
	challenge := digestChallenge{Realm: "3402000000", Nonce: "6f1d3a9e2c4b7a50", Algorithm: "MD5", QOP: "auth"}
	authorization := digestAuthorization(d, challenge, "REGISTER", "sip:3402000000@node-access:5060")
	if !strings.Contains(authorization, `username="34020000001320000001"`) {
		t.Fatalf("missing username in authorization: %s", authorization)
	}
	if !strings.Contains(authorization, `realm="3402000000"`) {
		t.Fatalf("missing realm in authorization: %s", authorization)
	}
	if !strings.Contains(authorization, `nonce="6f1d3a9e2c4b7a50"`) {
		t.Fatalf("missing nonce in authorization: %s", authorization)
	}
	if !strings.Contains(authorization, `qop=auth`) || !strings.Contains(authorization, "nc=00000001") {
		t.Fatalf("missing qop/nc in authorization: %s", authorization)
	}
	if !strings.Contains(authorization, "response=") {
		t.Fatalf("missing response in authorization: %s", authorization)
	}
}

func TestParseDigestChallenge(t *testing.T) {
	header := `Digest realm="3402000000", nonce="6f1d3a9e2c4b7a50", algorithm=MD5, qop="auth"`
	challenge, err := parseDigestChallenge(header)
	if err != nil {
		t.Fatal(err)
	}
	if challenge.Realm != "3402000000" || challenge.Nonce != "6f1d3a9e2c4b7a50" || challenge.Algorithm != "MD5" || challenge.QOP != "auth" {
		t.Fatalf("challenge = %+v", challenge)
	}
	if _, err = parseDigestChallenge(`Digest qop="auth"`); err == nil {
		t.Fatal("challenge without realm/nonce accepted")
	}
}

func TestParseSIPResponse(t *testing.T) {
	raw := "SIP/2.0 200 OK\r\nVia: SIP/2.0/UDP 127.0.0.1:0;branch=z9hG4bK1234;rport=5060\r\nTo: <sip:34020000001320000001@3402000000>;tag=abc123\r\nContent-Length: 0\r\n\r\n"
	response, err := parseSIPResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if response.Status() != "200" || response.Reason() != "OK" {
		t.Fatalf("status = %s %s", response.Status(), response.Reason())
	}
	if !strings.Contains(response.Header("To"), "tag=abc123") {
		t.Fatalf("to header = %q", response.Header("To"))
	}
}

type lookupStub struct{ d device.Device }

func (s *lookupStub) GetByAccessID(context.Context, string) (device.Device, error) {
	return s.d, nil
}

func TestSIPRegisterParses401AndRetries(t *testing.T) {
	d := device.Device{
		DeviceAccessID: "34020000001320000001",
		SIPUsername:    "34020000001320000001",
		SIPRealm:       "3402000000",
		DigestHA1:      device.DeriveHA1("34020000001320000001", "3402000000", "testpass123"),
		Enabled:        true,
	}
	server := startSIPTestServer(t)
	sim := NewSIPSimulator("127.0.0.1", server.port, 2*time.Second, &lookupStub{d: d})
	result, err := sim.Register(context.Background(), d.DeviceAccessID, 3600)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Status, "200") {
		t.Fatalf("status = %s", result.Status)
	}
}

func TestSIPRegisterDisabledDevice(t *testing.T) {
	d := device.Device{
		DeviceAccessID: "34020000001320000001",
		SIPUsername:    "34020000001320000001",
		SIPRealm:       "3402000000",
		DigestHA1:      device.DeriveHA1("34020000001320000001", "3402000000", "testpass123"),
		Enabled:        false,
	}
	sim := NewSIPSimulator("127.0.0.1", 5060, time.Second, &lookupStub{d: d})
	_, err := sim.Register(context.Background(), d.DeviceAccessID, 3600)
	if !errors.Is(err, device.ErrInvalid) {
		t.Fatalf("expected ErrInvalid, got %v", err)
	}
}

type sipTestServer struct {
	conn *net.UDPConn
	port int
}

func startSIPTestServer(t *testing.T) *sipTestServer {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	server := &sipTestServer{conn: conn, port: conn.LocalAddr().(*net.UDPAddr).Port}
	t.Cleanup(func() { _ = conn.Close() })
	go server.serve(t)
	return server
}

func (s *sipTestServer) serve(t *testing.T) {
	buf := make([]byte, 65536)
	for {
		n, addr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		request := string(buf[:n])
		var response string
		if strings.Contains(request, "Authorization: Digest") {
			response = "SIP/2.0 200 OK\r\nVia: test\r\nTo: test\r\nContent-Length: 0\r\n\r\n"
		} else {
			response = "SIP/2.0 401 Unauthorized\r\nVia: test\r\nWWW-Authenticate: Digest realm=\"3402000000\", nonce=\"6f1d3a9e2c4b7a50\", algorithm=MD5, qop=\"auth\"\r\nTo: <sip:34020000001320000001@3402000000>;tag=server-tag\r\nContent-Length: 0\r\n\r\n"
		}
		_, _ = s.conn.WriteToUDP([]byte(response), addr)
	}
}