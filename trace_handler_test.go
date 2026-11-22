package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// useLocalTargets lets the handler talk to httptest servers on 127.0.0.1 for
// the duration of a test: it relaxes the early literal-IP check and returns a
// client with no dialer guard.
func useLocalTargets(t *testing.T) *http.Client {
	t.Helper()
	allowPrivateTargets = true
	t.Cleanup(func() { allowPrivateTargets = false })
	return newHTTPClient(nil)
}

func post(client *http.Client, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/trace", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	traceHandler(rec, req, client)
	return rec
}

// run submits the form to traceHandler. build turns the local test server's
// base URL into the URL that gets typed into the form.
func run(t *testing.T, form url.Values, build func(target string) string) (code int, body string, seen []string) {
	t.Helper()
	client := useLocalTargets(t)

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
	rec := post(client, form)
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
	rec := post(createHTTPClient(), url.Values{
		"unwrap": {"1"},
		"url":    {"https://www.google.com/url?q=" + url.QueryEscape("https://example.com/trace")},
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// --- SSRF, at the handler level (production client, no test knobs) ---

func TestPrivateLiteralsRejectedUpFront(t *testing.T) {
	for _, target := range []string{
		"http://127.0.0.1/",
		"http://127.0.0.1:8080/admin",
		"http://[::1]/",
		"http://10.0.0.5/",
		"http://192.168.1.1/",
		"http://169.254.169.254/latest/meta-data/",
		"http://[::ffff:127.0.0.1]/",
		"http://100.64.0.1/",
	} {
		rec := post(createHTTPClient(), url.Values{"url": {target}})
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "can't be traced") {
			t.Errorf("%s: got %d %q", target, rec.Code, strings.TrimSpace(rec.Body.String()))
		}
	}
}

func TestWrapperCannotSmuggleInPrivateTarget(t *testing.T) {
	rec := post(createHTTPClient(), url.Values{
		"unwrap": {"1"},
		"url":    {"https://l.facebook.com/l.php?u=" + url.QueryEscape("http://192.168.1.1/admin")},
	})
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "can't be traced") {
		t.Errorf("got %d %q", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
}

// Hostnames can't be judged up front, so the dialer must stop them, and the
// visitor must get the same bland message as any other connection failure.
func TestHostnameResolvingToLoopbackIsBlockedQuietly(t *testing.T) {
	rec := post(createHTTPClient(), url.Values{"url": {"http://localhost:9/"}})
	body := rec.Body.String()
	if rec.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", rec.Code)
	}
	for _, leak := range []string{"127.0.0.1", "::1", "dial tcp", "not allowed", "refused", "blocked"} {
		if strings.Contains(body, leak) {
			t.Errorf("response leaks %q: %q", leak, body)
		}
	}
}

func TestConnectionFailureMessageIsGeneric(t *testing.T) {
	client := useLocalTargets(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	dead := srv.URL
	srv.Close() // nothing is listening any more

	rec := post(client, url.Values{"url": {dead}})
	body := rec.Body.String()
	if rec.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", rec.Code)
	}
	for _, leak := range []string{"dial tcp", "refused", "error accessing"} {
		if strings.Contains(body, leak) {
			t.Errorf("response leaks %q: %q", leak, body)
		}
	}
	if !strings.Contains(body, "Couldn't connect to") {
		t.Errorf("unexpected message: %q", body)
	}
}

func TestTooManyHopsShowsPartialTrace(t *testing.T) {
	client := useLocalTargets(t)
	var mu sync.Mutex
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		n++
		v := n
		mu.Unlock()
		http.Redirect(w, r, "/r/"+strings.Repeat("x", v), http.StatusFound)
	}))
	defer srv.Close()

	rec := post(client, url.Values{"url": {srv.URL + "/r/start"}, "clean": {"0"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected the partial trace page, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Stopped after 20 hops") {
		t.Errorf("result page missing the stop message")
	}
}

func TestGetTokenCheck(t *testing.T) {
	client := useLocalTargets(t)
	t.Setenv("GET_TOKEN", "s3cret")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	get := func(q string) int {
		req := httptest.NewRequest("GET", "/trace?"+q, nil)
		rec := httptest.NewRecorder()
		traceHandler(rec, req, client)
		return rec.Code
	}
	if code := get("token=wrong&url=" + url.QueryEscape(srv.URL)); code != http.StatusBadRequest {
		t.Errorf("wrong token: got %d, want 400", code)
	}
	if code := get("url=" + url.QueryEscape(srv.URL)); code != http.StatusBadRequest {
		t.Errorf("no token: got %d, want 400", code)
	}
	if code := get("token=s3cret&url=" + url.QueryEscape(srv.URL)); code != http.StatusOK {
		t.Errorf("right token: got %d, want 200", code)
	}
}

func TestUnsupportedMethodRedirectsOnce(t *testing.T) {
	req := httptest.NewRequest("PUT", "/trace", nil)
	rec := httptest.NewRecorder()
	traceHandler(rec, req, createHTTPClient())
	if rec.Code != http.StatusFound {
		t.Errorf("expected 302, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "Invalid URL format") {
		t.Errorf("handler kept going after redirecting")
	}
}
