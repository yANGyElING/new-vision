package nodeapp

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// SIPSimulator is a test-only SIP client used by the console to exercise the
// node-access (Kamailio) control plane: REGISTER with Digest authentication,
// GB28181 KeepAlive MESSAGE, and unregister (Expires: 0). It is not a
// production media gateway.
type SIPSimulator struct {
	host    string
	port    int
	timeout time.Duration
	lookup  DeviceLookup
}

// DeviceLookup resolves a 20-digit access ID to the authoritative device
// record (whose stored MD5 HA1 is used to answer the Digest challenge).
type DeviceLookup interface {
	GetByAccessID(context.Context, string) (Device, error)
}

type SIPTestResult struct {
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

func NewSIPSimulator(host string, port int, timeout time.Duration, lookup DeviceLookup) *SIPSimulator {
	return &SIPSimulator{host: host, port: port, timeout: timeout, lookup: lookup}
}

// Register performs a full SIP REGISTER exchange: an unauthenticated
// REGISTER, then, on a 401 challenge, a second REGISTER carrying the Digest
// Authorization header computed from the stored HA1. Both requests belong to
// the same SIP transaction (shared Call-ID, From tag and branch), as required
// by RFC 3261 for a challenged retry.
func (s *SIPSimulator) Register(ctx context.Context, accessID string, expires int) (SIPTestResult, error) {
	device, err := s.lookup.GetByAccessID(ctx, accessID)
	if err != nil {
		return SIPTestResult{}, err
	}
	if !device.Enabled {
		return SIPTestResult{}, fmt.Errorf("%w: device is disabled", ErrInvalid)
	}
	transaction := newSIPTransaction()
	first, err := s.exchange(func(source string) string {
		return s.registerPacket(device, expires, transaction, "", "", source)
	})
	if err != nil {
		return SIPTestResult{}, err
	}
	if first.Status() == "200" {
		return SIPTestResult{Status: first.Status() + " " + first.Reason()}, nil
	}
	if first.Status() != "401" {
		return SIPTestResult{}, fmt.Errorf("register denied: %s %s", first.Status(), first.Reason())
	}
	challenge, err := parseDigestChallenge(first.Header("WWW-Authenticate"))
	if err != nil {
		return SIPTestResult{}, fmt.Errorf("parse digest challenge: %w", err)
	}
	uri := s.uri(device)
	authorization := digestAuthorization(device, challenge, "REGISTER", uri)
	transaction.cseq++
	second, err := s.exchange(func(source string) string {
		return s.registerPacket(device, expires, transaction, first.Header("To"), authorization, source)
	})
	if err != nil {
		return SIPTestResult{}, err
	}
	return SIPTestResult{Status: second.Status() + " " + second.Reason()}, nil
}

// KeepAlive sends a GB28181 KeepAlive SIP MESSAGE for an online device.
func (s *SIPSimulator) KeepAlive(ctx context.Context, accessID string) (SIPTestResult, error) {
	device, err := s.lookup.GetByAccessID(ctx, accessID)
	if err != nil {
		return SIPTestResult{}, err
	}
	if !device.Enabled {
		return SIPTestResult{}, fmt.Errorf("%w: device is disabled", ErrInvalid)
	}
	response, err := s.exchange(func(source string) string {
		return s.keepAlivePacket(device, source)
	})
	if err != nil {
		return SIPTestResult{}, err
	}
	return SIPTestResult{Status: response.Status() + " " + response.Reason()}, nil
}

// Unregister sends REGISTER with Expires: 0 to force the Access layer to drop
// the runtime registration.
func (s *SIPSimulator) Unregister(ctx context.Context, accessID string) (SIPTestResult, error) {
	device, err := s.lookup.GetByAccessID(ctx, accessID)
	if err != nil {
		return SIPTestResult{}, err
	}
	transaction := newSIPTransaction()
	response, err := s.exchange(func(source string) string {
		return s.registerPacket(device, 0, transaction, "", "", source)
	})
	if err != nil {
		return SIPTestResult{}, err
	}
	return SIPTestResult{Status: response.Status() + " " + response.Reason()}, nil
}

// uri returns the REGISTER request-URI: sip:<realm>@<access host>:<port>.
// The host is resolved to an IP so Kamailio's check_uri (URI must belong to
// the server) matches: the container IP is one of its local addresses.
func (s *SIPSimulator) uri(device Device) string {
	host := s.host
	if ip, err := resolveHost(s.host); err == nil && ip != "" {
		host = ip
	}
	return fmt.Sprintf("sip:%s@%s:%d", device.SIPRealm, host, s.port)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

type sipResponse struct {
	statusLine string
	headers    map[string]string
	body       string
}

func (r *sipResponse) Status() string {
	parts := strings.SplitN(r.statusLine, " ", 3)
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

func (r *sipResponse) Reason() string {
	parts := strings.SplitN(r.statusLine, " ", 3)
	if len(parts) >= 3 {
		return parts[2]
	}
	return ""
}

func (r *sipResponse) Header(name string) string {
	return r.headers[strings.ToLower(name)]
}

func (s *SIPSimulator) exchange(build func(source string) string) (sipResponse, error) {
	raddr, err := net.ResolveUDPAddr("udp4", net.JoinHostPort(s.host, strconv.Itoa(s.port)))
	if err != nil {
		return sipResponse{}, fmt.Errorf("resolve access sip endpoint: %w", err)
	}
	sourceIP, err := probeSourceIP(raddr)
	if err != nil {
		return sipResponse{}, fmt.Errorf("probe source ip: %w", err)
	}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: sourceIP})
	if err != nil {
		return sipResponse{}, fmt.Errorf("listen udp: %w", err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(s.timeout))
	source := conn.LocalAddr().String()
	packet := []byte(build(source))
	if _, err = conn.WriteToUDP(packet, raddr); err != nil {
		return sipResponse{}, fmt.Errorf("send sip packet: %w", err)
	}
	buf := make([]byte, 65536)
	n, _, err := conn.ReadFromUDP(buf)
	if err != nil {
		return sipResponse{}, fmt.Errorf("read sip response (sent to %s from %s): %w; packet=%q", raddr, source, err, truncate(string(packet), 400))
	}
	return parseSIPResponse(string(buf[:n]))
}

// probeSourceIP dials the target without sending data to learn the local IP
// the kernel would use to reach it. Binding the SIP socket to that concrete
// address makes Via/Contact carry a routable sent-by instead of 0.0.0.0,
// which the access layer rejects (crash) on the authenticated retry.
func probeSourceIP(raddr *net.UDPAddr) (net.IP, error) {
	probe, err := net.DialUDP("udp4", nil, raddr)
	if err != nil {
		return nil, err
	}
	defer probe.Close()
	return probe.LocalAddr().(*net.UDPAddr).IP, nil
}

func resolveHost(host string) (string, error) {
	ip := net.ParseIP(host)
	if ip != nil {
		return ip.String(), nil
	}
	addresses, err := net.LookupHost(host)
	if err != nil || len(addresses) == 0 {
		return "", errors.New("resolve sip host")
	}
	return addresses[0], nil
}

func parseSIPResponse(raw string) (sipResponse, error) {
	headerEnd := strings.Index(raw, "\r\n\r\n")
	if headerEnd < 0 {
		return sipResponse{}, errors.New("malformed sip response: missing header terminator")
	}
	lines := strings.Split(raw[:headerEnd], "\r\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "SIP/2.0 ") {
		return sipResponse{}, errors.New("malformed sip response: bad status line")
	}
	headers := make(map[string]string)
	for _, line := range lines[1:] {
		colon := strings.IndexByte(line, ':')
		if colon <= 0 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(line[:colon]))
		value := strings.TrimSpace(line[colon+1:])
		if existing, ok := headers[name]; ok {
			headers[name] = existing + ", " + value
		} else {
			headers[name] = value
		}
	}
	return sipResponse{statusLine: lines[0], headers: headers, body: raw[headerEnd+4:]}, nil
}

type digestChallenge struct {
	Realm     string
	Nonce     string
	Algorithm string
	QOP       string
}

func parseDigestChallenge(header string) (digestChallenge, error) {
	var challenge digestChallenge
	challenge.Algorithm = "MD5"
	header = strings.TrimPrefix(strings.TrimSpace(header), "Digest ")
	// A minimal parser for the parameters that matter; unknown parameters are
	// ignored so the challenge remains forward compatible.
	for _, part := range splitChallengeParams(header) {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"`)
		switch strings.ToLower(key) {
		case "realm":
			challenge.Realm = value
		case "nonce":
			challenge.Nonce = value
		case "algorithm":
			challenge.Algorithm = strings.ToUpper(value)
		case "qop":
			challenge.QOP = value
		}
	}
	if challenge.Realm == "" || challenge.Nonce == "" {
		return digestChallenge{}, errors.New("digest challenge missing realm or nonce")
	}
	return challenge, nil
}

func splitChallengeParams(header string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(header); i++ {
		switch header[i] {
		case '"':
			for i++; i < len(header) && header[i] != '"'; i++ {
			}
		case ',':
			parts = append(parts, header[start:i])
			start = i + 1
		}
	}
	parts = append(parts, header[start:])
	return parts
}

func digestAuthorization(device Device, challenge digestChallenge, method, uri string) string {
	cnonce := randomHex(8)
	ha2 := md5Hex(method + ":" + uri)
	response := md5Hex(strings.Join([]string{
		device.DigestHA1, challenge.Nonce, "00000001", cnonce, "auth", ha2,
	}, ":"))
	return fmt.Sprintf(
		`Digest username="%s", realm="%s", nonce="%s", uri="%s", response="%s", algorithm=%s, cnonce="%s", qop=auth, nc=00000001`,
		device.SIPUsername, challenge.Realm, challenge.Nonce, uri, response, challenge.Algorithm, cnonce)
}

func md5Hex(input string) string {
	sum := md5.Sum([]byte(input))
	return hex.EncodeToString(sum[:])
}

func randomHex(bytes int) string {
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(buffer)
}

type sipTransaction struct {
	callID string
	branch string
	tag    string
	cseq   int
}

func newSIPTransaction() sipTransaction {
	return sipTransaction{
		callID: randomHex(8) + "@" + randomHex(4),
		branch: "z9hG4bK" + randomHex(12),
		tag:    randomHex(8),
		cseq:   1,
	}
}

func (s *SIPSimulator) registerPacket(device Device, expires int, transaction sipTransaction, toHeader, authorization, source string) string {
	uri := s.uri(device)
	from := fmt.Sprintf(`<sip:%s@%s>;tag=%s`, device.SIPUsername, device.SIPRealm, transaction.tag)
	to := fmt.Sprintf(`<sip:%s@%s>`, device.SIPUsername, device.SIPRealm)
	if toHeader != "" {
		// The challenged response carries the server-assigned To tag; reuse it.
		if tagStart := strings.Index(toHeader, ";tag="); tagStart >= 0 {
			to = toHeader
		}
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "REGISTER %s SIP/2.0\r\n", uri)
	builder.WriteString("Via: SIP/2.0/UDP " + source + ";branch=" + transaction.branch + ";rport\r\n")
	builder.WriteString("From: " + from + "\r\n")
	builder.WriteString("To: " + to + "\r\n")
	builder.WriteString("Call-ID: " + transaction.callID + "\r\n")
	fmt.Fprintf(&builder, "CSeq: %d REGISTER\r\n", transaction.cseq)
	builder.WriteString("Contact: <sip:" + device.SIPUsername + "@" + source + ">\r\n")
	builder.WriteString("Max-Forwards: 70\r\n")
	builder.WriteString("Expires: " + strconv.Itoa(expires) + "\r\n")
	if authorization != "" {
		builder.WriteString("Authorization: " + authorization + "\r\n")
	}
	builder.WriteString("Content-Length: 0\r\n\r\n")
	return builder.String()
}

func (s *SIPSimulator) keepAlivePacket(device Device, source string) string {
	callID := randomHex(8) + "@" + randomHex(4)
	branch := "z9hG4bK" + randomHex(12)
	tag := randomHex(8)
	body := fmt.Sprintf(
		"<?xml version=\"1.0\" encoding=\"UTF-8\"?>\r\n<Notify>\r\n<CmdType>Keepalive</CmdType>\r\n<DeviceID>%s</DeviceID>\r\n<Status>OK</Status>\r\n</Notify>\r\n",
		device.SIPUsername)
	var builder strings.Builder
	fmt.Fprintf(&builder, "MESSAGE sip:%s@%s SIP/2.0\r\n", device.SIPUsername, device.SIPRealm)
	builder.WriteString("Via: SIP/2.0/UDP " + source + ";branch=" + branch + ";rport\r\n")
	builder.WriteString("From: " + fmt.Sprintf(`<sip:%s@%s>;tag=%s`, device.SIPUsername, device.SIPRealm, tag) + "\r\n")
	builder.WriteString("To: " + fmt.Sprintf(`<sip:%s@%s>`, device.SIPUsername, device.SIPRealm) + "\r\n")
	builder.WriteString("Call-ID: " + callID + "\r\n")
	builder.WriteString("CSeq: 1 MESSAGE\r\n")
	builder.WriteString("Max-Forwards: 70\r\n")
	builder.WriteString("Content-Type: application/MANSCDP+xml\r\n")
	builder.WriteString("Content-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n")
	builder.WriteString(body)
	return builder.String()
}
