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
	// Create a rate limiter with a limit of 10 requests per minute
	lim := tollbooth.NewLimiter(1, &limiter.ExpirableOptions{
		DefaultExpirationTTL: time.Hour,
	})
	lim.SetIPLookups([]string{"RemoteAddr", "X-Forwarded-For", "X-Real-IP"})

	// Set the headers for rate limiting
	lim.SetTokenBucketExpirationTTL(time.Hour)
	lim.SetBasicAuthExpirationTTL(time.Hour)
	lim.SetHeaderEntryExpirationTTL(time.Hour)

	// Handle SIGINT signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	// Load the cached value on startup
	config, err := LoadConfig()
	if err != nil {
		fmt.Println("No cached value found.")
		config = &Config{UseCount: 0}
	}

	// Listen for SIGINT
	go func() {
		<-c
		fmt.Println("\nReceived SIGINT. Caching the current value...")

		// Cache the value on SIGINT
		if err := SaveConfig(config); err != nil {
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
		// Limit the request using the rate limiter
		httpError := tollbooth.LimitByRequest(lim, w, r)
		if httpError != nil {
			http.Error(w, httpError.Message, httpError.StatusCode)
			return
		}
		// Handle the request
		homeHandler(w, r, config)
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
	http.HandleFunc("/timeout/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/timeout.html")
	})
	http.HandleFunc("/static/css/", cssHandler)
	http.HandleFunc("/static/js/", jsHandler)
	http.HandleFunc("/static/data/", dataHandler)

	addr := fmt.Sprintf(":%s", port)
	fmt.Printf("Server listening on http://localhost%s\n", addr)
	http.ListenAndServe(addr, nil)
}
