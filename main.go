package main

import (
	"encoding/json"
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

type Concordance struct {
	Keyword  string
	FileName string
	Matches  []Match
}

type Match struct {
	Left  string
	Right string
}

type ConcordanceFinder interface {
	FindConcordance(text string, keyword string) Concordance
}

type BruteForceConcordanceFinder struct{}

func (cf BruteForceConcordanceFinder) FindConcordance(text string, keyword string) Concordance {
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
	}

	return Concordance{
		Keyword: keyword, Matches: matches,
	}
}

const CONTEXT_LENGTH = 30

func printConcordance(concordance Concordance) {
	for _, match := range concordance.Matches {
		fmt.Printf("%s%s%s\n", match.Left, concordance.Keyword, match.Right)
	}
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: ./fast-concordance <keyword>")
		os.Exit(1)
	}

	// TODO: parse command-line args
	keyword := os.Args[1]
	if keyword == "-serve" {
		webServer()
	} else {
		searcher := ParallelDirectorySearcher{}
		concorder := BruteForceConcordanceFinder{}
		// _, err := measureConcordance(concorder, "examples/dostoyevsky/", keyword)
		concordances, err := measureConcordance(&searcher, concorder, "downloads/", keyword)
		if err != nil {
			panic(err)
		}

		fmt.Printf("files: %d\n", len(concordances))
		// for _, concordance := range concordances {
		// 	fmt.Printf("%s:\n", concordance.FileName)
		// 	printConcordance(concordance)
		// }
	}
}

func handleConcord(writer http.ResponseWriter, req *http.Request) {
	query := req.URL.Query()
	keyword := query.Get("w")
	searcher := ParallelDirectorySearcher{}
	concorder := BruteForceConcordanceFinder{}

	// TODO: can't hard-code directory
	start := time.Now()
	concordances, err := searcher.SearchDirectory(concorder, "downloads", keyword)
	duration := time.Since(start)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		log.Printf("error finding concordance")
		return
	} else {
		log.Printf("request took %.3f ms", float64(duration.Microseconds())/1000.0)
		start = time.Now()
		jsonBytes, err := json.Marshal(concordances)
		duration = time.Since(start)
		log.Printf("json took %.3f ms", float64(duration.Microseconds())/1000.0)
		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			log.Printf("error marshaling JSON")
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.Write(jsonBytes)
	}
}

func streamSearch(directory string, keyword string) chan ParallelResult {
	files, err := os.ReadDir(directory)
	if err != nil {
		return nil
	}

	var wg sync.WaitGroup
	var concorder BruteForceConcordanceFinder
	resultsChannel := make(chan ParallelResult, 100)
	for _, file := range files {
		if file.IsDir() {
			txtPath := fmt.Sprintf("%s/%s/merged.txt", directory, file.Name())

			wg.Add(1)
			go func(path string) {
				defer wg.Done()
				parallelDoOne(concorder, path, keyword, resultsChannel)
			}(txtPath)
		}
	}

	go func() {
		wg.Wait()
		close(resultsChannel)
	}()

	return resultsChannel
}

func normalize(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.Predicate(isNonAscii)), norm.NFC)
	result, _, _ := transform.String(t, s)
	return strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(result, "\n", ""), "\r", ""), "\t", "")
}

func isNonAscii(r rune) bool {
	return r > unicode.MaxASCII
}

func handleConcord2(writer http.ResponseWriter, req *http.Request) {
	query := req.URL.Query()
	keyword := query.Get("w")

	// TODO: can't hard-code directory
	ch := streamSearch("downloads", keyword)
	if ch == nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	writer.Header().Set("Content-Type", "application/x-ndjson")
	flusher := writer.(http.Flusher)
	for result := range ch {
		for _, match := range result.Concordance.Matches {
			s := fmt.Sprintf("{\"filename\":\"%s\",\"left\":\"%s\",\"right\":\"%s\"}\n", result.Concordance.FileName, normalize(match.Left), normalize(match.Right))
			writer.Write([]byte(s))
			flusher.Flush()
		}
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

func webServer() {
	http.HandleFunc("/concord", handleConcord2)
	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/fast.js", handleJs)
	http.HandleFunc("/fast.css", handleCss)
	addr := ":8722"
	log.Printf("listening on %s", addr)
	log.Fatal("server failed", http.ListenAndServe(addr, nil))
}

type DirectorySearcher interface {
	SearchDirectory(concorder ConcordanceFinder, directory string, keyword string) ([]Concordance, error)
	Stats() SearcherStats
}

type SearcherStats struct {
	NumMatches int
	NumFiles   int
	NumBytes   int
}

type ParallelDirectorySearcher struct {
	stats SearcherStats
}

func (ds ParallelDirectorySearcher) Stats() SearcherStats {
	return ds.stats
}

type ParallelResult struct {
	NumBytes    int
	Concordance Concordance
}

func (ds *ParallelDirectorySearcher) SearchDirectory(concorder ConcordanceFinder, directory string, keyword string) ([]Concordance, error) {
	files, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}

	var wg sync.WaitGroup
	resultsChannel := make(chan ParallelResult, 100)
	for _, file := range files {
		if file.IsDir() {
			txtPath := fmt.Sprintf("%s/%s/merged.txt", directory, file.Name())

			wg.Add(1)
			go func(path string) {
				defer wg.Done()
				parallelDoOne(concorder, path, keyword, resultsChannel)
			}(txtPath)
		}
	}

	go func() {
		wg.Wait()
		close(resultsChannel)
	}()

	r := []Concordance{}
	for result := range resultsChannel {
		if result.NumBytes == 0 {
			continue
		}

		ds.stats.NumBytes += result.NumBytes
		ds.stats.NumFiles += 1
		ds.stats.NumMatches += len(result.Concordance.Matches)
		r = append(r, result.Concordance)
	}
	return r, nil
}

func parallelDoOne(concorder ConcordanceFinder, path string, keyword string, resultsChannel chan ParallelResult) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	concordance := concorder.FindConcordance(string(data), keyword)

	resultsChannel <- ParallelResult{
		NumBytes:    len(data),
		Concordance: concordance,
	}
}

type SequentialDirectorySearcher struct {
	stats SearcherStats
}

func (ds SequentialDirectorySearcher) Stats() SearcherStats {
	return ds.stats
}

func (ds *SequentialDirectorySearcher) SearchDirectory(concorder ConcordanceFinder, directory string, keyword string) ([]Concordance, error) {
	files, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}

	r := []Concordance{}
	for _, file := range files {
		txtPath := fmt.Sprintf("%s/%s/merged.txt", directory, file.Name())
		if file.IsDir() {
			data, err := os.ReadFile(txtPath)
			if err != nil {
				continue
			}

			ds.stats.NumBytes += len(data)
			ds.stats.NumFiles += 1

			c := concorder.FindConcordance(string(data), keyword)
			if len(c.Matches) == 0 {
				continue
			}

			c.FileName = file.Name()
			ds.stats.NumMatches += len(c.Matches)
			r = append(r, c)
		}
	}

	return r, nil
}

func measureConcordance(searcher DirectorySearcher, concorder ConcordanceFinder, directory string, keyword string) ([]Concordance, error) {
	start := time.Now()

	r, err := searcher.SearchDirectory(concorder, directory, keyword)
	if err != nil {
		return nil, err
	}

	duration := time.Since(start)
	stats := searcher.Stats()
	fmt.Println("perf:")
	fmt.Printf("  results: %d\n", stats.NumMatches)
	fmt.Printf("  bytes:   %d\n", stats.NumBytes)
	fmt.Printf("  files:   %d\n", stats.NumFiles)
	fmt.Printf("  time:    %.3f ms\n", float64(duration.Microseconds())/1000.0)
	return r, nil
}
