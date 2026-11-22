package main

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"syscall"
)

// SSRF protection
//
// The tracer fetches whatever URL a visitor hands it, and follows whatever
// redirects come back. Without a guard, that makes it a way to reach things
// only the *server* can reach: localhost services, the LAN, cloud metadata
// endpoints, and so on.
//
// The guard lives in the dialer's Control hook (see newHTTPClient). Control
// runs after DNS resolution, on the exact IP address about to be connected
// to, for every connection, including each redirect hop. Checking there,
// rather than checking the URL's hostname up front, is what makes it
// robust against:
//   - hostnames that resolve to private addresses,
//   - redirects from a public server to a private one,
//   - DNS rebinding (the name resolves differently between check and use),
//   - alternate spellings of the same address.

// errBlockedAddress is returned (wrapped) when a connection is refused
// because the destination isn't a public address.
var errBlockedAddress = errors.New("destination address is not allowed")

// allowPrivateTargets turns off the early literal-IP check in checkTarget.
// It exists only so tests can talk to httptest servers on 127.0.0.1; the
// dialer guard itself is controlled by which client you build.
var allowPrivateTargets = false

// blockedPrefixes are ranges that netip's IsPrivate/IsLoopback/etc. don't
// already cover. (Those are checked first in isAllowedIP.)
var blockedPrefixes = mustPrefixes(
	// IPv4
	"0.0.0.0/8",       // "this network"
	"100.64.0.0/10",   // carrier-grade NAT (also used by Tailscale)
	"192.0.0.0/24",    // IETF protocol assignments
	"192.0.2.0/24",    // TEST-NET-1
	"198.18.0.0/15",   // benchmarking
	"198.51.100.0/24", // TEST-NET-2
	"203.0.113.0/24",  // TEST-NET-3
	"240.0.0.0/4",     // reserved, including 255.255.255.255

	// IPv6
	"::/96",          // deprecated IPv4-compatible addresses (::127.0.0.1)
	"64:ff9b::/96",   // NAT64: can embed a private IPv4 address.
	"64:ff9b:1::/48", // local-use NAT64. (If you run the tracer on an IPv6-only network that relies on NAT64, remove these two.)
	"100::/64",       // discard-only
	"2001::/32",      // Teredo
	"2001:db8::/32",  // documentation
	"2002::/16",      // 6to4: can embed a private IPv4 address
	"fec0::/10",      // deprecated site-local
)

func mustPrefixes(cidrs ...string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(cidrs))
	for _, c := range cidrs {
		out = append(out, netip.MustParsePrefix(c))
	}
	return out
}

// isAllowedIP reports whether ip is a public, globally routable address
// that the tracer may connect to.
func isAllowedIP(ip netip.Addr) bool {
	// ::ffff:10.0.0.1 is just 10.0.0.1 in disguise.
	ip = ip.Unmap().WithZone("")
	if !ip.IsValid() {
		return false
	}
	if ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsUnspecified() ||
		ip.IsMulticast() ||
		ip.IsLinkLocalUnicast() || // includes 169.254.169.254 (cloud metadata)
		ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() {
		return false
	}
	for _, p := range blockedPrefixes {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

// hostIsBlockedLiteral reports whether host is a literal IP address that we
// would refuse to connect to. Hostnames return false: they can only be
// judged after DNS resolution, which the dialer hook does.
func hostIsBlockedLiteral(host string) bool {
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	return !isAllowedIP(ip)
}

// blockPrivateDial is the net.Dialer Control hook.
func blockPrivateDial(network, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%w: unparseable address %q", errBlockedAddress, address)
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || !isAllowedIP(ip) {
		return fmt.Errorf("%w: %s", errBlockedAddress, host)
	}
	return nil
}
