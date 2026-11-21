package main

import (
	"reflect"
	"testing"
)

func TestStripTrackingParams(t *testing.T) {
	tests := []struct {
		name        string
		in          string
		wantURL     string
		wantRemoved []string
	}{
		{"no query", "https://example.com/a", "https://example.com/a", nil},
		{"nothing to strip", "https://example.com/a?id=5&page=2", "https://example.com/a?id=5&page=2", nil},
		{"utm only", "https://example.com/a?utm_source=x&utm_medium=y", "https://example.com/a", []string{"utm_source", "utm_medium"}},
		{"mixed keeps order", "https://example.com/a?b=2&fbclid=abc&a=1", "https://example.com/a?b=2&a=1", []string{"fbclid"}},
		{"keeps fragment", "https://example.com/a?gclid=1&x=2#top", "https://example.com/a?x=2#top", []string{"gclid"}},
		{"case insensitive", "https://example.com/a?UTM_Source=x&FBCLID=y&k=v", "https://example.com/a?k=v", []string{"UTM_Source", "FBCLID"}},
		{"preserves encoding", "https://example.com/a?q=a%20b%2Fc&utm_x=1", "https://example.com/a?q=a%20b%2Fc", []string{"utm_x"}},
		{"nested url param untouched", "https://example.com/r?to=https%3A%2F%2Fb.com%2F%3Futm_a%3D1&utm_b=2", "https://example.com/r?to=https%3A%2F%2Fb.com%2F%3Futm_a%3D1", []string{"utm_b"}},
		{"valueless param", "https://example.com/a?fbclid&x=1", "https://example.com/a?x=1", []string{"fbclid"}},
		{"generic names left alone", "https://example.com/a?cid=7&ser=1", "https://example.com/a?cid=7&ser=1", nil},
		{"repeated keys", "https://example.com/a?t=1&t=2&utm_a=3", "https://example.com/a?t=1&t=2", []string{"utm_a"}},
		{"unparseable", "http://[::1", "http://[::1", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotURL, gotRemoved := stripTrackingParams(tc.in)
			if gotURL != tc.wantURL {
				t.Errorf("url: got %q, want %q", gotURL, tc.wantURL)
			}
			if !reflect.DeepEqual(gotRemoved, tc.wantRemoved) {
				t.Errorf("removed: got %v, want %v", gotRemoved, tc.wantRemoved)
			}
		})
	}
}
