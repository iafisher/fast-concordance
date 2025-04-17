package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/iafisher/fast-concordance/internal/concordance"
)

func main() {
	directory := flag.String("directory", "", "serve this directory of ebook files")
	query := flag.String("query", "", "keyword to query")
	takeProfile := flag.Bool("profile", false, "take a pprof profile")
	maxGoroutines := flag.Int("max-goroutines", -1, "use this many goroutines (-1 for no limit -- the default, 0 for 1 per CPU core)")
	measureBaseline := flag.Bool("measure-baseline", false, "measure baseline performance")
	results := flag.Int("results", 0, "show this many results (-1 for all, 0 for none)")
	flag.Parse()

	if *directory == "" {
		fmt.Fprintln(os.Stderr, "-directory is required")
		os.Exit(1)
	}

	if *measureBaseline {
		runMeasureBaseline(*directory, *maxGoroutines)
	} else {
		if *query == "" {
			fmt.Fprintln(os.Stderr, "-query is required")
			os.Exit(1)
		}

		runOneQuery(*query, *directory, *takeProfile, *maxGoroutines, *results)
	}
}

func countLetterA(page concordance.Page) int {
	n := 0
	for _, c := range page.Text {
		if c == 'a' {
			n += 1
		}
	}
	return n
}

func runMeasureBaseline(directory string, maxGoroutines int) {
	pages, err := concordance.LoadPages(directory)
	if err != nil {
		panic(err)
	}

	startTime := time.Now()
	total := 0
	if maxGoroutines == 1 {
		for _, page := range pages.Pages {
			total += countLetterA(page)
		}
	} else {
		var wg sync.WaitGroup

		output := make([]int, len(pages.Pages))

		if maxGoroutines == -1 {
			for i, page := range pages.Pages {
				wg.Add(1)
				go func(i int, page concordance.Page) {
					defer wg.Done()
					output[i] = countLetterA(page)
				}(i, page)
			}
		} else {
			// TODO: pull this logic out into a common function in `lib.go`
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

					for pageIndex, page := range pages.Pages[start:end] {
						output[pageIndex+start] = countLetterA(page)
					}
				}(i)
			}
		}

		wg.Wait()

		for _, n := range output {
			total += n
		}
	}
	durationMillis := time.Since(startTime).Milliseconds()

	fmt.Printf("result:   %d\n", total)
	fmt.Printf("duration: %d ms\n", durationMillis)
}

func runOneQuery(query string, directory string, takeProfile bool, maxGoroutines int, results int) {
	pages, err := concordance.LoadPages(directory)
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

	ch, err := concordance.StreamSearch(pages, query, quitChannel, maxGoroutines)
	if err != nil {
		panic(err)
	}

	var durationToFirstMs int64 = -1
	n := 0
	resultsShown := 0
	for match := range ch {
		if durationToFirstMs == -1 {
			durationToFirstMs = time.Since(startTime).Milliseconds()
		}

		jsonB, err := json.Marshal(match)
		if err != nil {
			continue
		}

		if resultsShown < results {
			fmt.Println(string(jsonB))
			resultsShown += 1
		}
		n += 1

		select {
		case _, ok := <-quitChannel:
			if !ok {
				break
			}
		default:
			continue
		}
	}
	durationMs := time.Since(startTime).Milliseconds()
	if takeProfile {
		pprof.StopCPUProfile()
	}

	fmt.Printf("results: %d\n", n)
	fmt.Printf("first:   % 6d ms\n", durationToFirstMs)
	fmt.Printf("last:    % 6d ms\n", durationMs)
}
