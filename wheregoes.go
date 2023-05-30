package main

import (
	"fmt"
	"html/template"
	"net/http"
	"strings"
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
	RedirectURL string
	Hops        []Hop
	LastIndex   int
	StatusCode  int
}

func main() {
	formTemplate = template.Must(template.ParseFiles("form.html"))
	resultTemplate = template.Must(template.ParseFiles("result.html"))

	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/trace", traceHandler)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	fmt.Println("Server listening on http://localhost:8080")
	http.ListenAndServe(":8080", nil)
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		formTemplate.Execute(w, nil)
	} else {
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

func traceHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		url := r.FormValue("url")

		redirectURL, hops, err := followRedirects(url)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error following redirects: %s", err), http.StatusInternalServerError)
			return
		}

		lastIndex := len(hops) - 1

		data := ResultData{
			RedirectURL: redirectURL,
			Hops:        hops,
			LastIndex:   lastIndex,
		}

		resultTemplate.Execute(w, data)
	} else {
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

func followRedirects(url string) (string, []Hop, error) {
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

	for {
		resp, err := client.Get(url)
		if err != nil {
			return "", nil, fmt.Errorf("error accessing URL: %s", err)
		}
		defer resp.Body.Close()

		hop := Hop{
			Number:     number,
			URL:        url,
			StatusCode: resp.StatusCode,
		}
		hop.StatusCodeClass = getStatusCodeClass(resp.StatusCode)
		hops = append(hops, hop)

		if resp.StatusCode >= 300 && resp.StatusCode <= 399 {
			// Handle redirect
			url = resp.Header.Get("Location")
			if url == "" {
				return "", nil, fmt.Errorf("redirect location not found")
			}
			number++
			continue
		}

		return url, hops, nil
	}
}

func staticHandler(w http.ResponseWriter, r *http.Request) {
	filePath := "static/css/" + strings.TrimPrefix(r.URL.Path, "/static/css/")
	http.ServeFile(w, r, filePath)

	// Set the Content-Type header to "text/css"
	w.Header().Set("Content-Type", "text/css")
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
