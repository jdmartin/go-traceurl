package main

import (
	"net/url"
	"reflect"
	"testing"
)

func enc(s string) string { return url.QueryEscape(s) }

func TestUnwrapRedirect(t *testing.T) {
	dest := "https://example.org/?id=42&utm_source=newsletter"

	tests := []struct {
		name    string
		in      string
		want    string
		wantVia []string
	}{
		{"google url=", "https://www.google.com/url?sa=t&url=" + enc(dest) + "&usg=abc", dest, []string{"www.google.com"}},
		{"google q=", "https://www.google.com/url?q=" + enc(dest), dest, []string{"www.google.com"}},
		{"google country domain", "https://www.google.ca/url?q=" + enc(dest), dest, []string{"www.google.ca"}},
		{"google look-alike not matched", "https://google.evil.example/url?q=" + enc(dest), "https://google.evil.example/url?q=" + enc(dest), nil},
		{"google q is just a search", "https://www.google.com/url?q=cats", "https://www.google.com/url?q=cats", nil},
		{"facebook", "https://l.facebook.com/l.php?u=" + enc(dest) + "&h=AT0abc", dest, []string{"l.facebook.com"}},
		{"youtube", "https://www.youtube.com/redirect?event=video_description&q=" + enc(dest), dest, []string{"www.youtube.com"}},
		{"reddit", "https://out.reddit.com/t3_abc?url=" + enc(dest), dest, []string{"out.reddit.com"}},
		{"safelinks", "https://nam12.safelinks.protection.outlook.com/?url=" + enc(dest) + "&data=x", dest, []string{"nam12.safelinks.protection.outlook.com"}},
		{"duckduckgo", "https://duckduckgo.com/l/?uddg=" + enc(dest), dest, []string{"duckduckgo.com"}},
		{"nested", "https://nam12.safelinks.protection.outlook.com/?url=" + enc("https://www.google.com/url?q="+enc(dest)), dest,
			[]string{"nam12.safelinks.protection.outlook.com", "www.google.com"}},
		{"awstrack (path-embedded)",
			"https://sgdbs6pn.r.eu-west-1.awstrack.me/L0/https:%2F%2Fnerdist.com%2Farticle%2Frings-of-power-season-3-finale-release-date-early-time%2F%3Futm_campaign=nerdist%26utm_medium=email%26utm_edition=202611200100%26utm_source=newsletter/1/010201a1fd043401-ca967e99-c095-49ad-911b-d6de46ac6b52-000000/O77mN-ie5RzXaMKvoOpW-TSGYwU=473",
			"https://nerdist.com/article/rings-of-power-season-3-finale-release-date-early-time/?utm_campaign=nerdist&utm_medium=email&utm_edition=202611200100&utm_source=newsletter",
			[]string{"sgdbs6pn.r.eu-west-1.awstrack.me"}},
		{"awstrack wrong prefix", "https://x.r.eu-west-1.awstrack.me/other/" + enc(dest) + "/1/abc/sig", "https://x.r.eu-west-1.awstrack.me/other/" + enc(dest) + "/1/abc/sig", nil},
		{"awstrack non-http segment", "https://x.r.eu-west-1.awstrack.me/L0/not-a-url/1/abc/sig", "https://x.r.eu-west-1.awstrack.me/L0/not-a-url/1/abc/sig", nil},
		{"not a wrapper", "https://example.org/?url=" + enc("https://other.example/"), "https://example.org/?url=" + enc("https://other.example/"), nil},
		{"non-http destination ignored", "https://www.google.com/url?q=" + enc("javascript:alert(1)"), "https://www.google.com/url?q=" + enc("javascript:alert(1)"), nil},
		{"wrong path", "https://www.google.com/search?q=" + enc(dest), "https://www.google.com/search?q=" + enc(dest), nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, via := unwrapRedirect(tc.in)
			if got != tc.want {
				t.Errorf("url: got %q, want %q", got, tc.want)
			}
			if !reflect.DeepEqual(via, tc.wantVia) {
				t.Errorf("via: got %v, want %v", via, tc.wantVia)
			}
		})
	}
}

func TestUnwrapDepthLimit(t *testing.T) {
	// A wrapper that wraps itself forever must terminate.
	inner := "https://www.google.com/url?q=" + enc("https://example.org/")
	cur := inner
	for i := 0; i < 20; i++ {
		cur = "https://www.google.com/url?q=" + enc(cur)
	}
	_, via := unwrapRedirect(cur)
	if len(via) != maxUnwrapDepth {
		t.Errorf("expected %d layers peeled, got %d", maxUnwrapDepth, len(via))
	}
}

func TestAwstrackThenPrune(t *testing.T) {
	in := "https://sgdbs6pn.r.eu-west-1.awstrack.me/L0/https:%2F%2Fnerdist.com%2Farticle%2Frings-of-power-season-3-finale-release-date-early-time%2F%3Futm_campaign=nerdist%26utm_medium=email%26utm_edition=202611200100%26utm_source=newsletter/1/010201a1fd043401-ca967e99-c095-49ad-911b-d6de46ac6b52-000000/O77mN-ie5RzXaMKvoOpW-TSGYwU=473"
	dest, _ := unwrapRedirect(in)
	got, removed := stripTrackingParams(dest)
	want := "https://nerdist.com/article/rings-of-power-season-3-finale-release-date-early-time/"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if len(removed) != 4 {
		t.Errorf("expected 4 params removed, got %v", removed)
	}
}
