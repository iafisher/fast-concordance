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

const MIN_KEYWORD_LENGTH = 4
const MAX_KEYWORD_LENGTH = 30

func main() {
	directory := flag.String("directory", "", "serve this directory of ebook files")
	slow := flag.Bool("slow", false, "run the webserver in slow mode")
	port := flag.Int("port", -1, "listen on this port")
	maxConcurrent := flag.Int("max-concurrent", 4, "maximum requests to allow at once")
	limitTexts := flag.Int("limit-texts", -1, "load a subset of texts")
	rateLimitRequests := flag.Int("rate-limit-requests", 10, "with -rate-limit-interval, maximum requests to allow in interval")
	rateLimitInterval := flag.Duration("rate-limit-interval", time.Second*10, "with -rate-limit-requests, maximum requests to allow in interval")
	rateLimitPenalty := flag.Duration("rate-limit-penalty", time.Minute, "penalty for rate-limited IPs")
	timeOutQuery := flag.Duration("timeout-query", time.Second, "time-out for concordance queries")
	timeOutReadHeader := flag.Duration("timeout-read-header", time.Second*10, "time-out for HTTP headers")
	timeOutRead := flag.Duration("timeout-read", time.Second*10, "time-out for reading HTTP request")
	timeOutWrite := flag.Duration("timeout-write", time.Minute, "time-out for writing HTTP response (all endpoints)")
	timeOutIdle := flag.Duration("timeout-idle", 2*time.Minute, "time-out for idle connections")
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
		Directory:         *directory,
		SlowMode:          *slow,
		TimeOutQuery:      *timeOutQuery,
		TimeOutReadHeader: *timeOutReadHeader,
		TimeOutRead:       *timeOutRead,
		TimeOutWrite:      *timeOutWrite,
		TimeOutIdle:       *timeOutIdle,
		Port:              *port,
		Semaphore:         semaphore.NewWeighted(int64(*maxConcurrent)),
		RateLimiter:       &rateLimiter,
		LimitTexts:        *limitTexts,
	}

	webServer(config)
}

func webServer(config ServerConfig) {
	pages, err := concordance.LoadPages(config.Directory, false, config.LimitTexts)
	if err != nil {
		log.Fatalf("could not load pages: %v", err)
	}

	handler := &http.ServeMux{}

	handler.HandleFunc("/concord", func(writer http.ResponseWriter, req *http.Request) {
		handleConcord(config, pages, writer, req)
	})
	handler.HandleFunc("/", handleIndex)
	handler.HandleFunc("/static/fast.js", handleJs)
	handler.HandleFunc("/static/fast.css", handleCss)
	handler.HandleFunc("/manifest", func(writer http.ResponseWriter, req *http.Request) {
		handleManifest(pages, writer, req)
	})

	addr := fmt.Sprintf(":%d", config.Port)
	server := &http.Server{
		ReadHeaderTimeout: config.TimeOutReadHeader,
		ReadTimeout:       config.TimeOutRead,
		WriteTimeout:      config.TimeOutWrite,
		IdleTimeout:       config.TimeOutIdle,
		Addr:              addr,
		Handler:           handler,
	}

	log.Printf("listening on %s", addr)
	log.Fatal("server failed", server.ListenAndServe())
}

type ServerConfig struct {
	Directory         string
	SlowMode          bool
	TimeOutQuery      time.Duration
	TimeOutReadHeader time.Duration
	TimeOutRead       time.Duration
	TimeOutWrite      time.Duration
	TimeOutIdle       time.Duration
	Port              int
	RateLimiter       *ratelimiter.IpRateLimiter
	Semaphore         *semaphore.Weighted
	LimitTexts        int
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
	go func() {
		select {
		case <-req.Context().Done():
		case <-time.After(config.TimeOutQuery):
		}
		close(quitChannel)
	}()

	ch, err := concordance.StreamSearch(pages, keyword, quitChannel, 0)
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
		case <-quitChannel:
			quitEarly = true
		default:
			continue
		}

		if quitEarly {
			break
		}
	}

	durationMs := time.Since(startTime).Milliseconds()
	if quitEarly {
		log.Printf("%d result(s) for '%v' in %d ms (timed out/cancelled; ip: %s)", resultCount, keyword, durationMs, ip)
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
