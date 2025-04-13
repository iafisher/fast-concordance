package pkg

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"sync"
)

type Match struct {
	FileName string `json:"filename"`
	Left     string `json:"left"`
	Right    string `json:"right"`
}

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

func findConcordance(page Page, rgx *regexp.Regexp, outChannel chan Match, quitChannel chan struct{}) {
	text := page.Text
	indices := rgx.FindAllStringSubmatchIndex(text, -1)

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
		outChannel <- match

		select {
		case _, ok := <-quitChannel:
			if !ok {
				break
			}
		default:
			continue
		}
	}
}

type Pages struct {
	Pages        []Page
	ManifestJson []byte
}

type Page struct {
	FileName string
	Text     string
}

func LoadPages(directory string) (Pages, error) {
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

func StreamSearch(pages Pages, keyword string, quitChannel chan struct{}, maxGoroutines int) (chan Match, error) {
	var wg sync.WaitGroup
	outChannel := make(chan Match, 1000)

	// The '\b' word boundary regex pattern is very slow. So we don't use it here and
	// instead filter for word boundaries inside `findConcordance`.
	// TODO: case-insensitive matching - (?i) flag (but it's slow)
	rgx, err := regexp.Compile(regexp.QuoteMeta(keyword))
	if err != nil {
		return nil, err
	}

	if maxGoroutines == -1 {
		for _, page := range pages.Pages {
			wg.Add(1)
			go func(page Page) {
				defer wg.Done()
				findConcordance(page, rgx, outChannel, quitChannel)
			}(page)
		}
	} else {
		maxGoroutines = min(len(pages.Pages), maxGoroutines)
		if maxGoroutines == 0 {
			maxGoroutines = runtime.NumCPU()
		}

		rangeLen := len(pages.Pages) / maxGoroutines

		for i := range maxGoroutines {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()

				start := i * rangeLen
				var end int
				if i == maxGoroutines-1 {
					end = len(pages.Pages)
				} else {
					end = start + rangeLen
				}

				for _, page := range pages.Pages[start:end] {
					findConcordance(page, rgx, outChannel, quitChannel)
				}
			}(i)
		}
	}

	go func() {
		wg.Wait()
		close(outChannel)
	}()

	return outChannel, nil
}
