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

	i = Search("asdfhjkasdfhklasdhfklasdjfhjkasdjhfjkasdhvajkdv", "lasdh", 14)
	if i != -1 {
		t.Fatal()
	}
}
