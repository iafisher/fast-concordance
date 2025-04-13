package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime/pprof"
	"time"

	"github.com/iafisher/fast-concordance/pkg"
)

func main() {
	directory := flag.String("directory", "", "serve this directory of ebook files")
	query := flag.String("query", "", "keyword to query")
	takeProfile := flag.Bool("profile", false, "take a pprof profile")
	maxGoroutines := flag.Int("max-goroutines", -1, "use this many goroutines (-1 for no limit -- the default, 0 for 1 per CPU core)")
	flag.Parse()

	if *directory == "" {
		fmt.Fprintln(os.Stderr, "-directory is required")
		os.Exit(1)
	}

	if *query == "" {
		fmt.Fprintln(os.Stderr, "-query is required")
		os.Exit(1)
	}

	runOneQuery(*query, *directory, *takeProfile, *maxGoroutines)
}

func runOneQuery(query string, directory string, takeProfile bool, maxGoroutines int) {
	pages, err := pkg.LoadPages(directory)
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

	ch, err := pkg.StreamSearch(pages, query, quitChannel, maxGoroutines)
	if err != nil {
		panic(err)
	}

	var durationToFirstMs int64 = -1
	n := 0
	for match := range ch {
		if durationToFirstMs == -1 {
			durationToFirstMs = time.Since(startTime).Milliseconds()
		}

		_, err := json.Marshal(match)
		if err != nil {
			continue
		}
		// fmt.Println(string(jsonB))
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
