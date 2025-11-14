package main

import (
	"crypto/rand"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
)

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
func followRedirects(client *http.Client, urlStr string, w http.ResponseWriter, r *http.Request) (string, []Hop, bool, error) {
	var cloudflareStatus bool // CF didn't break anything yet, so defaults to false
	var previousURL *url.URL

	hops := []Hop{}
	number := 1

	// Use a set to keep track of visited URLs
	visitedURLs := make(map[string]int)
	// Ensure the initial URL is marked as visited
	visitedURLs[urlStr] = 1

	for {
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

		req, err := http.NewRequest("GET", urlStr, nil)
		if err != nil {
			return "", nil, cloudflareStatus, fmt.Errorf("error creating request: %s", err)
		}

		// Set the user agent header
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

		resp, err := client.Do(req)
		if err != nil {
			if err, ok := err.(*url.Error); ok && err.Timeout() {
				doTimeout(w, r)
				return "", nil, cloudflareStatus, nil
			}

			if strings.Contains(err.Error(), "x509: certificate signed by unknown authority") {
				// Handle certificate verification error
				doValidationError(w, r)
				return "", nil, cloudflareStatus, nil
			}

			// Close response body in case of error
			if resp != nil && resp.Body != nil {
				resp.Body.Close()
			}

			return "", nil, cloudflareStatus, fmt.Errorf("error accessing URL: %s", err)
		}

		if resp != nil && resp.Body != nil {
			defer resp.Body.Close()
		}

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
				return "", nil, cloudflareStatus, fmt.Errorf("error handling relative redirect: %s", err)
			}

			// Convert redirectURL to a string
			redirectURLString := redirectURL.String()

			// Check if the "returnUri" query parameter is present
			u, err := url.Parse(redirectURLString)
			if err != nil {
				return "", nil, cloudflareStatus, fmt.Errorf("error parsing URL: %s", err)
			}

			queryParams := u.Query()
			if returnURI := queryParams.Get("returnUri"); returnURI != "" {
				decodedReturnURI, err := url.PathUnescape(returnURI)
				if err != nil {
					return "", nil, cloudflareStatus, fmt.Errorf("error decoding returnUri: %s", err)
				}
				partialDecode(&decodedReturnURI)
				redirectURLString = u.Scheme + "://" + u.Host + u.Path + "?returnUri=" + decodedReturnURI
			}

			if redirURI := queryParams.Get("redir"); redirURI != "" {
				decodedRedirURI, err := url.PathUnescape(redirURI)
				if err != nil {
					return "", nil, cloudflareStatus, fmt.Errorf("error decoding redir param: %s", err)
				}
				partialDecode(&decodedRedirURI)
				redirectURLString = u.Scheme + "://" + u.Host + u.Path + "?redir=" + decodedRedirURI
			}

			urlStr = redirectURLString
			number++

			previousURL, err = url.Parse(urlStr)
			if err != nil {
				return "", nil, cloudflareStatus, fmt.Errorf("error parsing URL: %s", err)
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

func homeHandler(w http.ResponseWriter, r *http.Request) {
	nonce, err := GenerateNonce()
	if err != nil {
		fmt.Println("Failed to generate nonce:", err)
	}

	if r.Method == "GET" {
		data := struct {
			Nonce          string
			ShowSourceLink bool
			UseCount       int
			Version        string
		}{
			Nonce:          nonce, // Pass the nonce value to the template data
			ShowSourceLink: showSourceLink,
			UseCount:       useCount,
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
	useCount++
	fmt.Println("Updated UseCount:", useCount)

	// Close the HTTP connections when done
	defer httpClient.CloseIdleConnections()

	nonce, err := GenerateNonce()
	if err != nil {
		fmt.Println("Failed to generate nonce:", err)
	}

	var theURL string
	switch r.Method {
	case "POST":
		theURL = r.FormValue("url")
	case "GET":
		token := r.URL.Query().Get("token")
		if token != "" && token == os.Getenv("GET_TOKEN") {
			theURL = r.URL.Query().Get("url")
		}
	default:
		http.Redirect(w, r, "/", http.StatusFound)
	}

	// Validate the URL format
	parsedURL, err := url.ParseRequestURI(theURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" || !parsedURL.IsAbs() {
		http.Error(w, "Invalid URL format", http.StatusBadRequest)
		return
	}

	// Check if the URL parameter contains the server name
	if strings.Contains(parsedURL.Host, r.Host) {
		http.Error(w, "Redirecting to URLs within the same server is not allowed", http.StatusBadRequest)
		return
	}

	//Sanitize that URL before proceeding.
	sanitizeURL(&theURL)
	redirectURL, hops, cloudflareStatus, err := followRedirects(httpClient, theURL, w, r)
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
		hops = append(hops, Hop{Number: 1, URL: theURL, StatusCode: finalStatusCode, StatusCodeClass: getStatusCodeClass(finalStatusCode)})
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

	resultTemplate.Execute(w, data)
}
