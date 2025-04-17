package concordance

import "regexp"

type Finder struct {
	rgx *regexp.Regexp
}

func NewFinder(keyword string) Finder {
	// The '\b' word boundary regex pattern is very slow. So we don't use it here and
	// instead filter for word boundaries inside `findConcordance`.
	// TODO: case-insensitive matching - (?i) flag (but it's slow)
	rgx, err := regexp.Compile(regexp.QuoteMeta(keyword))
	if err != nil {
		panic(err)
	}
	return Finder{rgx: rgx}
}

func (fdr *Finder) Find(page Page, outChannel chan Match, quitChannel chan struct{}) {
	text := page.Text
	indices := fdr.rgx.FindAllStringSubmatchIndex(text, -1)

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
