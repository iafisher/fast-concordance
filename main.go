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

type Concordance struct {
	Keyword  string
	FileName string
	Matches  []Match
}

type Match struct {
	FileName string
	Left     string
	Right    string
}

const CONTEXT_LENGTH = 30

func findConcordance(text string, keyword string, quitChannel chan struct{}) Concordance {
	text = strings.ReplaceAll(text, "\r\n", " ")

	// TODO: match word boundaries only
	// TODO: escape regex characters in `keyword` itself
	rgx := regexp.MustCompile(keyword)

	indices := rgx.FindAllStringSubmatchIndex(text, -1)

	matches := []Match{}
	for _, pair := range indices {
		start := pair[0]
		end := pair[1]
		leftStart := max(0, start-CONTEXT_LENGTH)
		rightEnd := min(end+CONTEXT_LENGTH, len(text))
		matches = append(matches, Match{
			Left:  text[leftStart:start],
			Right: text[end:rightEnd],
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

	return Concordance{
		Keyword: keyword, Matches: matches,
	}
}

func main() {
	directory := flag.String("directory", "", "serve this directory of ebook files")
	slow := flag.Bool("slow", false, "run the webserver in slow mode")
	timeOutMillis := flag.Int("timeout-ms", 1000, "time out requests after this many milliseconds")
	flag.Parse()

	config := ServerConfig{
		Directory:     *directory,
		SlowMode:      *slow,
		TimeOutMillis: *timeOutMillis,
	}

	webServer(config)
}

type Pages struct {
	Pages []string
}

func (p *Pages) Load(directory string) error {
	files, err := os.ReadDir(directory)
	if err != nil {
		return err
	}

	for _, file := range files {
		txtPath := fmt.Sprintf("%s/%s/merged.txt", directory, file.Name())
		if file.IsDir() {
			data, err := os.ReadFile(txtPath)
			if err != nil {
				continue
			}

			p.Pages = append(p.Pages, string(data))
		}
	}

	return nil
}

func streamSearch(pages Pages, keyword string, quitChannel chan struct{}) chan SearchResult {
	var wg sync.WaitGroup
	resultsChannel := make(chan SearchResult, 1000)

	for _, page := range pages.Pages {
		wg.Add(1)
		go func(page string) {
			defer wg.Done()
			concordance := findConcordance(page, keyword, quitChannel)

			resultsChannel <- SearchResult{
				NumBytes:    len(page),
				Concordance: concordance,
			}
		}(page)
	}

	go func() {
		wg.Wait()
		close(resultsChannel)
	}()

	return resultsChannel
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
}

func handleConcord(config ServerConfig, pages Pages, writer http.ResponseWriter, req *http.Request) {
	startTime := time.Now()
	query := req.URL.Query()
	keyword := query.Get("w")

	quitChannel := make(chan struct{})
	go func() {
		time.Sleep(time.Millisecond * time.Duration(config.TimeOutMillis))
		close(quitChannel)
	}()

	ch := streamSearch(pages, keyword, quitChannel)
	if ch == nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	writer.Header().Set("Content-Type", "application/x-ndjson")
	flusher := writer.(http.Flusher)
	resultCount := 0
	shouldQuit := false
	for result := range ch {
		for _, match := range result.Concordance.Matches {
			resultCount += 1
			s := fmt.Sprintf("{\"filename\":\"%s\",\"left\":\"%s\",\"right\":\"%s\"}\n", result.Concordance.FileName, normalize(match.Left), normalize(match.Right))
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

func webServer(config ServerConfig) {
	var pages Pages
	err := pages.Load(config.Directory)
	if err != nil {
		panic("could not load pages")
	}

	http.HandleFunc("/concord", func(writer http.ResponseWriter, req *http.Request) {
		handleConcord(config, pages, writer, req)
	})
	http.HandleFunc("/", handleIndex)
	// TODO: only serve static assets in dev
	http.HandleFunc("/static/fast.js", handleJs)
	http.HandleFunc("/static/fast.css", handleCss)
	// TODO: don't hard-code port
	addr := ":8722"
	log.Printf("listening on %s", addr)
	log.Fatal("server failed", http.ListenAndServe(addr, nil))
}

type SearcherStats struct {
	NumMatches int
	NumFiles   int
	NumBytes   int
}

type SearchResult struct {
	NumBytes    int
	Concordance Concordance
}
