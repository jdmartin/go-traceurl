package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/microcosm-cc/bluemonday"
)

// *** Helper Functions ***
func doTimeout(w http.ResponseWriter, r *http.Request) {
	thereWasATimeout = true
	// Set the Content-Type header to "application/json"
	w.Header().Set("Content-Type", "text/html")
	http.Redirect(w, r, "/timeout", http.StatusFound)
}

func doValidationError(w http.ResponseWriter, r *http.Request) {
	thereWasAValidationError = true
	// Set the Content-Type header to "application/json"
	http.Redirect(w, r, "/certerror", http.StatusFound)
}

func GenerateNonce() (string, error) {
	// Define the desired length of the nonce (in bytes)
	nonceLength := 16

	// Generate random bytes for the nonce
	nonceBytes := make([]byte, nonceLength)
	_, err := rand.Read(nonceBytes)
	if err != nil {
		return "", err
	}

	// Encode the random bytes as a base64 string
	nonce := base64.StdEncoding.EncodeToString(nonceBytes)

	return nonce, nil
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

func isPathValid(validPaths []string, path string) bool {
	for _, p := range validPaths {
		if p == path {
			return true
		}
	}
	return false
}

// *** Handlers ***
func followRedirects(urlStr string, w http.ResponseWriter, r *http.Request) (string, []Hop, error) {
	// CF didn't break anything yet.
	cloudflareStatus = false

	client := &http.Client{
		Transport: &http.Transport{
			ResponseHeaderTimeout: 5 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Stop following redirects after the first hop
			if len(via) >= 1 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	hops := []Hop{}
	number := 1

	var previousURL *url.URL

	for {
		req, err := http.NewRequest("GET", urlStr, nil)
		if err != nil {
			return "", nil, fmt.Errorf("error creating request: %s", err)
		}

		// Set the user agent header
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

		resp, err := client.Do(req)
		if err != nil {
			if err, ok := err.(*url.Error); ok && err.Timeout() {
				doTimeout(w, r)
				return "", nil, nil
			}

			if strings.Contains(err.Error(), "x509: certificate signed by unknown authority") {
				// Handle certificate verification error
				doValidationError(w, r)
				return "", nil, nil
			}
			return "", nil, fmt.Errorf("error accessing URL: %s", err)
		}
		defer resp.Body.Close()

		hop := Hop{
			Number:     number,
			URL:        urlStr,
			StatusCode: resp.StatusCode,
		}
		hop.StatusCodeClass = getStatusCodeClass(resp.StatusCode)
		hops = append(hops, hop)

		if resp.StatusCode >= 300 && resp.StatusCode <= 399 {
			location := resp.Header.Get("Location")
			if location == "" {
				if strings.Contains(resp.Header.Get("Server"), "cloudflare") {
					cloudflareStatus = true
				}
				return "", []Hop{}, nil // Return empty slice of Hop when redirect location is not found
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

				return location, hops, nil
			}

			redirectURL, err := handleRelativeRedirect(previousURL, location, req.URL)
			if err != nil {
				return "", nil, fmt.Errorf("error handling relative redirect: %s", err)
			}

			// Convert redirectURL to a string
			redirectURLString := redirectURL.String()

			// Check if the "returnUri" query parameter is present
			u, err := url.Parse(redirectURLString)
			if err != nil {
				return "", nil, fmt.Errorf("error parsing URL: %s", err)
			}
			queryParams := u.Query()
			if returnURI := queryParams.Get("returnUri"); returnURI != "" {
				decodedReturnURI, err := url.PathUnescape(returnURI)
				if err != nil {
					return "", nil, fmt.Errorf("error decoding returnUri: %s", err)
				}
				decodedReturnURI = strings.ReplaceAll(decodedReturnURI, "%3A", ":")
				decodedReturnURI = strings.ReplaceAll(decodedReturnURI, "%2F", "/")

				redirectURLString = u.Scheme + "://" + u.Host + u.Path + "?returnUri=" + decodedReturnURI
			}

			if redirURI := queryParams.Get("redir"); redirURI != "" {
				decodedRedirURI, err := url.PathUnescape(redirURI)
				if err != nil {
					return "", nil, fmt.Errorf("error decoding redir param: %s", err)
				}
				decodedRedirURI = strings.ReplaceAll(decodedRedirURI, "%3A", ":")
				decodedRedirURI = strings.ReplaceAll(decodedRedirURI, "%2F", "/")

				redirectURLString = u.Scheme + "://" + u.Host + u.Path + "?redir=" + decodedRedirURI
			}

			urlStr = redirectURLString
			number++

			previousURL, err = url.Parse(urlStr)
			if err != nil {
				return "", nil, fmt.Errorf("error parsing URL: %s", err)
			}
			continue
		}

		return urlStr, hops, nil
	}
}

func handleRelativeRedirect(previousURL *url.URL, location string, requestURL *url.URL) (*url.URL, error) {
	redirectURL, err := url.Parse(location)
	if err != nil {
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

func homeHandler(w http.ResponseWriter, r *http.Request, config *Config) {
	nonce, err := GenerateNonce()
	if err != nil {
		fmt.Println("Failed to generate nonce:", err)
	}

	if r.Method == "GET" {
		data := struct {
			Nonce    string
			UseCount int
			Version  string
		}{
			Nonce:    nonce, // Pass the nonce value to the template data
			UseCount: config.UseCount,
			Version:  Version,
		}
		formTemplate.Execute(w, data)
	} else {
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

func traceHandler(w http.ResponseWriter, r *http.Request, config *Config) {
	// No timeouts, yet.
	thereWasATimeout = false

	// No cert errors, yet.
	thereWasAValidationError = false

	// Increment the UseCount
	config.UseCount++
	fmt.Println("Updated UseCount:", config.UseCount)

	nonce, err := GenerateNonce()
	if err != nil {
		fmt.Println("Failed to generate nonce:", err)
	}

	var rawURL string
	if r.Method == "POST" {
		rawURL = r.FormValue("url")
	} else if r.Method == "GET" {
		token := r.URL.Query().Get("token")
		if token != "" && token == os.Getenv("GET_TOKEN") {
			rawURL = r.URL.Query().Get("url")
		}
	} else {
		http.Redirect(w, r, "/", http.StatusFound)
	}

	// Validate the URL format
	parsedURL, err := url.ParseRequestURI(rawURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" || !parsedURL.IsAbs() {
		http.Error(w, "Invalid URL format", http.StatusBadRequest)
		return
	}

	// Check if the URL parameter contains the server name
	if strings.Contains(parsedURL.Host, r.Host) {
		http.Error(w, "Redirecting to URLs within the same server is not allowed", http.StatusBadRequest)
		return
	}

	// Sanitize URL input using bluemonday
	sanitizedURL := bluemonday.UGCPolicy().Sanitize(rawURL)
	fixedSanitizedURL := strings.ReplaceAll(sanitizedURL, "&amp;", "&")

	redirectURL, hops, err := followRedirects(fixedSanitizedURL, w, r)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error following redirects: %s", err), http.StatusInternalServerError)
		return
	}

	lastIndex := len(hops) - 1

	var finalStatusCode int
	var finalMessage string

	if lastIndex >= 0 {
		finalStatusCode = hops[lastIndex].StatusCode
	} else {
		finalStatusCode = http.StatusInternalServerError
		finalMessage = "Redirect Location Not Provided By Headers"
		hops = append(hops, Hop{Number: 1, URL: rawURL, StatusCode: finalStatusCode, StatusCodeClass: getStatusCodeClass(finalStatusCode)})
	}

	data := ResultData{
		RedirectURL:      redirectURL,
		Hops:             hops,
		LastIndex:        lastIndex,
		StatusCode:       finalStatusCode,
		FinalMessage:     template.HTML(finalMessage),
		Nonce:            nonce,
		CloudflareStatus: cloudflareStatus,
	}

	if !thereWasATimeout && !thereWasAValidationError {
		resultTemplate.Execute(w, data)
	}
}
