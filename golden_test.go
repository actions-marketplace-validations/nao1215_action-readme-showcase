package main

import (
	"flag"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// update rewrites the golden files instead of comparing with them:
//
//	go test -run '^TestGolden_' -update
//
// Review the diff of testdata/golden/ before committing it: a golden file is
// the picture a reader's README shows.
var update = flag.Bool("update", false, "rewrite testdata/golden/ with the current output")

// checkGolden compares got with testdata/golden/<name>, or writes it there
// with -update.
func checkGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path) // #nosec G304 -- a fixed name under testdata/
	if err != nil {
		t.Fatalf("%v (run go test -run '^TestGolden_' -update to create it)", err)
	}
	if got != string(want) {
		t.Errorf("%s differs from the golden file; if the change is intended, run go test -run '^TestGolden_' -update and review the diff\n got:\n%s", name, got)
	}
}

// goldenCards are the shapes a card takes: a featured card with every field,
// a plain one with a long description that wraps and is cut, a card with
// counts in the thousands and no language color, a bare one, and one with
// both badges and a title long enough to be cut before them.
var goldenCards = map[string]CardData{
	"featured": {Name: "gup", FullName: "nao1215/gup", Description: "Update binaries installed with `go install` in parallel.",
		Language: "Go", LanguageColor: "#00ADD8", Stars: 622, Forks: 31, Release: "v1.10.5", Featured: true},
	"wrapped": {Name: "sqly", FullName: "nao1215/sqly", Description: "Run SQL against CSV, TSV, LTSV, JSON, Microsoft Excel and Parquet files, " +
		"with an interactive shell, completion, history and output in every format the input can take, all without a database server.",
		Language: "Go", LanguageColor: "#00ADD8", Stars: 9, Forks: 0},
	"thousands": {Name: "a-repository-with-a-name-far-too-long-for-the-card-title", FullName: "other/a-repository-with-a-name-far-too-long-for-the-card-title",
		Description: "日本語の説明と emoji 🚀 が混ざる説明文", Language: "C++", Stars: 12345, Forks: 1234, Release: "v10.0.0-rc.1"},
	"bare": {Name: "bare", FullName: "o/bare"},
	"awesome": {Name: "a-long-enough-name-to-meet-both-badges", FullName: "nao1215/a-long-enough-name-to-meet-both-badges",
		Description: "Listed in an awesome list and featured.", Language: "Go", LanguageColor: "#00ADD8", Stars: 128, Forks: 4,
		Release: "v1.0.0", Featured: true, Awesome: "avelino/awesome-go"},
}

func TestGolden_Cards(t *testing.T) {
	t.Parallel()
	for name, d := range goldenCards {
		for _, theme := range themes {
			t.Run(name+"-"+theme.Name, func(t *testing.T) {
				t.Parallel()
				checkGolden(t, "card-"+name+"-"+theme.Name+".svg", renderCard(d, theme))
			})
		}
	}
}

func TestGolden_StarGraph(t *testing.T) {
	t.Parallel()
	day := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }
	var gup, sqly []time.Time
	for i := range 40 {
		gup = append(gup, day(2021, time.March, 1).AddDate(0, 0, i*25))
	}
	for i := range 15 {
		sqly = append(sqly, day(2022, time.November, 5).AddDate(0, 0, i*40))
	}
	total := make([]time.Time, 0, len(gup)+len(sqly))
	total = append(total, gup...)
	total = append(total, sqly...)
	slices.SortFunc(total, time.Time.Compare)
	series := []StarSeries{
		{Name: "total (all public)", Scope: "Total: all public repositories owned by nao1215, including forks and archived repositories", Dates: total},
		{Name: "gup", Dates: gup},
		{Name: "sqly", Dates: sqly},
	}
	now := day(2026, time.November, 19)
	for _, theme := range themes {
		t.Run(theme.Name, func(t *testing.T) {
			t.Parallel()
			checkGolden(t, "star-history-"+theme.Name+".svg", renderStarGraph(series, now, theme))
		})
	}
}
