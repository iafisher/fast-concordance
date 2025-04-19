package simdsearch

import (
	"testing"
)

func TestSearch(t *testing.T) {
	i := Search("asdfhjkasdfhklasdhfklasdjfhjkasdjhfjkasdhvajkdv", "lasdh", 0)
	if i != 13 {
		t.Fatal()
	}

	i = Search("asdfhjkasdfhklasdhfklasdjfhjkasdjhfjkasdhvajkdv", "lasdh", 1)
	if i != 13 {
		t.Fatal()
	}

	i = Search("asdfhjkasdfhklasdhfklasdjfhjkasdjhfjkasdhvajkdv", "lasdh", 10)
	if i != 13 {
		t.Fatal()
	}

	expectNoMatch(t, "asdfhjkasdfhklasdhfklasdjfhjkasdjhfjkasdhvajkdv", "lasdh", 14)
	expectNoMatch(t, " Gerani Gerani Gerani Gerani Gerani Gerani", "vampire", 0)
	expectNoMatch(t, "And in the window pots of perfumed flowers, Geraniums, asters, wallflowers, violets. And in one window stood the traveller.", "vampire", 0)
}

func expectMatch(t *testing.T, pos int, text string, keyword string, offset int) {
	i := Search(text, keyword, offset)
	if i == -1 {
		t.Errorf("expected to find '%s' in '%s' but didn't", keyword, text)
	} else if i != pos {
		t.Errorf("expected to find '%s' in '%s' at %d but found instead at %d", keyword, text, pos, i)
	}
}

func expectNoMatch(t *testing.T, text string, keyword string, offset int) {
	t.Helper()

	i := Search(text, keyword, offset)
	if i != -1 {
		t.Errorf("expected no match but got one at i=%d: '%s'", i, text[i:i+len(keyword)])
	}
}
