//go:build linux && amd64 && cgo

package concordance

import "github.com/iafisher/fast-concordance/internal/simdsearch"

type Finder struct {
	keyword    string
	keywordLen int
}

func NewFinder(keyword string) Finder {
	return Finder{keyword: keyword, keywordLen: len(keyword)}
}

func (fdr *Finder) Find(page Page, outChannel chan Match, quitChannel chan struct{}) {
	// TODO: reduce duplication with non-SIMD implementation
	text := page.Text

	offset := 0
	for {
		start := simdsearch.Search(text, fdr.keyword, offset)
		if start == -1 {
			break
		}
		offset = start + 1
		end := start + fdr.keywordLen
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
