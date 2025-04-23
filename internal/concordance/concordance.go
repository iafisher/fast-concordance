package concordance

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"sync"
	"time"
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

type IFinder interface {
	Find(page Page, outChannel chan Match, quitChannel chan struct{})
}

type Pages struct {
	Pages        []Page
	ManifestJson []byte
}

type Page struct {
	FileName string
	FilePath string
	Text     string
}

func LoadPages(directory string, fileNamesOnly bool, limit int) (Pages, error) {
	files, err := os.ReadDir(directory)
	if err != nil {
		return Pages{}, err
	}

	pages := []Page{}
	for _, file := range files {
		if limit != -1 && len(pages) == limit {
			break
		}

		txtPath := fmt.Sprintf("%s/%s/merged.txt", directory, file.Name())
		if file.IsDir() {
			data := []byte{}
			if !fileNamesOnly {
				data, err = os.ReadFile(txtPath)
				if err != nil {
					log.Printf("failed to load file: %s (%s)", txtPath, err)
					continue
				}
			}

			pages = append(pages, Page{FileName: file.Name(), FilePath: txtPath, Text: string(data)})
		}
	}

	manifestJson, err := os.ReadFile(fmt.Sprintf("%s/manifest.json", directory))
	if err != nil {
		return Pages{}, err
	}

	return Pages{Pages: pages, ManifestJson: manifestJson}, nil
}

func StreamSearch(pages Pages, keyword string, quitChannel chan struct{}, maxGoroutines int) (chan Match, error) {
	startTime := time.Now()

	var wg sync.WaitGroup
	outChannel := make(chan Match, 1000)

	finder, err := NewFinder(keyword)
	if err != nil {
		return nil, err
	}

	if maxGoroutines == -1 {
		for _, page := range pages.Pages {
			wg.Add(1)
			go func(page Page) {
				defer wg.Done()
				finder.Find(page, outChannel, quitChannel)
			}(page)
		}
	} else {
		workChannel := make(chan Page, len(pages.Pages))

		maxGoroutines = min(len(pages.Pages), maxGoroutines)
		if maxGoroutines == 0 {
			maxGoroutines = runtime.NumCPU()
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, page := range pages.Pages {
				workChannel <- page
			}
			close(workChannel)
		}()

		for i := 0; i < maxGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for page := range workChannel {
					finder.Find(page, outChannel, quitChannel)
				}
			}()
		}
	}

	go func() {
		wg.Wait()
		durationMs := time.Since(startTime).Milliseconds()
		log.Printf("goroutines exited after %d ms", durationMs)
		close(outChannel)
	}()

	return outChannel, nil
}
