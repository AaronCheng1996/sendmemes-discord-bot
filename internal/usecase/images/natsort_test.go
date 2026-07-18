package images

import (
	"sort"
	"testing"
)

func TestNaturalLess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{name: "numeric 2 before 10", a: "2.jpg", b: "10.jpg", want: true},
		{name: "numeric 10 not before 2", a: "10.jpg", b: "2.jpg", want: false},
		{name: "prefixed a2 before a10", a: "a2", b: "a10", want: true},
		{name: "prefixed a10 not before a2", a: "a10", b: "a2", want: false},
		{name: "case-insensitive equal prefix then numeric", a: "A2", b: "a10", want: true},
		{name: "case-insensitive letters", a: "Apple", b: "banana", want: true},
		{name: "shorter prefix sorts first", a: "a", b: "ab", want: true},
		{name: "longer does not sort before its prefix", a: "ab", b: "a", want: false},
		{name: "equal strings are not less", a: "img5.png", b: "img5.png", want: false},
		{name: "leading zeros equal value shorter first", a: "1.jpg", b: "01.jpg", want: true},
		{name: "natural order within name", a: "page2.jpg", b: "page10.jpg", want: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := naturalLess(tc.a, tc.b); got != tc.want {
				t.Fatalf("naturalLess(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestNaturalLessSortsComicPages(t *testing.T) {
	t.Parallel()

	got := []string{"10.jpg", "2.jpg", "1.jpg", "cover.jpg", "20.jpg", "3.jpg"}
	sort.SliceStable(got, func(i, j int) bool { return naturalLess(got[i], got[j]) })

	want := []string{"1.jpg", "2.jpg", "3.jpg", "10.jpg", "20.jpg", "cover.jpg"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sorted = %v, want %v", got, want)
		}
	}
}
