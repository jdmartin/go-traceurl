package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Limits on a single trace.
const (
	maxHops           = 20               // redirects followed before we give up
	totalTraceTimeout = 25 * time.Second // wall-clock budget for the whole chain
)

type ctxKey int

const nonceKey ctxKey = iota

// Why a trace failed. traceHandler maps these to the user-facing response;
// the underlying detail only goes to the server log.
var (
	errTimeout     = errors.New("timed out")
	errCertificate = errors.New("certificate verification failed")
	errUnreachable = errors.New("could not connect") // includes blocked addresses
	errBadRedirect = errors.New("unusable redirect")
	errTooManyHops = errors.New("too many redirects")
)

// traceError carries a failure kind plus enough context to log it usefully
// without ever showing it to the visitor.
type traceError struct {
	Kind error  // one of the err* values above
	Hop  int    // which hop failed (1-based)
	Host string // host we were talking to
	Err  error  // underlying cause (URL stripped); for logs only
}

func (e *traceError) Error() string {
	return fmt.Sprintf("hop %d (%s): %v: %v", e.Hop, e.Host, e.Kind, e.Err)
}

// Unwrap lets errors.Is match both the kind and the underlying cause
// (for example errBlockedAddress).
func (e *traceError) Unwrap() []error { return []error{e.Kind, e.Err} }

func newTraceError(kind error, hop int, urlStr string, err error) *traceError {
	// url.Error repeats the whole URL in its message; keep only the cause.
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	host := ""
	if u, perr := url.Parse(urlStr); perr == nil {
		host = u.Host
	}
	return &traceError{Kind: kind, Hop: hop, Host: host, Err: err}
}

func classifyFetchError(hop int, urlStr string, err error) *traceError {
	kind := errUnreachable
	var ne net.Error
	var certErr *tls.CertificateVerificationError
	switch {
	case errors.As(err, &certErr):
		kind = errCertificate
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()):
		kind = errTimeout
	}
	return newTraceError(kind, hop, urlStr, err)
}

// requestNonce returns the CSP nonce that secureHeaders put on this request,
// or a fresh one if the handler is called without the middleware (tests).
func requestNonce(r *http.Request) string {
	if n, ok := r.Context().Value(nonceKey).(string); ok && n != "" {
		return n
	}
	n, err := GenerateNonce()
	if err != nil {
		fmt.Println("Failed to generate nonce:", err)
	}
	return n
}

// checkTarget validates a URL we are about to trace. If it isn't acceptable
// it writes the error response itself and returns false.
func checkTarget(w http.ResponseWriter, r *http.Request, rawURL string) bool {
	u, err := url.ParseRequestURI(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || !u.IsAbs() {
		http.Error(w, "Invalid URL format", http.StatusBadRequest)
		return false
	}

	// Check if the URL parameter contains the server name
	if strings.Contains(u.Host, r.Host) {
		http.Error(w, "Redirecting to URLs within the same server is not allowed", http.StatusBadRequest)
		return false
	}

	// Literal private/loopback/etc. addresses can be refused up front with a
	// clear message. Hostnames are judged at connect time by the dialer.
	if !allowPrivateTargets && hostIsBlockedLiteral(u.Hostname()) {
		http.Error(w, "That address can't be traced.", http.StatusBadRequest)
		return false
	}
	return true
}

// traceFailure turns a followRedirects error into a response. Everything
// except the hostname stays in the log: error text from the network layer
// can contain internal IP addresses, and "blocked" must look the same as
// "refused" or "no such host" so it can't be used to probe the network.
func traceFailure(w http.ResponseWriter, r *http.Request, err error) {
	log.Printf("trace failed: %v", err)

	var te *traceError
	errors.As(err, &te)

	switch {
	case errors.Is(err, errTimeout):
		doTimeout(w, r)
	case errors.Is(err, errCertificate):
		doValidationError(w, r)
	case errors.Is(err, errUnreachable) && te != nil && te.Host != "":
		http.Error(w, fmt.Sprintf("Couldn't connect to %s.", te.Host), http.StatusBadGateway)
	default:
		http.Error(w, "Couldn't follow that link's redirects.", http.StatusBadGateway)
	}
}

// *** Helper Functions ***
func doTimeout(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/timeout", http.StatusFound)
}

func doValidationError(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/certerror", http.StatusFound)
}

func GenerateNonce() (string, error) {
	const nonceLength = 16 // Length of nonce in bytes

	// Reusable buffer to avoid unnecessary allocations
	buf := make([]byte, nonceLength)

	// Generate random bytes for the nonce
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	// Encode the random bytes as a base64 string
	nonce := make([]byte, nonceEncoding.EncodedLen(nonceLength))
	nonceEncoding.Encode(nonce, buf)

	return string(nonce), nil
}

func getStatusCodeClass(statusCode int) string {
	switch {
	case statusCode >= 200 && statusCode < 300:
		return "2xx"
	case statusCode >= 300 && statusCode < 400:
		return "3xx"
	case statusCode >= 400 && statusCode < 500:
		return "4xx"
	case statusCode >= 500 && statusCode < 600:
		return "5xx"
	default:
		return ""
	}
}

func partialDecode(url *string) {
	//This function exists to handle these two specific character replacements.
	//Largely, this is because requests can go funny when these are encoded.
	*url = strings.ReplaceAll(*url, "%2F", "/")
	*url = strings.ReplaceAll(*url, "%3A", ":")
	*url = strings.ReplaceAll(*url, "%3D", "=")
}

func sanitizeURL(url *string) {
	// Sanitize URL input using bluemonday
	// Fix ampersands in final output because, again, things can go awry.
	*url = ugcPolicy.Sanitize(*url)
	*url = strings.ReplaceAll(*url, "&amp;", "&")
}

// *** Handlers ***
func followRedirects(ctx context.Context, client *http.Client, urlStr string) (string, []Hop, bool, error) {
	var cloudflareStatus bool // CF didn't break anything yet, so defaults to false
	var previousURL *url.URL

	hops := []Hop{}
	number := 1

	// Use a set to keep track of visited URLs
	visitedURLs := make(map[string]int)
	// Ensure the initial URL is marked as visited
	visitedURLs[urlStr] = 1

	for {
		// Hard cap on chain length. Loop detection below only catches chains
		// that repeat; a server can also just keep inventing new URLs.
		if number > maxHops {
			return urlStr, hops, cloudflareStatus, errTooManyHops
		}

		// Check if the URL has been visited before
		if visitedURLs[urlStr] > 1 {
			// Redirect loop detected
			hops = append(hops, Hop{
				Number:          number,
				URL:             urlStr,
				StatusCode:      http.StatusLoopDetected,
				StatusCodeClass: getStatusCodeClass(http.StatusLoopDetected),
			})
			return urlStr, hops, cloudflareStatus, nil
		} else {
			visitedURLs[urlStr]++
		}

		req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
		if err != nil {
			return "", hops, cloudflareStatus, newTraceError(errBadRedirect, number, urlStr, err)
		}

		// Set the user agent header
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

		resp, err := client.Do(req)
		if err != nil {
			return "", hops, cloudflareStatus, classifyFetchError(number, urlStr, err)
		}

		// All we need is the status line and a couple of headers. Close right
		// away: a defer here would hold every hop's connection open until the
		// whole function returned.
		statusCode := resp.StatusCode
		location := resp.Header.Get("Location")
		serverHeader := resp.Header.Get("Server")
		resp.Body.Close()

		hop := Hop{
			Number:     number,
			URL:        urlStr,
			StatusCode: statusCode,
		}
		hop.StatusCodeClass = getStatusCodeClass(statusCode)
		hops = append(hops, hop)

		if statusCode >= 300 && statusCode <= 399 {
			if location == "" {
				if strings.Contains(serverHeader, "cloudflare") {
					cloudflareStatus = true
				}
				return "", []Hop{}, cloudflareStatus, nil // Return empty slice of Hop when redirect location is not found
			}
			if strings.HasPrefix(location, "https://outlook.office365.com") {
				// Only include the final request as the last hop
				finalHop := Hop{
					Number:     number + 2, // Increment the hop number for the final request
					URL:        location,
					StatusCode: http.StatusOK, // Set the status code to 200 for the final request
				}
				finalHop.StatusCodeClass = getStatusCodeClass(http.StatusOK)
				hops = append(hops, finalHop)

				return location, hops, cloudflareStatus, nil
			}

			redirectURL, err := handleRelativeRedirect(previousURL, location, req.URL)
			if err != nil {
				return "", hops, cloudflareStatus, newTraceError(errBadRedirect, number, urlStr, err)
			}

			// Convert redirectURL to a string
			redirectURLString := redirectURL.String()

			// Check if the "returnUri" query parameter is present
			u, err := url.Parse(redirectURLString)
			if err != nil {
				return "", hops, cloudflareStatus, newTraceError(errBadRedirect, number, urlStr, err)
			}

			queryParams := u.Query()
			if returnURI := queryParams.Get("returnUri"); returnURI != "" {
				decodedReturnURI, err := url.PathUnescape(returnURI)
				if err != nil {
					return "", hops, cloudflareStatus, newTraceError(errBadRedirect, number, urlStr, err)
				}
				partialDecode(&decodedReturnURI)
				redirectURLString = u.Scheme + "://" + u.Host + u.Path + "?returnUri=" + decodedReturnURI
			}

			if redirURI := queryParams.Get("redir"); redirURI != "" {
				decodedRedirURI, err := url.PathUnescape(redirURI)
				if err != nil {
					return "", hops, cloudflareStatus, newTraceError(errBadRedirect, number, urlStr, err)
				}
				partialDecode(&decodedRedirURI)
				redirectURLString = u.Scheme + "://" + u.Host + u.Path + "?redir=" + decodedRedirURI
			}

			urlStr = redirectURLString
			number++

			previousURL, err = url.Parse(urlStr)
			if err != nil {
				return "", hops, cloudflareStatus, newTraceError(errBadRedirect, number, urlStr, err)
			}
			continue
		}

		return urlStr, hops, cloudflareStatus, nil
	}
}

func handleRelativeRedirect(previousURL *url.URL, location string, requestURL *url.URL) (*url.URL, error) {
	redirectURL, err := url.Parse(location)
	if err != nil {
		log.Printf("Error parsing redirect URL: %v", err)
		return nil, err
	}

	if redirectURL.Scheme == "" {
		// If the scheme is missing, set it to the scheme of the previous URL or the request URL
		if previousURL != nil {
			redirectURL.Scheme = previousURL.Scheme
		} else if requestURL != nil {
			redirectURL.Scheme = requestURL.Scheme
		} else {
			return nil, errors.New("missing scheme for relative redirect")
		}
	}

	if redirectURL.Host == "" {
		// If the host is missing, set it to the host of the previous URL or the request URL
		if previousURL != nil {
			redirectURL.Host = previousURL.Host
		} else if requestURL != nil {
			redirectURL.Host = requestURL.Host
		} else {
			return nil, errors.New("missing host for relative redirect")
		}
	}

	return redirectURL, nil
}

// wantsClean reports whether tracking parameters should be pruned before the
// link is traced. Pruning is ON unless the request explicitly says otherwise.
// The form sends a hidden clean=0 followed by the checkbox's clean=1, so we
// look at the last value: checked -> "1", unchecked -> "0". Requests that don't
// mention "clean" at all (scripts, the token-based GET) get the default: on.
func wantsClean(r *http.Request) bool {
	if err := r.ParseForm(); err != nil {
		return true
	}
	vals := r.Form["clean"]
	if len(vals) == 0 {
		return true
	}
	switch strings.ToLower(vals[len(vals)-1]) {
	case "0", "false", "off", "no":
		return false
	}
	return true
}

// wantsUnwrap reports whether known redirect wrappers (google.com/url?q=...,
// l.facebook.com/l.php?u=..., etc.) should be unwrapped before tracing.
// Unlike pruning this is OFF unless explicitly requested: a plain checkbox
// that sends unwrap=1 when checked and nothing at all when it isn't.
func wantsUnwrap(r *http.Request) bool {
	if err := r.ParseForm(); err != nil {
		return false
	}
	vals := r.Form["unwrap"]
	if len(vals) == 0 {
		return false
	}
	switch strings.ToLower(vals[len(vals)-1]) {
	case "1", "true", "on", "yes":
		return true
	}
	return false
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	nonce := requestNonce(r)

	if r.Method == "GET" {
		data := struct {
			Nonce          string
			ShowSourceLink bool
			UseCount       int
			Version        string
		}{
			Nonce:          nonce, // Pass the nonce value to the template data
			ShowSourceLink: showSourceLink,
			UseCount:       int(useCount.Load()),
			Version:        Version,
		}
		formTemplate.Execute(w, data)
	} else {
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

func traceHandler(w http.ResponseWriter, r *http.Request, httpClient *http.Client) {
	// Honeypot trap
	if r.Method == "POST" && r.FormValue("email_confirm") != "" {
		log.Printf("Honeypot triggered by IP: %s", r.RemoteAddr)
		w.WriteHeader(444) // Use non-standard 444 to silently drop
		return
	}

	// Increment the useCount
	fmt.Println("Updated UseCount:", useCount.Add(1))

	// Close the HTTP connections when done
	defer httpClient.CloseIdleConnections()

	var theURL string
	switch r.Method {
	case "POST":
		theURL = r.FormValue("url")
	case "GET":
		token := r.URL.Query().Get("token")
		want := os.Getenv("GET_TOKEN")
		if token != "" && want != "" && subtle.ConstantTimeCompare([]byte(token), []byte(want)) == 1 {
			theURL = r.URL.Query().Get("url")
		}
	default:
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	// Validate the URL format, and refuse ourselves and private addresses.
	if !checkTarget(w, r, theURL) {
		return
	}

	originalURL := theURL

	// If asked, peel off known redirect wrappers first, so we trace the real
	// destination without ever contacting the wrapper. The destination gets
	// exactly the same checks as anything typed into the form.
	var unwrappedVia []string
	if wantsUnwrap(r) {
		theURL, unwrappedVia = unwrapRedirect(theURL)
		if len(unwrappedVia) > 0 && !checkTarget(w, r, theURL) {
			return
		}
	}

	// Prune obvious tracking parameters before we follow anything, unless the
	// user asked for the raw link. Keep what we removed so we can report it.
	var preRemoved []string
	if wantsClean(r) {
		theURL, preRemoved = stripTrackingParams(theURL)
	}

	//Sanitize that URL before proceeding.
	sanitizeURL(&theURL)

	// The whole chain gets one deadline, on top of the per-request timeouts.
	ctx, cancel := context.WithTimeout(r.Context(), totalTraceTimeout)
	defer cancel()

	var finalMessage string
	redirectURL, hops, cloudflareStatus, err := followRedirects(ctx, httpClient, theURL)
	if errors.Is(err, errTooManyHops) && len(hops) > 0 {
		// Show what we saw, and say why we stopped.
		finalMessage = fmt.Sprintf("Stopped after %d hops; this link may redirect forever.", maxHops)
		err = nil
	}
	if err != nil {
		traceFailure(w, r, err)
		return
	}

	lastIndex := len(hops) - 1

	var finalStatusCode int

	if lastIndex >= 0 {
		finalStatusCode = hops[lastIndex].StatusCode
	} else {
		finalStatusCode = http.StatusInternalServerError
		finalMessage = "Redirect Location Not Provided By Headers"
		hops = append(hops, Hop{Number: 1, URL: theURL, StatusCode: finalStatusCode, StatusCodeClass: getStatusCodeClass(finalStatusCode)})
	}

	data := ResultData{
		RedirectURL:      redirectURL,
		Hops:             hops,
		LastIndex:        lastIndex,
		StatusCode:       finalStatusCode,
		FinalMessage:     template.HTML(finalMessage),
		Nonce:            requestNonce(r),
		CloudflareStatus: cloudflareStatus,
		OriginalURL:      originalURL,
		PreRemoved:       strings.Join(preRemoved, ", "),
		Unwrapped:        strings.Join(unwrappedVia, ", "),
	}

	resultTemplate.Execute(w, data)
}
