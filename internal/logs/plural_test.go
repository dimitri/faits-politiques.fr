package logs

import "testing"

func TestPlural(t *testing.T) {
	for _, tc := range []struct {
		n    int
		noun string
		want string
	}{
		{1, "page", "1 page"},
		{2, "page", "2 pages"},
		{0, "problem", "0 problems"},
		// The one that prompted this: "503 querys" was on screen.
		{503, "query", "503 queries"},
		{1, "query", "1 query"},
		// A vowel before the y keeps it.
		{2, "day", "2 days"},
		// Sibilants take -es.
		{2, "class", "2 classes"},
		{3, "batch", "3 batches"},
		{2, "dash", "2 dashes"},
		// Multi-word nouns pluralise their last word, which is what a
		// message assembled from several parts passes.
		{4, "source connector", "4 source connectors"},
		{2, "build artifact", "2 build artifacts"},
	} {
		if got := Plural(tc.n, tc.noun); got != tc.want {
			t.Errorf("Plural(%d, %q) = %q, want %q", tc.n, tc.noun, got, tc.want)
		}
	}
}
