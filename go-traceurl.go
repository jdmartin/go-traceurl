package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"os/signal"
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

type Config struct {
	UseCount int `json:"UseCount"`
}

func main() {
	// Handle SIGINT signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	// Load the cached value on startup
	config, err := loadConfig()
	if err != nil {
		fmt.Println("No cached value found.")
		config = &Config{UseCount: 0}
	}

	// Listen for SIGINT
	go func() {
		<-c
		fmt.Println("\nReceived SIGINT. Caching the current value...")

		// Cache the value on SIGINT
		if err := saveConfig(config); err != nil {
			fmt.Println("Error saving the config:", err)
		}

		os.Exit(0)
	}()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	formTemplate = template.Must(template.ParseFiles("static/form.html"))
	resultTemplate = template.Must(template.ParseFiles("static/result.html"))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		homeHandler(w, r, config)
	})
	http.HandleFunc("/trace", func(w http.ResponseWriter, r *http.Request) {
		traceHandler(w, r, config)
	})
	http.HandleFunc("/static/css/", cssHandler)
	http.HandleFunc("/static/js/", jsHandler)
	http.HandleFunc("/static/data/", dataHandler)

	addr := fmt.Sprintf(":%s", port)
	fmt.Printf("Server listening on http://localhost%s\n", addr)
	http.ListenAndServe(addr, nil)
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

func homeHandler(w http.ResponseWriter, r *http.Request, config *Config) {
	nonce, err := GenerateNonce()
	if err != nil {
		fmt.Println("Failed to generate nonce:", err)
	}

	// Set security headers
	w.Header().Set("Content-Security-Policy", fmt.Sprintf("default-src 'self'; script-src 'nonce-%s'", nonce))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")

	if r.Method == "GET" {
		data := struct {
			Nonce    string
			UseCount int
		}{
			Nonce:    nonce, // Pass the nonce value to the template data
			UseCount: config.UseCount,
		}
		formTemplate.Execute(w, data)
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

func traceHandler(w http.ResponseWriter, r *http.Request, config *Config) {
	// Increment the UseCount
	config.UseCount++
	fmt.Println("Updated UseCount:", config.UseCount)

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

	redirectURL, hops, err := followRedirects(fixedSanitizedURL)
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

func saveConfig(config *Config) error {
	data, err := json.Marshal(config)
	if err != nil {
		return err
	}

	return os.WriteFile("static/data/count.json", data, 0644)
}

func loadConfig() (*Config, error) {
	data, err := os.ReadFile("static/data/count.json")
	if err != nil {
		return nil, err
	}

	var config Config
	err = json.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}
