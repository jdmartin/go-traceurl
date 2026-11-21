package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

func run(t *testing.T, form url.Values) (body string, seen []string) {
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

	form.Set("url", target.URL+"/start?keep=1&utm_source=nl&fbclid=zzz")
	req := httptest.NewRequest("POST", "/trace", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	traceHandler(rec, req, httpClient)
	return rec.Body.String(), seen
}

func TestE2E(t *testing.T) {
	// checkbox checked: browser sends clean=0 then clean=1
	body, seen := run(t, url.Values{"clean": {"0", "1"}})
	t.Logf("checked   -> first request: %v", seen)
	if seen[0] != "/start?keep=1" {
		t.Errorf("expected pruned request, got %v", seen[0])
	}
	if !strings.Contains(body, "Removed Before Tracing") || !strings.Contains(body, "utm_source, fbclid") {
		t.Errorf("result page missing removed-params row")
	}

	// checkbox unchecked: only clean=0
	_, seen = run(t, url.Values{"clean": {"0"}})
	t.Logf("unchecked -> first request: %v", seen)
	if seen[0] != "/start?keep=1&utm_source=nl&fbclid=zzz" {
		t.Errorf("expected raw request, got %v", seen[0])
	}

	// field absent entirely: default is on
	_, seen = run(t, url.Values{})
	t.Logf("absent    -> first request: %v", seen)
	if seen[0] != "/start?keep=1" {
		t.Errorf("expected pruned by default, got %v", seen[0])
	}
}
