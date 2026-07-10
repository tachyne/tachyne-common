// Package proxyproto reads the PROXY protocol v1 header (haproxy text form)
// that tachyne-dispatch prepends on internal hops so gateways see real client
// addresses. Absent header = direct connection; both are fine.
package proxyproto

import (
	"bufio"
	"strings"
)

// ReadV1 consumes a leading "PROXY ..." line if present and returns the real
// source "ip:port", or "" when no header is present (direct connection).
func ReadV1(br *bufio.Reader) (string, error) {
	peek, err := br.Peek(6)
	if err != nil || string(peek) != "PROXY " {
		return "", nil // not proxied (or too short to be); leave bytes alone
	}
	line, err := br.ReadString('\n')
	if err != nil {
		return "", err
	}
	f := strings.Fields(strings.TrimSpace(line)) // PROXY TCP4 src dst sport dport
	if len(f) >= 6 {
		return f[2] + ":" + f[4], nil
	}
	return "", nil
}
