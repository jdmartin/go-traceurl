package main

import (
	"net/url"
	"strings"
)

// Pre-resolution pruning
//
// These lists are deliberately conservative. We strip parameters *before*
// following the link, so a wrong guess could break the redirect itself.
// (Stripping at the very end, as main.js does, can only cost a cosmetic
// parameter.) Generic names like "cid", "ser", or "mailId" are left out
// here for that reason; main.js still removes them from the final URL.
//
// All comparisons are done on the lowercased parameter name.

// trackingParams are exact (lowercase) parameter names.
var trackingParams = map[string]bool{
	"_hsenc":           true,
	"_hsmi":            true,
	"_kx":              true,
	"bbeml":            true,
	"ck_subscriber_id": true,
	"dclid":            true,
	"dm_i":             true,
	"dm_t":             true,
	"ea.tracking.id":   true,
	"emlcid":           true,
	"emldtl":           true,
	"fbclid":           true,
	"gbraid":           true,
	"gclid":            true,
	"hsctatracking":    true,
	"igshid":           true,
	"li_fat_id":        true,
	"mc_cid":           true,
	"mc_eid":           true,
	"mgparam":          true,
	"mkt_tok":          true,
	"msclkid":          true,
	"oly_anon_id":      true,
	"oly_enc_id":       true,
	"rfrr":             true,
	"ttclid":           true,
	"twclid":           true,
	"vero_conv":        true,
	"vero_id":          true,
	"wbraid":           true,
	"yclid":            true,
}

// trackingPrefixes match any parameter that starts with one of these.
var trackingPrefixes = []string{
	"__hs", // HubSpot
	"cm_",  // IBM/Coremetrics
	"mtm_", // Matomo
	"pk_",  // Piwik/Matomo
	"utm_", // Google Analytics
}

func isTrackingParam(name string) bool {
	name = strings.ToLower(name)
	if trackingParams[name] {
		return true
	}
	for _, prefix := range trackingPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// stripTrackingParams removes obvious tracking parameters from rawURL's query
// string. It returns the cleaned URL and the names of the parameters it
// removed, in the order they appeared.
//
// Everything that isn't removed is left byte-for-byte as it was: order,
// percent-encoding, repeated keys, and the fragment all survive. (Parsing into
// url.Values and calling Encode() would re-sort and re-encode the query, which
// can break links that are picky about their format.)
//
// If rawURL can't be parsed, it is returned unchanged.
func stripTrackingParams(rawURL string) (string, []string) {
	u, err := url.Parse(rawURL)
	if err != nil || u.RawQuery == "" {
		return rawURL, nil
	}

	var kept, removed []string
	for _, pair := range strings.Split(u.RawQuery, "&") {
		if pair == "" {
			continue
		}

		rawKey := pair
		if i := strings.IndexByte(pair, '='); i >= 0 {
			rawKey = pair[:i]
		}

		// Decode only to compare. If the key won't decode, compare it as-is.
		key, err := url.QueryUnescape(rawKey)
		if err != nil {
			key = rawKey
		}

		if isTrackingParam(key) {
			removed = append(removed, key)
		} else {
			kept = append(kept, pair)
		}
	}

	if len(removed) == 0 {
		return rawURL, nil
	}

	u.RawQuery = strings.Join(kept, "&")
	return u.String(), removed
}
