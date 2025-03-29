package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
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

	concorder := BruteForceConcordanceFinder{}
	// _, err := measureConcordance(concorder, "examples/dostoyevsky/", keyword)
	concordances, err := measureConcordance(concorder, "downloads/", keyword)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%d\n", len(concordances))
	// for _, concordance := range concordances {
	// 	fmt.Printf("%s:\n", concordance.FileName)
	// 	printConcordance(concordance)
	// }
}

type DirectorySearcher struct {
	NumMatches int
	NumFiles   int
	NumBytes   int
}

func (ds *DirectorySearcher) searchDirectory(concorder ConcordanceFinder, directory string, keyword string) ([]Concordance, error) {
	files, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}

	r := []Concordance{}
	for _, file := range files {
		fullPath := fmt.Sprintf("%s/%s", directory, file.Name())
		if file.IsDir() {
			r2, err := ds.searchDirectory(concorder, fullPath, keyword)
			if err != nil {
				return nil, err
			}
			r = append(r, r2...)
			continue
		}

		data, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, err
		}
		ds.NumBytes += len(data)
		ds.NumFiles += 1

		c := concorder.FindConcordance(string(data), keyword)
		if len(c.Matches) == 0 {
			continue
		}

		c.FileName = file.Name()
		ds.NumMatches += len(c.Matches)
		r = append(r, c)
	}

	return r, nil
}

func measureConcordance(concorder ConcordanceFinder, directory string, keyword string) ([]Concordance, error) {
	start := time.Now()

	searcher := DirectorySearcher{}
	r, err := searcher.searchDirectory(concorder, directory, keyword)
	if err != nil {
		return nil, err
	}

	duration := time.Since(start)
	fmt.Println("perf:")
	fmt.Printf("  results: %d\n", searcher.NumMatches)
	fmt.Printf("  bytes:   %d\n", searcher.NumBytes)
	fmt.Printf("  files:   %d\n", searcher.NumFiles)
	fmt.Printf("  time:    %.3f ms\n", float64(duration.Microseconds())/1000.0)
	return r, nil
}
