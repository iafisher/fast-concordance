package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/iafisher/fast-concordance/internal/concordance"
	"github.com/iafisher/fast-concordance/internal/ratelimiter"
	"golang.org/x/sync/semaphore"
)

// TODO: truncate at 10,000 results (same as frontend)

const MIN_KEYWORD_LENGTH = 4
const MAX_KEYWORD_LENGTH = 30

func main() {
	directory := flag.String("directory", "", "serve this directory of ebook files")
	slow := flag.Bool("slow", false, "run the webserver in slow mode")
	timeOutMillis := flag.Int("timeout-ms", 1000, "time out requests after this many milliseconds")
	port := flag.Int("port", -1, "listen on this port")
	maxConcurrent := flag.Int("max-concurrent", 4, "maximum requests to allow at once")
	rateLimitRequests := flag.Int("rate-limit-requests", 10, "with -rate-limit-interval, maximum requests to allow in interval")
	rateLimitInterval := flag.Duration("rate-limit-interval", time.Second*10, "with -rate-limit-requests, maximum requests to allow in interval")
	rateLimitPenalty := flag.Duration("rate-limit-penalty", time.Minute, "penalty for rate-limited IPs")
	flag.Parse()

	if *directory == "" {
		fmt.Fprintln(os.Stderr, "-directory is required")
		os.Exit(1)
	}

	if *port == -1 {
		fmt.Fprintln(os.Stderr, "-port is required")
		os.Exit(1)
	}

	rateLimiter := ratelimiter.NewRateLimiter(*rateLimitRequests, *rateLimitInterval, *rateLimitPenalty)
	config := ServerConfig{
		Directory:     *directory,
		SlowMode:      *slow,
		TimeOutMillis: *timeOutMillis,
		Port:          *port,
		Semaphore:     semaphore.NewWeighted(int64(*maxConcurrent)),
		RateLimiter:   &rateLimiter,
	}

	webServer(config)
}

func webServer(config ServerConfig) {
	pages, err := concordance.LoadPages(config.Directory)
	if err != nil {
		log.Fatalf("could not load pages: %v", err)
	}

	// TODO: Prod URLs are rooted at `/concordance` while localhost URLs are rooted
	// at `/`, so we have to have duplicate entries here.
	http.HandleFunc("/concord", func(writer http.ResponseWriter, req *http.Request) {
		handleConcord(config, pages, writer, req)
	})
	http.HandleFunc("/concordance/concord", func(writer http.ResponseWriter, req *http.Request) {
		handleConcord(config, pages, writer, req)
	})
	http.HandleFunc("/", handleIndex)
	// TODO: only serve static assets in dev
	http.HandleFunc("/static/fast.js", handleJs)
	http.HandleFunc("/static/fast.css", handleCss)
	http.HandleFunc("/manifest", func(writer http.ResponseWriter, req *http.Request) {
		handleManifest(pages, writer, req)
	})
	http.HandleFunc("/concordance/static/fast.js", handleJs)
	http.HandleFunc("/concordance/static/fast.css", handleCss)
	http.HandleFunc("/concordance/manifest", func(writer http.ResponseWriter, req *http.Request) {
		handleManifest(pages, writer, req)
	})

	addr := fmt.Sprintf(":%d", config.Port)
	log.Printf("listening on %s", addr)
	log.Fatal("server failed", http.ListenAndServe(addr, nil))
}

type ServerConfig struct {
	Directory     string
	SlowMode      bool
	TimeOutMillis int
	Port          int
	RateLimiter   *ratelimiter.IpRateLimiter
	Semaphore     *semaphore.Weighted
}

func writeError(writer http.ResponseWriter, message string) {
	writer.WriteHeader(http.StatusBadRequest)
	s := fmt.Sprintf("{\"error\":{\"message\":\"%s\"}}", message)
	writer.Write([]byte(s))
}

type ServerStatusMessage struct {
	Status string `json:"status"`
}

func writeJsonLineIgnoreError(writer http.ResponseWriter, flusher http.Flusher, v any) {
	jsonB, err := json.Marshal(v)
	if err != nil {
		return
	}

	writer.Write(jsonB)
	writer.Write([]byte("\n"))
	flusher.Flush()
}

func handleConcord(config ServerConfig, pages concordance.Pages, writer http.ResponseWriter, req *http.Request) {
	startTime := time.Now()
	query := req.URL.Query()
	keyword := query.Get("w")

	if len(keyword) < MIN_KEYWORD_LENGTH {
		writeError(writer, fmt.Sprintf("The keyword must be at least %d letters long.", MIN_KEYWORD_LENGTH))
		return
	}

	if len(keyword) > MAX_KEYWORD_LENGTH {
		writeError(writer, fmt.Sprintf("The keyword cannot be longer than %d letters.", MAX_KEYWORD_LENGTH))
		return
	}

	ipList, ok := req.Header["X-Real-Ip"]

	ip := "unknown"
	if ok && len(ipList) > 0 {
		ip = ipList[0]
		if !config.RateLimiter.IsOk(ip, startTime) {
			writer.WriteHeader(http.StatusTooManyRequests)
			return
		}
	}

	flusher := writer.(http.Flusher)
	acquired := config.Semaphore.TryAcquire(1)
	if !acquired {
		writeJsonLineIgnoreError(writer, flusher, ServerStatusMessage{Status: "queued"})
		// `req.Context()` ensures that we no longer try to acquire if the request is cancelled.
		err := config.Semaphore.Acquire(req.Context(), 1)
		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer config.Semaphore.Release(1)

		writeJsonLineIgnoreError(writer, flusher, ServerStatusMessage{Status: "ready"})
	} else {
		defer config.Semaphore.Release(1)
	}

	quitChannel := make(chan struct{})
	if config.TimeOutMillis != -1 {
		go func() {
			time.Sleep(time.Millisecond * time.Duration(config.TimeOutMillis))
			close(quitChannel)
		}()
	}

	ch, err := concordance.StreamSearch(pages, keyword, quitChannel, -1)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	writer.Header().Set("Content-Type", "application/x-ndjson")
	resultCount := 0
	quitEarly := false
	for match := range ch {
		resultCount += 1
		writeJsonLineIgnoreError(writer, flusher, match)

		if config.SlowMode {
			time.Sleep(100 * time.Millisecond)
		}

		select {
		case _, ok = <-quitChannel:
			if !ok {
				quitEarly = true
			}
		default:
			continue
		}

		if quitEarly {
			break
		}
	}

	durationMs := time.Since(startTime).Milliseconds()
	if quitEarly {
		log.Printf("%d result(s) for '%v' in %d ms (timed out; ip: %s)", resultCount, keyword, durationMs, ip)
	} else {
		log.Printf("%d result(s) for '%v' in %d ms (ip: %s)", resultCount, keyword, durationMs, ip)
	}
}

func handleIndex(writer http.ResponseWriter, req *http.Request) {
	// We meant to only match a literal "/" path, but in Go "/" matches *every* path,
	// so we have to handle 404 here.
	if req.URL.Path != "/" {
		writer.WriteHeader(http.StatusNotFound)
		return
	}

	httpWriteFile(writer, "public/fast.html", "text/html")
}

func handleJs(writer http.ResponseWriter, req *http.Request) {
	httpWriteFile(writer, "public/fast.js", "application/javascript")
}

func handleCss(writer http.ResponseWriter, req *http.Request) {
	httpWriteFile(writer, "public/fast.css", "text/css")
}

func handleManifest(pages concordance.Pages, writer http.ResponseWriter, req *http.Request) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Write(pages.ManifestJson)
}

func httpWriteFile(writer http.ResponseWriter, path string, mimeType string) {
	data, err := os.ReadFile(path)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	writer.Header().Set("Content-Type", mimeType)
	writer.Write(data)
}
