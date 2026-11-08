package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestGraphSeriesTotal(t *testing.T) {
	t.Parallel()
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	in := []StarSeries{
		{Name: "later", Dates: []time.Time{base.AddDate(0, 0, 1)}},
		{Name: "earlier", Dates: []time.Time{base, base}},
		{Name: "empty"},
	}
	got := graphSeries(in, true)
	want := []time.Time{base, base, base.AddDate(0, 0, 1)}
	if len(got) != 4 || got[0].Name != "total" || !slices.Equal(got[0].Dates, want) {
		t.Fatalf("unexpected combined history: %+v", got)
	}
	if in[0].Name != "later" || len(in[0].Dates) != 1 || !in[0].Dates[0].Equal(want[2]) {
		t.Fatal("combining histories modified the input")
	}
	if disabled := graphSeries(in, false); len(disabled) != len(in) || disabled[0].Name != "later" {
		t.Fatalf("disabled total changed the series: %+v", disabled)
	}
	for _, theme := range themes {
		svg := renderStarGraph(graphSeries(nil, true), base, theme)
		if !strings.Contains(svg, "total 0") || !strings.Contains(svg, "No stars yet") {
			t.Fatal("empty total should render zero stars")
		}
	}
}

func TestRunTotalIncludesRepositoriesBeyondDisplayLimit(t *testing.T) {
	config := `
starHistory:
  repos: [r1, r2, r3, r4, r5, r6, empty]
  total: true
sections:
  - groups:
      - title: Tools
        repos:
          - name: card-only
`
	fake := &fakeGitHub{stars: map[string]int{"me/card-only": 100}}
	for i := 1; i <= 6; i++ {
		fake.stars[fmt.Sprintf("me/r%d", i)] = i
	}
	dir := setupRun(t, config, fake)
	out, err := runIn(dir)
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if !strings.Contains(out, "first 5 of 7 repositories") {
		t.Fatalf("missing display-limit warning: %s", out)
	}
	if fake.starRequests != 7 {
		t.Fatalf("history requests = %d, want 7", fake.starRequests)
	}
	for _, theme := range themes {
		data, err := os.ReadFile(filepath.Join(dir, "cards", starGraphFile(theme.Name)))
		if err != nil {
			t.Fatal(err)
		}
		svg := string(data)
		if !strings.Contains(svg, "Star history: total 21, r1 1, r2 2, r3 3, r4 4, r5 5") || strings.Contains(svg, "r6") || strings.Contains(svg, "card-only") {
			t.Fatalf("unexpected total or displayed repositories: %s", svg)
		}
	}
}
