package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

type Concordance struct {
	Keyword string
	Matches []Match
}

type Match struct {
	Left  string
	Right string
}

const CONTEXT_LENGTH = 30

func findConcordance(text string, keyword string) Concordance {
	text = strings.ReplaceAll(text, "\r\n", " ")

	// TODO: match word boundaries only
	// TODO: escape regex characters in `keyword` itself
	rgx := regexp.MustCompile(keyword)

	indices := rgx.FindAllStringSubmatchIndex(text, -1)

	matches := []Match{}
	for _, pair := range indices {
		start := pair[0]
		end := pair[1]
		matches = append(matches, Match{
			Left:  text[start-CONTEXT_LENGTH : start],
			Right: text[end : end+CONTEXT_LENGTH],
		})
	}

	return Concordance{
		Keyword: keyword, Matches: matches,
	}
}

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

	keyword := os.Args[1]

	data, err := os.ReadFile("examples/pride-and-prejudice.txt")
	if err != nil {
		panic(err)
	}

	text := string(data)

	concordance := findConcordance(text, keyword)
	printConcordance(concordance)
}
