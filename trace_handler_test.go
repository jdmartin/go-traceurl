package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// run submits the form to traceHandler. build turns the local test server's
// base URL into the URL that gets typed into the form.
func run(t *testing.T, form url.Values, build func(target string) string) (code int, body string, seen []string) {
	t.Helper()
	var mu sync.Mutex
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.URL.RequestURI())
		mu.Unlock()
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/final?ok=1", http.StatusFound)
			return
		}
		w.WriteHeader(200)
	}))
	defer target.Close()

	form.Set("url", build(target.URL))
	req := httptest.NewRequest("POST", "/trace", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	traceHandler(rec, req, httpClient)
	return rec.Code, rec.Body.String(), seen
}

func plain(target string) string {
	return target + "/start?keep=1&utm_source=nl&fbclid=zzz"
}

func TestPruneCheckbox(t *testing.T) {
	// checkbox checked: browser sends clean=0 then clean=1
	_, body, seen := run(t, url.Values{"clean": {"0", "1"}}, plain)
	if seen[0] != "/start?keep=1" {
		t.Errorf("checked: expected pruned request, got %v", seen[0])
	}
	if !strings.Contains(body, "Removed Before Tracing") || !strings.Contains(body, "utm_source, fbclid") {
		t.Errorf("checked: result page missing removed-params row")
	}

	// checkbox unchecked: only clean=0
	_, _, seen = run(t, url.Values{"clean": {"0"}}, plain)
	if seen[0] != "/start?keep=1&utm_source=nl&fbclid=zzz" {
		t.Errorf("unchecked: expected raw request, got %v", seen[0])
	}

	// field absent entirely: default is on
	_, _, seen = run(t, url.Values{}, plain)
	if seen[0] != "/start?keep=1" {
		t.Errorf("absent: expected pruned by default, got %v", seen[0])
	}
}

func TestUnwrapCheckbox(t *testing.T) {
	wrapped := func(target string) string {
		return "https://www.google.com/url?sa=t&url=" + url.QueryEscape(plain(target))
	}

	// unwrap + prune: only the destination is ever contacted, already pruned.
	code, body, seen := run(t, url.Values{"unwrap": {"1"}, "clean": {"0", "1"}}, wrapped)
	if code != 200 || len(seen) == 0 || seen[0] != "/start?keep=1" {
		t.Errorf("unwrap+prune: code=%d seen=%v", code, seen)
	}
	for _, want := range []string{"Unwrapped Redirect From", "www.google.com", "Removed Before Tracing", "Link As Submitted"} {
		if !strings.Contains(body, want) {
			t.Errorf("unwrap+prune: result page missing %q", want)
		}
	}

	// unwrap without prune: destination contacted with its params intact.
	_, _, seen = run(t, url.Values{"unwrap": {"1"}, "clean": {"0"}}, wrapped)
	if len(seen) == 0 || seen[0] != "/start?keep=1&utm_source=nl&fbclid=zzz" {
		t.Errorf("unwrap only: got %v", seen)
	}
}

func TestUnwrapCannotPointAtOurselves(t *testing.T) {
	// httptest.NewRequest gives r.Host == "example.com"; a wrapper whose
	// destination is that host must be refused, same as typing it directly.
	req := httptest.NewRequest("POST", "/trace",
		strings.NewReader(url.Values{
			"unwrap": {"1"},
			"url":    {"https://www.google.com/url?q=" + url.QueryEscape("https://example.com/trace")},
		}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	traceHandler(rec, req, httpClient)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}
