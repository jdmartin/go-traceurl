package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHopCap(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Endless chain of *unique* URLs, which loop detection can't catch.
		http.Redirect(w, r, fmt.Sprintf("/r/%d", n.Add(1)), http.StatusFound)
	}))
	defer srv.Close()

	// Own deadline, so that if the cap ever stops working this fails instead of hanging.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, hops, _, err := followRedirects(ctx, newHTTPClient(nil), srv.URL+"/r/0")
	if !errors.Is(err, errTooManyHops) {
		t.Fatalf("expected errTooManyHops, got %v", err)
	}
	if len(hops) != maxHops {
		t.Errorf("recorded %d hops, want %d", len(hops), maxHops)
	}
	if int(n.Load()) != maxHops {
		t.Errorf("server saw %d requests, want exactly %d", n.Load(), maxHops)
	}
}

func TestTotalDeadline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(400 * time.Millisecond)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, _, _, err := followRedirects(ctx, newHTTPClient(nil), srv.URL)
	if !errors.Is(err, errTimeout) {
		t.Fatalf("expected errTimeout, got %v", err)
	}
}

func TestCertificateErrorClassified(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	// The test server's certificate isn't in the system trust store.
	_, _, _, err := followRedirects(context.Background(), newHTTPClient(nil), srv.URL)
	if !errors.Is(err, errCertificate) {
		t.Fatalf("expected errCertificate, got %v", err)
	}
}

func TestLimiterLookups(t *testing.T) {
	cases := []struct {
		mode, env string
		want      string
	}{
		{"socket", "", "X-Forwarded-For,RemoteAddr"},
		{"tcp", "", "RemoteAddr"},
		{"tcp", "true", "X-Forwarded-For,RemoteAddr"},
		{"socket", "false", "RemoteAddr"},
		{"socket", "garbage", "X-Forwarded-For,RemoteAddr"},
	}
	for _, c := range cases {
		if got := strings.Join(limiterLookups(c.mode, c.env), ","); got != c.want {
			t.Errorf("limiterLookups(%q,%q) = %q, want %q", c.mode, c.env, got, c.want)
		}
	}
}

func TestNonceMatchesCSP(t *testing.T) {
	h := secureHeaders(http.HandlerFunc(homeHandler))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	m := regexp.MustCompile(`nonce-([A-Za-z0-9_-]+)`).FindStringSubmatch(rec.Header().Get("Content-Security-Policy"))
	if m == nil {
		t.Fatalf("no nonce in CSP header: %q", rec.Header().Get("Content-Security-Policy"))
	}
	if !strings.Contains(rec.Body.String(), `nonce="`+m[1]+`"`) {
		t.Errorf("page's script nonce doesn't match the CSP header nonce %q", m[1])
	}
}
