package main

import (
	"encoding/base64"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/didip/tollbooth/v7"
	"github.com/didip/tollbooth/v7/limiter"
	"github.com/microcosm-cc/bluemonday"
)

var (
	formTemplate *template.Template
	host         string
	httpClient   = createHTTPClient()
	lim          = tollbooth.NewLimiter(1, &limiter.ExpirableOptions{
		DefaultExpirationTTL: 3 * time.Second,
	})
	mode           string
	nonceEncoding  = base64.RawURLEncoding
	port           string
	resultTemplate *template.Template
	serveMode      string
	showSourceLink = true
	useCount       int
	ugcPolicy      = bluemonday.UGCPolicy()
	Version        = "2025.03.27.1"
)

var allowedEndpoints = map[string]bool{
	"/":           true,
	"/certerror/": true,
	"/timeout/":   true,
	"/trace":      true,
}

type Hop struct {
	Number          int
	URL             string
	StatusCode      int
	StatusCodeClass string
}

type ResultData struct {
	CloudflareStatus bool
	RedirectURL      string
	Hops             []Hop
	LastIndex        int
	StatusCode       int
	FinalMessage     template.HTML
	Nonce            string
}

type CustomTransport struct {
	*http.Transport
}

func (ct *CustomTransport) CloseIdleConnections() {
	// Implement the logic to close idle connections
	ct.Transport.CloseIdleConnections()
}

// Utility and Helper

func createHTTPClient() *http.Client {
	transport := &CustomTransport{
		Transport: &http.Transport{
			ResponseHeaderTimeout: 5 * time.Second,
			MaxIdleConns:          2,                // Maximum idle connections across all hosts
			IdleConnTimeout:       90 * time.Second, // Keep connections idle for up to 90 seconds
			TLSHandshakeTimeout:   10 * time.Second, // Limit time spent on the TLS handshake
		},
	}
	return &http.Client{
		Timeout:   8 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Stop following redirects after the first hop
			if len(via) >= 1 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
}

func handleSIGINT(socketPath string, serveMode string) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	go func() {
		<-c
		fmt.Println("\nReceived SIGINT. Stopping server...")

		if serveMode == "socket" {
			// Close the Unix domain socket
			if l, err := net.Listen("unix", socketPath); err == nil {
				l.Close()
			}

			// Remove the Unix domain socket file
			if err := os.Remove(socketPath); err != nil {
				fmt.Printf("Error removing socket file: %v\n", err)
			}
		}

		os.Exit(0)
	}()
}

func initLimiter() {
	lim.SetIPLookups([]string{"RemoteAddr", "X-Forwarded-For", "X-Real-IP"})
}

func initTemplates() error {
	var err error

	// Parse form template
	formTemplate, err = parseTemplate("static/form.html")
	if err != nil {
		return fmt.Errorf("failed to parse form template: %v", err)
	}

	// Parse result template
	resultTemplate, err = parseTemplate("static/result.html")
	if err != nil {
		return fmt.Errorf("failed to parse result template: %v", err)
	}

	return nil
}

func parseTemplate(filename string) (*template.Template, error) {
	// Attempt to parse the template
	tmpl, err := template.ParseFiles(filename)
	return tmpl, err
}

// Middleware function to set security headers
func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonce, err := GenerateNonce()
		if err != nil {
			fmt.Println("Failed to generate nonce:", err)
		}

		w.Header().Set("Content-Security-Policy", fmt.Sprintf("default-src 'self'; script-src 'self' 'nonce-%s'", nonce))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "accelerometer=(), ambient-light-sensor=(), autoplay=(), battery=(), camera=(), cross-origin-isolated=(), display-capture=(), document-domain=(), encrypted-media=(), execution-while-not-rendered=(), execution-while-out-of-viewport=(), fullscreen=(), geolocation=(), gyroscope=(), keyboard-map=(), magnetometer=(), microphone=(), midi=(), navigation-override=(), payment=(), picture-in-picture=(), publickey-credentials-get=(), screen-wake-lock=(), sync-xhr=(), usb=(), web-share=(), xr-spatial-tracking=()")

		// USE HSTS when in production mode only, because testing.
		if os.Getenv("MODE") == "production" {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
		}

		// Call the next handler
		next.ServeHTTP(w, r)
	})
}

// Init and Main

func init() {
	// Set counter to zero on startup
	useCount = 0

	// Detect dev or production mode
	mode = os.Getenv("MODE")
	if mode == "" {
		mode = "production"
	}

	// Make sure we have a port to serve on
	port = os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Allow setting of listen address
	host = os.Getenv("HOST")
	if host == "" {
		host = "127.0.0.1"
	}

	// Set default serveMode
	serveMode = os.Getenv("SERVE")
	if serveMode != "tcp" {
		serveMode = "socket"
	}

	// Initialize templates
	if err := initTemplates(); err != nil {
		fmt.Printf("Failed to initialize templates: %v\n", err)
		os.Exit(1)
	}

	// Set rate limiter with a limit of 1 requests per second
	initLimiter()
}

func main() {
	// Set socket path for listenting
	socketPath := "/tmp/go-trace.sock"

	// Handle SIGINT signal
	handleSIGINT(socketPath, serveMode)

	// Initialize Templates
	if err := initTemplates(); err != nil {
		fmt.Printf("Failed to initialize templates: %v\n", err)
		os.Exit(1)
	}

	// Establish Routes
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Check if the requested endpoint is allowed
		if _, ok := allowedEndpoints[r.URL.Path]; !ok {
			http.NotFound(w, r)
			return
		}
		// Limit the request using the rate limiter
		httpError := tollbooth.LimitByRequest(lim, w, r)
		if httpError != nil {
			http.Error(w, httpError.Message, httpError.StatusCode)
			return
		}
		// Handle the request
		homeHandler(w, r)
	})

	http.HandleFunc("/certerror/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/certerror.html")
	})

	http.HandleFunc("/timeout/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/timeout.html")
	})

	http.HandleFunc("/trace", func(w http.ResponseWriter, r *http.Request) {
		// Limit the request using the rate limiter
		httpError := tollbooth.LimitByRequest(lim, w, r)
		if httpError != nil {
			http.Error(w, httpError.Message, httpError.StatusCode)
			return
		}
		// Handle the request
		traceHandler(w, r, httpClient)
	})

	// Serve static files using http.FileServer and http.StripPrefix
	http.Handle("/static/css/", http.StripPrefix("/static/css/", http.FileServer(http.Dir("static/css"))))
	http.Handle("/static/data/", http.StripPrefix("/static/data/", http.FileServer(http.Dir("static/data"))))
	http.Handle("/static/images/", http.StripPrefix("/static/images/", http.FileServer(http.Dir("static/images"))))
	http.Handle("/static/js/", http.StripPrefix("/static/js/", http.FileServer(http.Dir("static/js"))))

	switch {
	case serveMode == "socket":
		l, err := net.Listen("unix", socketPath)
		if err != nil {
			fmt.Printf("Failed to listen on Unix socket: %v\n", err)
			os.Exit(1)
		}
		defer l.Close()

		// Set the permissions to 775 for the Unix domain socket
		if err := os.Chmod(socketPath, 0775); err != nil {
			fmt.Printf("Failed to set socket permissions: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Server listening on Unix socket: %s\n", socketPath)
		http.Serve(l, secureHeaders(http.DefaultServeMux))

	case serveMode == "tcp":
		addr := fmt.Sprintf("%s:%s", host, port)
		fmt.Printf("Server listening on http://%s\n", addr)
		http.ListenAndServe(addr, secureHeaders(http.DefaultServeMux))
	}
}
