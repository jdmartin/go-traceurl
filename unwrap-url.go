package main

import (
	"net/url"
	"regexp"
	"strings"
)

// Redirect-wrapper unwrapping
//
// Some links are just a wrapper around the real destination: the real URL
// rides along in a query parameter, and the wrapper's server bounces you
// there after (maybe) logging the click. When we recognise one, we can pull
// the destination out directly and never contact the wrapper at all.
//
// A rule only fires when BOTH the host/path match a known wrapper AND the
// named parameter holds an absolute http(s) URL. A google.com/url?q=cats
// search-ish value is therefore left alone.

type unwrapRule struct {
	match  func(host, path string) bool
	params []string // candidate query parameter names, tried in order

	// pathSegment is for wrappers that embed the destination in the URL path
	// instead of the query string. It is an index into the *escaped* path
	// split on "/" (element 0 is the empty string before the leading slash),
	// so the destination's own slashes, which arrive as %2F, don't split it.
	// 0 means unused.
	pathSegment int
}

// maxUnwrapDepth stops pathological or circular chains.
const maxUnwrapDepth = 5

var googleHost = regexp.MustCompile(`^(www\.)?google\.(com|co\.[a-z]{2}|com\.[a-z]{2}|[a-z]{2})$`)

// onHosts matches an exact host (lowercase, no port) and a path prefix.
func onHosts(pathPrefix string, hosts ...string) func(host, path string) bool {
	return func(host, path string) bool {
		if !strings.HasPrefix(path, pathPrefix) {
			return false
		}
		for _, h := range hosts {
			if host == h {
				return true
			}
		}
		return false
	}
}

var unwrapRules = []unwrapRule{
	// Google: google.com/url?q=... or ?url=... (any country domain)
	{
		match: func(host, path string) bool {
			return googleHost.MatchString(host) && strings.HasPrefix(path, "/url")
		},
		params: []string{"url", "q"},
	},
	// Facebook / Messenger outbound links
	{
		match:  onHosts("/l.php", "l.facebook.com", "lm.facebook.com", "l.messenger.com", "www.facebook.com", "facebook.com"),
		params: []string{"u"},
	},
	// YouTube description/comment links
	{
		match:  onHosts("/redirect", "www.youtube.com", "youtube.com", "m.youtube.com"),
		params: []string{"q"},
	},
	// Reddit outbound links
	{
		match:  onHosts("/", "out.reddit.com"),
		params: []string{"url"},
	},
	// Steam community link filter
	{
		match:  onHosts("/linkfilter", "steamcommunity.com"),
		params: []string{"u", "url"},
	},
	// Slack outbound links
	{
		match:  onHosts("/link", "slack-redir.net"),
		params: []string{"url"},
	},
	// Microsoft Defender / Outlook "Safe Links" (many regional hostnames)
	{
		match: func(host, path string) bool {
			return strings.HasSuffix(host, ".safelinks.protection.outlook.com")
		},
		params: []string{"url"},
	},
	// DuckDuckGo result links
	{
		match:  onHosts("/l/", "duckduckgo.com", "www.duckduckgo.com"),
		params: []string{"uddg"},
	},
	// VK
	{
		match:  onHosts("/away.php", "vk.com"),
		params: []string{"to"},
	},
	// Tumblr
	{
		match:  onHosts("/redirect", "t.umblr.com"),
		params: []string{"z"},
	},
	// Amazon SES click tracking:
	//   https://<id>.r.<region>.awstrack.me/L0/<encoded destination>/1/<message id>/<signature>
	{
		match: func(host, path string) bool {
			return strings.HasSuffix(host, ".awstrack.me") && strings.HasPrefix(path, "/L0/")
		},
		pathSegment: 2,
	},
}

// isHTTPURL reports whether s is an absolute http(s) URL with a host.
func isHTTPURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	return scheme == "http" || scheme == "https"
}

// unwrapRedirect peels known redirect wrappers off rawURL, up to
// maxUnwrapDepth layers deep. It returns the innermost destination and the
// hostnames of the wrappers it removed, outermost first. If rawURL isn't a
// recognised wrapper, it is returned unchanged with a nil slice.
func unwrapRedirect(rawURL string) (string, []string) {
	current := rawURL
	var via []string

	for i := 0; i < maxUnwrapDepth; i++ {
		u, err := url.Parse(current)
		if err != nil || u.Host == "" {
			break
		}
		host := strings.ToLower(u.Hostname())
		query := u.Query() // already percent-decoded

		dest := ""
		for _, rule := range unwrapRules {
			if !rule.match(host, u.Path) {
				continue
			}
			for _, p := range rule.params {
				if candidate := query.Get(p); isHTTPURL(candidate) {
					dest = candidate
					break
				}
			}
			if dest == "" && rule.pathSegment > 0 {
				segs := strings.Split(u.EscapedPath(), "/")
				if rule.pathSegment < len(segs) {
					if candidate, err := url.PathUnescape(segs[rule.pathSegment]); err == nil && isHTTPURL(candidate) {
						dest = candidate
					}
				}
			}
			if dest != "" {
				break
			}
		}
		if dest == "" {
			break
		}

		via = append(via, host)
		current = dest
	}

	return current, via
}
