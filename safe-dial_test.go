package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
)

func TestIsAllowedIP(t *testing.T) {
	blocked := []string{
		// IPv4
		"127.0.0.1", "127.1.2.3", "0.0.0.0", "10.0.0.1", "172.16.0.1", "172.31.255.255",
		"192.168.1.1", "169.254.169.254", "100.64.0.1", "100.127.255.255", "192.0.0.1",
		"192.0.2.1", "198.18.0.1", "198.51.100.1", "203.0.113.1", "224.0.0.1",
		"240.0.0.1", "255.255.255.255",
		// IPv6
		"::1", "::", "fe80::1", "fc00::1", "fd12:3456::1", "ff02::1",
		"::127.0.0.1", "64:ff9b::a00:1", "2002:c0a8:101::1", "2001:db8::1", "2001::1", "fec0::1",
		// IPv4 in IPv6 clothing
		"::ffff:127.0.0.1", "::ffff:10.0.0.1", "::ffff:169.254.169.254",
		// zone identifiers don't help
		"fe80::1%eth0",
	}
	for _, s := range blocked {
		ip, err := netip.ParseAddr(s)
		if err != nil {
			t.Fatalf("bad test address %q: %v", s, err)
		}
		if isAllowedIP(ip) {
			t.Errorf("%s should be blocked", s)
		}
	}

	allowed := []string{
		"8.8.8.8", "1.1.1.1", "93.184.215.14", "172.15.255.255", "172.32.0.1",
		"100.63.255.255", "100.128.0.1", "2606:4700:4700::1111", "2001:4860:4860::8888",
		"::ffff:8.8.8.8",
	}
	for _, s := range allowed {
		ip := netip.MustParseAddr(s)
		if !isAllowedIP(ip) {
			t.Errorf("%s should be allowed", s)
		}
	}
}

func TestHostIsBlockedLiteral(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1": true, "10.1.2.3": true, "::1": true, "169.254.169.254": true,
		"8.8.8.8": false, "example.org": false, "localhost": false, // names are the dialer's job
	}
	for host, want := range cases {
		if got := hostIsBlockedLiteral(host); got != want {
			t.Errorf("hostIsBlockedLiteral(%q) = %v, want %v", host, got, want)
		}
	}
}

// The production client must refuse to connect to loopback, and must do so
// before any request reaches the server.
func TestProductionClientRefusesLoopback(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer srv.Close()

	_, _, _, err := followRedirects(context.Background(), createHTTPClient(), srv.URL)
	if !errors.Is(err, errBlockedAddress) {
		t.Fatalf("expected errBlockedAddress, got %v", err)
	}
	if !errors.Is(err, errUnreachable) {
		t.Errorf("blocked connections should surface as errUnreachable, got %v", err)
	}
	if hits.Load() != 0 {
		t.Errorf("server received %d requests; expected none", hits.Load())
	}
}

// A public first hop that redirects to a private address must be stopped at
// the second hop. We fake "public" by letting the first server's address
// through the guard and blocking everything else with the real rule.
func TestRedirectToPrivateIsBlocked(t *testing.T) {
	var secondHits atomic.Int32
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { secondHits.Add(1) }))
	defer second.Close()

	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, second.URL+"/secret", http.StatusFound)
	}))
	defer first.Close()

	firstAddr := strings.TrimPrefix(first.URL, "http://")
	client := newHTTPClient(func(network, address string, c syscall.RawConn) error {
		if address == firstAddr {
			return nil
		}
		return blockPrivateDial(network, address, c)
	})

	_, hops, _, err := followRedirects(context.Background(), client, first.URL)
	if !errors.Is(err, errBlockedAddress) {
		t.Fatalf("expected errBlockedAddress, got %v", err)
	}
	if len(hops) != 1 || hops[0].StatusCode != http.StatusFound {
		t.Errorf("expected exactly the first hop to be recorded, got %+v", hops)
	}
	if secondHits.Load() != 0 {
		t.Errorf("private server received %d requests; expected none", secondHits.Load())
	}
}
