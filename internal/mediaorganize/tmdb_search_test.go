package mediaorganize

import (
	"testing"
)

func TestParseTMDBQueryID(t *testing.T) {
	cases := map[string]string{
		"980477":        "980477",
		"tmdb-980477":   "980477",
		"tmdb:980477":   "980477",
		"tmdbid=980477": "980477",
		"{tmdb-980477}": "980477",
		"TMDB-123":      "123",
		"tmdb-2012":     "2012",
		"哪吒之魔童闹海":       "",
		"暗战 1999":       "",
		"2012":          "",
		"2046":          "",
		"":              "",
	}
	for in, want := range cases {
		if got := parseTMDBQueryID(in); got != want {
			t.Fatalf("parseTMDBQueryID(%q)=%q want %q", in, got, want)
		}
	}
}
