package main

import (
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/didip/tollbooth/v7"
	"github.com/didip/tollbooth/v7/limiter"
)

var Version = "2023.10.26.5"

var (
	cloudflareStatus         bool
	formTemplate             *template.Template
	mode                     string
	resultTemplate           *template.Template
	thereWasATimeout         bool
	thereWasAValidationError bool
)

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

type Config struct {
	UseCount int
}

func main() {
	// Set counter to zero on startup
	config := &Config{UseCount: 0}

	// Detect dev or production mode
	mode = os.Getenv("MODE")

	// Make sure we have a port to serve on
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Allow setting of listen address
	host := os.Getenv("HOST")
	if host == "" {
		host = "127.0.0.1"
	}

	// Set default serveMode
	serveMode := os.Getenv("SERVE")
	if serveMode != "tcp" {
		serveMode = "socket"
	}

	// Set socket path for listenting
	socketPath := "/tmp/go-trace.sock"

	// Create a rate limiter with a limit of 1 requests per second
	lim := tollbooth.NewLimiter(1, &limiter.ExpirableOptions{
		DefaultExpirationTTL: time.Hour,
	})
	lim.SetIPLookups([]string{"RemoteAddr", "X-Forwarded-For", "X-Real-IP"})

	// Set the headers for rate limiting
	lim.SetTokenBucketExpirationTTL(time.Hour)
	lim.SetBasicAuthExpirationTTL(time.Hour)
	lim.SetHeaderEntryExpirationTTL(time.Hour)

	// Handle SIGINT signal
	handleSIGINT(config, socketPath, serveMode)

	// Define templates.
	formTemplate = template.Must(template.ParseFiles("static/form.html"))
	resultTemplate = template.Must(template.ParseFiles("static/result.html"))

	// Establish Routes
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Limit the request using the rate limiter
		httpError := tollbooth.LimitByRequest(lim, w, r)
		if httpError != nil {
			http.Error(w, httpError.Message, httpError.StatusCode)
			return
		}
		// Handle the request
		homeHandler(w, r, config)
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
		traceHandler(w, r, config)
	})

	http.HandleFunc("/static/css/", cssHandler)
	http.HandleFunc("/static/data/", dataHandler)
	http.HandleFunc("/static/js/", jsHandler)

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

func handleSIGINT(config *Config, socketPath string, serveMode string) {
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
		if mode == "production" {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
		}

		// Call the next handler
		next.ServeHTTP(w, r)
	})
}
