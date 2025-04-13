package concordance

import "testing"

func TestSliceUtf8(t *testing.T) {
	// the dash character is 3 bytes long
	s := "a–b–c"
	if SliceLeftUtf8(s, 4, 3) != "–" {
		t.Fatal()
	}

	if SliceLeftUtf8(s, 4, 2) != "–" {
		t.Fatal()
	}

	if SliceLeftUtf8(s, 4, 1) != "–" {
		t.Fatal()
	}

	if SliceRightUtf8(s, 5, 6) != "–" {
		t.Fatal()
	}

	if SliceRightUtf8(s, 5, 7) != "–" {
		t.Fatal()
	}

	if SliceRightUtf8(s, 5, 8) != "–" {
		t.Fatal()
	}
}
