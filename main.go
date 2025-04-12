package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"runtime/pprof"
	"sync"
	"time"
)

// TODO: truncate at 10,000 results (same as frontend)

type Match struct {
	FileName string `json:"filename"`
	Left     string `json:"left"`
	Right    string `json:"right"`
}

const MIN_KEYWORD_LENGTH = 4
const MAX_KEYWORD_LENGTH = 30
const CONTEXT_LENGTH = 40

func isLetter(b byte) bool {
	return ('a' <= b && b <= 'z') || ('A' <= b && b <= 'Z')
}

func isContinuationByte(b byte) bool {
	return b&0xC0 == 0x80
}

func isSingleByteChar(b byte) bool {
	return b&0x80 == 0
}

func SliceLeftUtf8(text string, index int, end int) string {
	for end >= 0 && isContinuationByte(text[end]) {
		end -= 1
	}
	return text[end:index]
}

func SliceRightUtf8(text string, index int, end int) string {
	if end == len(text) || isSingleByteChar(text[end]) {
		return text[index:end]
	} else {
		end += 1
		for end < len(text) && isContinuationByte(text[end]) {
			end += 1
		}
		return text[index:end]
	}
}

// TODO: directly write matches to channel?
func findConcordance(page Page, rgx *regexp.Regexp, quitChannel chan struct{}) []Match {
	text := page.Text
	indices := rgx.FindAllStringSubmatchIndex(text, -1)

	matches := []Match{}
	for _, pair := range indices {
		start := pair[0]
		end := pair[1]
		leftStart := max(0, start-CONTEXT_LENGTH)
		rightEnd := min(end+CONTEXT_LENGTH, len(text))

		// TODO: this doesn't work with Unicode
		if start > 0 && isLetter(text[start-1]) {
			continue
		}

		if end < len(text) && isLetter(text[end]) {
			continue
		}

		match := Match{
			FileName: page.FileName,
			Left:     SliceLeftUtf8(text, start, leftStart),
			Right:    SliceRightUtf8(text, end, rightEnd),
		}

		matches = append(matches, match)

		select {
		case _, ok := <-quitChannel:
			if !ok {
				break
			}
		default:
			continue
		}
	}

	return matches
}

func main() {
	directory := flag.String("directory", "", "serve this directory of ebook files")
	query := flag.String("query", "", "run a query instead of the web server")
	slow := flag.Bool("slow", false, "run the webserver in slow mode")
	timeOutMillis := flag.Int("timeout-ms", 1000, "time out requests after this many milliseconds")
	port := flag.Int("port", -1, "listen on this port")
	takeProfile := flag.Bool("profile", false, "take a pprof profile")
	flag.Parse()

	if *query != "" {
		runOneQuery(*query, *directory, *takeProfile)
		return
	}

	if *port == -1 {
		fmt.Fprintln(os.Stderr, "-port is required")
		os.Exit(1)
	}

	config := ServerConfig{
		Directory:     *directory,
		SlowMode:      *slow,
		TimeOutMillis: *timeOutMillis,
		Port:          *port,
	}

	webServer(config)
}

type Pages struct {
	Pages        []Page
	ManifestJson []byte
}

type Page struct {
	FileName string
	Text     string
}

func loadPages(directory string) (Pages, error) {
	files, err := os.ReadDir(directory)
	if err != nil {
		return Pages{}, err
	}

	pages := []Page{}
	for _, file := range files {
		txtPath := fmt.Sprintf("%s/%s/merged.txt", directory, file.Name())
		if file.IsDir() {
			data, err := os.ReadFile(txtPath)
			if err != nil {
				continue
			}

			pages = append(pages, Page{FileName: file.Name(), Text: string(data)})
		}
	}

	manifestJson, err := os.ReadFile(fmt.Sprintf("%s/manifest.json", directory))
	if err != nil {
		return Pages{}, err
	}

	return Pages{Pages: pages, ManifestJson: manifestJson}, nil
}

func streamSearch(pages Pages, keyword string, quitChannel chan struct{}) (chan SearchResult, error) {
	var wg sync.WaitGroup
	resultsChannel := make(chan SearchResult, 1000)

	// The '\b' word boundary regex pattern is very slow. So we don't use it here and
	// instead filter for word boundaries inside `findConcordance`.
	// TODO: case-insensitive matching - (?i) flag (but it's slow)
	rgx, err := regexp.Compile(regexp.QuoteMeta(keyword))
	if err != nil {
		return nil, err
	}

	for _, page := range pages.Pages {
		wg.Add(1)
		go func(page Page) {
			defer wg.Done()
			matches := findConcordance(page, rgx, quitChannel)

			resultsChannel <- SearchResult{
				NumBytes: len(page.Text),
				Matches:  matches,
			}
		}(page)
	}

	go func() {
		wg.Wait()
		close(resultsChannel)
	}()

	return resultsChannel, nil
}

type ServerConfig struct {
	Directory     string
	SlowMode      bool
	TimeOutMillis int
	Port          int
}

func writeError(writer http.ResponseWriter, message string) {
	writer.WriteHeader(http.StatusBadRequest)
	s := fmt.Sprintf("{\"error\":{\"message\":\"%s\"}}", message)
	writer.Write([]byte(s))
}

func handleConcord(config ServerConfig, pages Pages, writer http.ResponseWriter, req *http.Request) {
	startTime := time.Now()
	query := req.URL.Query()
	keyword := query.Get("w")

	if len(keyword) < MIN_KEYWORD_LENGTH {
		writeError(writer, fmt.Sprintf("The keyword must be at least %d letters long.", MIN_KEYWORD_LENGTH))
		return
	}

	if len(keyword) > MAX_KEYWORD_LENGTH {
		writeError(writer, fmt.Sprintf("The keyword canont be longer than %d letters.", MAX_KEYWORD_LENGTH))
		return
	}

	quitChannel := make(chan struct{})
	go func() {
		time.Sleep(time.Millisecond * time.Duration(config.TimeOutMillis))
		close(quitChannel)
	}()

	ch, err := streamSearch(pages, keyword, quitChannel)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	writer.Header().Set("Content-Type", "application/x-ndjson")
	flusher := writer.(http.Flusher)
	resultCount := 0
	shouldQuit := false
	for result := range ch {
		for _, match := range result.Matches {
			resultCount += 1
			jsonB, err := json.Marshal(match)
			if err != nil {
				continue
			}

			writer.Write(jsonB)
			writer.Write([]byte("\n"))
			flusher.Flush()
			if config.SlowMode {
				time.Sleep(10 * time.Millisecond)
			}

			select {
			case _, ok := <-quitChannel:
				if !ok {
					shouldQuit = true
					break
				}
			default:
				continue
			}
		}

		if shouldQuit {
			break
		}
	}

	durationMs := time.Since(startTime).Milliseconds()
	if shouldQuit {
		log.Printf("%d result(s) for '%v' in %d ms (timed out)", resultCount, keyword, durationMs)
	} else {
		log.Printf("%d result(s) for '%v' in %d ms", resultCount, keyword, durationMs)
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

func handleManifest(pages Pages, writer http.ResponseWriter, req *http.Request) {
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

func runOneQuery(query string, directory string, takeProfile bool) {
	pages, err := loadPages(directory)
	if err != nil {
		panic(err)
	}

	startTime := time.Now()
	quitChannel := make(chan struct{})
	go func() {
		time.Sleep(time.Millisecond * time.Duration(1000))
		close(quitChannel)
	}()

	if takeProfile {
		profFile, err := os.Create("fast.perf")
		defer func() { profFile.Close() }()
		if err != nil {
			panic(err)
		}
		pprof.StartCPUProfile(profFile)
	}

	ch, err := streamSearch(pages, query, quitChannel)
	if err != nil {
		panic(err)
	}

	var durationToFirstMs int64 = -1
	shouldQuit := false
	for result := range ch {
		for _, match := range result.Matches {
			if durationToFirstMs == -1 {
				durationToFirstMs = time.Since(startTime).Milliseconds()
			}

			_, err := json.Marshal(match)
			if err != nil {
				continue
			}
			// fmt.Println(string(jsonB))

			select {
			case _, ok := <-quitChannel:
				if !ok {
					shouldQuit = true
					break
				}
			default:
				continue
			}
		}

		if shouldQuit {
			break
		}
	}
	durationMs := time.Since(startTime).Milliseconds()
	if takeProfile {
		pprof.StopCPUProfile()
	}

	fmt.Printf("first:  % 6d ms\n", durationToFirstMs)
	fmt.Printf("last:   % 6d ms\n", durationMs)
}

func webServer(config ServerConfig) {
	pages, err := loadPages(config.Directory)
	if err != nil {
		// TODO: don't panic
		panic("could not load pages")
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

	// TODO: 404 page

	addr := fmt.Sprintf(":%d", config.Port)
	log.Printf("listening on %s", addr)
	log.Fatal("server failed", http.ListenAndServe(addr, nil))
}

type SearcherStats struct {
	NumMatches int
	NumFiles   int
	NumBytes   int
}

type SearchResult struct {
	NumBytes int
	Matches  []Match
}
