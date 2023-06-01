package main

import (
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

var (
	formTemplate   *template.Template
	resultTemplate *template.Template
)

type Hop struct {
	Number          int
	URL             string
	StatusCode      int
	StatusCodeClass string
}

type ResultData struct {
	RedirectURL  string
	Hops         []Hop
	LastIndex    int
	StatusCode   int
	FinalMessage template.HTML
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	formTemplate = template.Must(template.ParseFiles("static/form.html"))
	resultTemplate = template.Must(template.ParseFiles("static/result.html"))

	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/trace", traceHandler)
	http.HandleFunc("/static/css/", cssHandler)
	http.HandleFunc("/static/js/", jsHandler)
	http.HandleFunc("/static/data/", dataHandler)

	addr := fmt.Sprintf(":%s", port)
	fmt.Printf("Server listening on http://localhost%s\n", addr)
	http.ListenAndServe(addr, nil)
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		formTemplate.Execute(w, nil)
	} else {
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

func cssHandler(w http.ResponseWriter, r *http.Request) {
	filePath := "static/css/" + strings.TrimPrefix(r.URL.Path, "/static/css/")
	http.ServeFile(w, r, filePath)

	// Set the Content-Type header to "text/css"
	w.Header().Set("Content-Type", "text/css")
}

func jsHandler(w http.ResponseWriter, r *http.Request) {
	filePath := "static/js/" + strings.TrimPrefix(r.URL.Path, "/static/js/")
	http.ServeFile(w, r, filePath)

	// Set the Content-Type header to "application/javascript"
	w.Header().Set("Content-Type", "application/javascript")
}

func dataHandler(w http.ResponseWriter, r *http.Request) {
	filePath := "static/data/" + strings.TrimPrefix(r.URL.Path, "/static/data/")
	http.ServeFile(w, r, filePath)

	// Set the Content-Type header to "application/json"
	w.Header().Set("Content-Type", "application/json")
}

func traceHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		rawURL := r.FormValue("url")

		// Validate the URL format
		parsedURL, err := url.ParseRequestURI(rawURL)
		if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" || !parsedURL.IsAbs() {
			http.Error(w, "Invalid URL format", http.StatusBadRequest)
			return
		}

		// Sanitize URL input using bluemonday
		sanitizedURL := bluemonday.UGCPolicy().Sanitize(rawURL)

		redirectURL, hops, err := followRedirects(sanitizedURL)
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
			RedirectURL:  redirectURL,
			Hops:         hops,
			LastIndex:    lastIndex,
			StatusCode:   finalStatusCode,
			FinalMessage: template.HTML(finalMessage),
		}

		resultTemplate.Execute(w, data)
	} else {
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

func followRedirects(urlStr string) (string, []Hop, error) {
	client := &http.Client{
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
			// Handle redirect
			location := resp.Header.Get("Location")
			if location == "" {
				return "", []Hop{}, nil // Return empty slice of Hop when redirect location is not found
			}

			redirectURL, err := handleRelativeRedirect(previousURL, location)
			if err != nil {
				return "", nil, fmt.Errorf("error handling relative redirect: %s", err)
			}

			// Check if the "returnUri" query parameter is present
			u, err := url.Parse(redirectURL)
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

				redirectURL = u.Scheme + "://" + u.Host + u.Path + "?returnUri=" + decodedReturnURI
			}

			urlStr = redirectURL
			number++

			// Update previousURL with the current URL
			previousURL, err = url.Parse(urlStr)
			if err != nil {
				return "", nil, fmt.Errorf("error parsing URL: %s", err)
			}
			continue
		}

		return urlStr, hops, nil
	}
}

func handleRelativeRedirect(previousURL *url.URL, location string) (string, error) {
	redirectURL, err := url.Parse(location)
	if err != nil {
		return "", err
	}

	// Check if the redirect URL is an absolute URL
	if !redirectURL.IsAbs() {
		// Use the domain from the previous URL
		if previousURL != nil {
			redirectURL.Scheme = previousURL.Scheme
			redirectURL.Host = previousURL.Host
		} else {
			// Use the current host
			currentURL, err := url.Parse(location)
			if err == nil {
				redirectURL.Scheme = currentURL.Scheme
				redirectURL.Host = currentURL.Host
			} else {
				return "", err
			}
		}
	}
	absoluteURL := redirectURL.String()
	return absoluteURL, nil
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
