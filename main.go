package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// TODO: truncate at 50,000 results

type Match struct {
	FileName string
	Left     string
	Right    string
}

const CONTEXT_LENGTH = 30

func isLetter(b byte) bool {
	return ('a' <= b && b <= 'z') || ('A' <= b && b <= 'Z')
}

// TODO: directly write matches to channel?
func findConcordance(page Page, rgx *regexp.Regexp, quitChannel chan struct{}) []Match {
	// TODO: do this at scraping
	text := strings.ReplaceAll(page.Text, "\r\n", " ")

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

		matches = append(matches, Match{
			FileName: page.FileName,
			Left:     text[leftStart:start],
			Right:    text[end:rightEnd],
		})

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
	flag.Parse()

	if *query != "" {
		runOneQuery(*query, *directory)
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
	Pages []string
}

type Page struct {
	FileName string
	Text     string
}

func loadPages(directory string) ([]Page, error) {
	files, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
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

	return pages, nil
}

func streamSearch(pages []Page, keyword string, quitChannel chan struct{}) (chan SearchResult, error) {
	var wg sync.WaitGroup
	resultsChannel := make(chan SearchResult, 1000)

	// The '\b' word boundary regex pattern is very slow. So we don't use it here and
	// instead filter for word boundaries inside `findConcordance`.
	rgx, err := regexp.Compile(regexp.QuoteMeta(keyword))
	if err != nil {
		return nil, err
	}

	for _, page := range pages {
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

func normalize(s string) string {
	// TODO: do Unicode normalization at scrape time
	t := transform.Chain(norm.NFD, runes.Remove(runes.Predicate(isNonAscii)), norm.NFC)
	result, _, _ := transform.String(t, s)
	return strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(result, "\n", ""), "\r", ""), "\t", "")
}

func isNonAscii(r rune) bool {
	return r > unicode.MaxASCII
}

type ServerConfig struct {
	Directory     string
	SlowMode      bool
	TimeOutMillis int
	Port          int
}

func handleConcord(config ServerConfig, pages []Page, writer http.ResponseWriter, req *http.Request) {
	startTime := time.Now()
	query := req.URL.Query()
	keyword := query.Get("w")

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
			s := fmt.Sprintf("{\"filename\":\"%s\",\"left\":\"%s\",\"right\":\"%s\"}\n", match.FileName, normalize(match.Left), normalize(match.Right))
			writer.Write([]byte(s))
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
	httpWriteFile(writer, "public/fast.html", "text/html")
}

func handleJs(writer http.ResponseWriter, req *http.Request) {
	httpWriteFile(writer, "public/fast.js", "application/javascript")
}

func handleCss(writer http.ResponseWriter, req *http.Request) {
	httpWriteFile(writer, "public/fast.css", "text/css")
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

func runOneQuery(query string, directory string) {
	pages, err := loadPages(directory)
	if err != nil {
		panic(err)
	}

	quitChannel := make(chan struct{})
	go func() {
		time.Sleep(time.Millisecond * time.Duration(1000))
		close(quitChannel)
	}()

	ch, err := streamSearch(pages, query, quitChannel)
	if err != nil {
		panic(err)
	}

	shouldQuit := false
	for result := range ch {
		for _, match := range result.Matches {
			fmt.Printf("{\"filename\":\"%s\",\"left\":\"%s\",\"right\":\"%s\"}\n", match.FileName, normalize(match.Left), normalize(match.Right))

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
}

func webServer(config ServerConfig) {
	pages, err := loadPages(config.Directory)
	if err != nil {
		// TODO: don't panic
		panic("could not load pages")
	}

	http.HandleFunc("/concord", func(writer http.ResponseWriter, req *http.Request) {
		handleConcord(config, pages, writer, req)
	})
	http.HandleFunc("/", handleIndex)
	// TODO: only serve static assets in dev
	http.HandleFunc("/static/fast.js", handleJs)
	http.HandleFunc("/static/fast.css", handleCss)
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
