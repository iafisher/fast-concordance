package simdtest

import (
	"testing"
)

func TestSearch(t *testing.T) {
	i := Search("asdfhjkasdfhklasdhfklasdjfhjkasdjhfjkasdhvajkdv", "lasdh")
	if i != 13 {
		t.Fatal()
	}
}
