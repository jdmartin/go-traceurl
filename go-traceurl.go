package main

import (
	"fmt"
	"html/template"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/didip/tollbooth/v7"
	"github.com/didip/tollbooth/v7/limiter"
)

var (
	cloudflareStatus bool
	formTemplate     *template.Template
	resultTemplate   *template.Template
	thereWasATimeout bool
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
	UseCount int `json:"UseCount"`
}

func main() {
	// Load the cached value on startup
	config, err := LoadConfig()
	if err != nil {
		fmt.Println("No cached value found.")
		config = &Config{UseCount: 0}
	}

	// Make sure we have a port to serve on
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

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
	handleSIGINT(config)

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

	addr := fmt.Sprintf(":%s", port)
	fmt.Printf("Server listening on http://localhost%s\n", addr)
	http.ListenAndServe(addr, secureHeaders(http.DefaultServeMux))
}

func handleSIGINT(config *Config) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	go func() {
		<-c
		fmt.Println("\nReceived SIGINT. Caching the current value...")

		// Cache the value on SIGINT
		if err := SaveConfig(config); err != nil {
			fmt.Println("Error saving the config:", err)
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

		w.Header().Set("Content-Security-Policy", fmt.Sprintf("default-src 'self'; script-src 'self' 'nonce-%s'; referrer 'no-referrer'", nonce))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Permissions-Policy", "accelerometer=(), ambient-light-sensor=(), autoplay=(), battery=(), camera=(), cross-origin-isolated=(), display-capture=(), document-domain=(), encrypted-media=(), execution-while-not-rendered=(), execution-while-out-of-viewport=(), fullscreen=(), geolocation=(), gyroscope=(), keyboard-map=(), magnetometer=(), microphone=(), midi=(), navigation-override=(), payment=(), picture-in-picture=(), publickey-credentials-get=(), screen-wake-lock=(), sync-xhr=(), usb=(), web-share=(), xr-spatial-tracking=()")

		// Call the next handler
		next.ServeHTTP(w, r)
	})
}
