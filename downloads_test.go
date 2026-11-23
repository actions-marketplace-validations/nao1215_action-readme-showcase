package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseConfigDownloads(t *testing.T) {
	t.Parallel()

	base := "owner: me\nsections:\n  - groups:\n      - title: T\n        repos:\n          - name: tool\n"
	for _, tc := range []struct {
		name     string
		yaml     string
		wantTop  int
		wantErr  string
		wantNone bool
	}{
		{name: "no downloads key leaves the graph off", yaml: "", wantNone: true},
		{name: "an empty downloads key draws the top 10", yaml: "downloads: {}\n", wantTop: 10},
		{name: "top is kept", yaml: "downloads:\n  top: 3\n", wantTop: 3},
		{name: "top 20 is the largest allowed", yaml: "downloads:\n  top: 20\n", wantTop: 20},
		{name: "top above 20 is rejected", yaml: "downloads:\n  top: 21\n", wantErr: "downloads.top is 21"},
		{name: "a negative top is rejected", yaml: "downloads:\n  top: -1\n", wantErr: "downloads.top is -1"},
		{name: "position bottom is accepted", yaml: "downloads:\n  position: bottom\n", wantTop: 10},
		{name: "an unknown position is rejected", yaml: "downloads:\n  position: left\n", wantErr: `downloads.position is "left"`},
		{name: "a misspelled key is rejected", yaml: "downloads:\n  tops: 3\n", wantErr: "field tops not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg, err := parseConfig([]byte(tc.yaml+base), "")
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want it to contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantNone {
				if cfg.Downloads != nil {
					t.Fatalf("Downloads = %+v, want nil", cfg.Downloads)
				}
				return
			}
			if got := cfg.Downloads.limit(); got != tc.wantTop {
				t.Fatalf("limit() = %d, want %d", got, tc.wantTop)
			}
		})
	}
}

// releasesServer serves a public repository listing and, per repository, one
// release per entry of counts with a single asset of that many downloads.
func releasesServer(t *testing.T, repos []map[string]any, counts map[string][]int, fetched map[string]int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/users/") {
			_ = json.NewEncoder(w).Encode(repos)
			return
		}
		repo, ok := strings.CutSuffix(strings.TrimPrefix(r.URL.Path, "/repos/"), "/releases")
		if !ok {
			http.NotFound(w, r)
			return
		}
		fetched[repo]++
		if r.URL.Query().Get("per_page") != "100" {
			t.Errorf("releases of %s: per_page = %q, want 100", repo, r.URL.Query().Get("per_page"))
		}
		var page int
		_, _ = fmt.Sscan(r.URL.Query().Get("page"), &page)
		all := counts[repo]
		var out []map[string]any
		for i := (page - 1) * 100; i < len(all) && i < page*100; i++ {
			out = append(out, map[string]any{"tag_name": fmt.Sprintf("v%d", i), "assets": []map[string]any{{"name": "a.tar.gz", "download_count": all[i]}}})
		}
		if out == nil {
			out = []map[string]any{}
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
}

func useREST(t *testing.T, url string) {
	t.Helper()
	old := restEndpoint
	restEndpoint = url
	t.Cleanup(func() { restEndpoint = old })
}

func TestFetchDownloadsSumsEveryAssetOfEveryReleaseAcrossPages(t *testing.T) {
	repo := func(owner, name string, private bool) map[string]any {
		return map[string]any{"name": name, "owner": map[string]string{"login": owner}, "private": private}
	}
	many := make([]int, 101) // two pages: 100 releases of 1 download, then one of 5
	for i := range 100 {
		many[i] = 1
	}
	many[100] = 5
	fetched := map[string]int{}
	srv := releasesServer(t,
		[]map[string]any{repo("me", "many", false), repo("me", "one", false), repo("me", "none", false), repo("me", "secret", true), repo("other", "theirs", false)},
		map[string][]int{"me/many": many, "me/one": {300}, "me/none": {0, 0}, "me/secret": {9999}, "other/theirs": {9999}},
		fetched)
	defer srv.Close()
	useREST(t, srv.URL)

	got, err := fetchDownloads(context.Background(), srv.Client(), "token", "me")
	if err != nil {
		t.Fatal(err)
	}
	want := []RepoDownloads{{Name: "one", Count: 300}, {Name: "many", Count: 105}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("fetchDownloads = %v, want %v (repositories with no downloads are left out)", got, want)
	}
	if fetched["me/many"] != 2 || fetched["me/secret"] != 0 || fetched["other/theirs"] != 0 {
		t.Fatalf("release requests = %v, want two pages for me/many and none for private or other owners' repositories", fetched)
	}
}

func TestFetchDownloadsBreaksTiesByName(t *testing.T) {
	repo := func(name string) map[string]any {
		return map[string]any{"name": name, "owner": map[string]string{"login": "me"}}
	}
	srv := releasesServer(t, []map[string]any{repo("b"), repo("A"), repo("c")}, map[string][]int{"me/b": {7}, "me/A": {7}, "me/c": {8}}, map[string]int{})
	defer srv.Close()
	useREST(t, srv.URL)

	got, err := fetchDownloads(context.Background(), srv.Client(), "token", "me")
	if err != nil {
		t.Fatal(err)
	}
	if want := "[{c 8} {A 7} {b 7}]"; fmt.Sprint(got) != want {
		t.Fatalf("fetchDownloads = %v, want %s", got, want)
	}
}

func TestFetchDownloadsFailsInsteadOfReturningAPartialRanking(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/users/"):
			_, _ = fmt.Fprint(w, `[{"name":"ok","owner":{"login":"me"}},{"name":"denied","owner":{"login":"me"}}]`)
		case strings.Contains(r.URL.Path, "/denied/"):
			w.WriteHeader(http.StatusForbidden)
			_, _ = fmt.Fprint(w, `{"message":"Resource not accessible"}`)
		default:
			_, _ = fmt.Fprint(w, `[{"assets":[{"download_count":3}]}]`)
		}
	}))
	defer srv.Close()
	useREST(t, srv.URL)

	got, err := fetchDownloads(context.Background(), srv.Client(), "token", "me")
	if !errors.Is(err, errStarHistoryAccess) || got != nil || !strings.Contains(err.Error(), "release downloads of me/denied") {
		t.Fatalf("got=%v error=%v", got, err)
	}
}

func TestFetchDownloadsRejectsANegativeCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/users/") {
			_, _ = fmt.Fprint(w, `[{"name":"odd","owner":{"login":"me"}}]`)
			return
		}
		_, _ = fmt.Fprint(w, `[{"assets":[{"download_count":-1}]}]`)
	}))
	defer srv.Close()
	useREST(t, srv.URL)

	if _, err := fetchDownloads(context.Background(), srv.Client(), "token", "me"); err == nil || !strings.Contains(err.Error(), "negative") {
		t.Fatalf("error = %v, want a negative count error", err)
	}
}

func TestRenderDownloadGraph(t *testing.T) {
	t.Parallel()

	rows := []RepoDownloads{{Name: "gup", Count: 69920}, {Name: "a<b", Count: 12}}
	svg := renderDownloadGraph(rows, "me", themes[0])
	for _, want := range []string{
		"<title>Release downloads: gup 69,920, a&lt;b 12</title>",
		">Release downloads<",
		"public repositories owned by me",
		"go install",
		">69,920<",
		">a&lt;b<",
	} {
		if !strings.Contains(svg, want) {
			t.Errorf("graph lacks %q:\n%s", want, svg)
		}
	}
	if n := strings.Count(svg, `class="bar"`); n != 2 {
		t.Errorf("graph has %d bars, want 2", n)
	}
	one := renderDownloadGraph(rows[:1], "me", themes[0])
	if !strings.Contains(svg, `height="`) || graphHeightOf(t, one) >= graphHeightOf(t, svg) {
		t.Error("a graph with more bars is not taller")
	}
}

func TestRenderDownloadGraphWithoutDownloads(t *testing.T) {
	t.Parallel()

	svg := renderDownloadGraph(nil, "me", themes[1])
	if !strings.Contains(svg, "No release downloads yet") || strings.Contains(svg, `class="bar"`) {
		t.Fatalf("empty graph:\n%s", svg)
	}
}

func graphHeightOf(t *testing.T, svg string) int {
	t.Helper()
	var w, h int
	if _, err := fmt.Sscanf(svg, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d"`, &w, &h); err != nil {
		t.Fatalf("no size in %q: %v", svg[:min(len(svg), 120)], err)
	}
	return h
}

func TestDownloadBarWidth(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		count, most int
		want        float64
	}{
		{count: 100, most: 100, want: 400},
		{count: 50, most: 100, want: 200},
		{count: 1, most: 1000000, want: 2}, // never thinner than 2px, so a bar stays visible
		{count: 0, most: 100, want: 0},
	} {
		if got := downloadBarWidth(tc.count, tc.most, 400); got != tc.want {
			t.Errorf("downloadBarWidth(%d, %d, 400) = %v, want %v", tc.count, tc.most, got, tc.want)
		}
	}
}

func TestRunDrawsTheTopDownloadsBelowTheCardsAndRemovesThemWhenDropped(t *testing.T) {
	config := `
downloads:
  top: 2
  position: bottom
sections:
  - groups:
      - title: Tools
        repos:
          - name: card
`
	fake := &fakeGitHub{
		stars:       map[string]int{"me/card": 1},
		publicRepos: []string{"me/card", "me/big", "me/mid", "me/small"},
		downloads:   map[string][]int{"me/big": {900, 100}, "me/mid": {500}, "me/small": {3}},
	}
	dir := setupRun(t, config, fake)
	out, err := runIn(dir)
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	readme, err := os.ReadFile(filepath.Join(dir, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := string(readme)
	card, graph := strings.Index(doc, "me__card.svg"), strings.Index(doc, `srcset="cards/downloads-dark.svg"`)
	if card < 0 || graph < 0 || graph < card {
		t.Fatalf("downloads graph is missing or above the cards:\n%s", doc)
	}
	for _, theme := range themes {
		data, err := os.ReadFile(filepath.Join(dir, "cards", downloadsFile(theme.Name)))
		if err != nil {
			t.Fatal(err)
		}
		svg := string(data)
		if !strings.Contains(svg, "Release downloads: big 1,000, mid 500</title>") || strings.Contains(svg, ">small<") {
			t.Fatalf("%s graph does not show exactly the top 2:\n%s", theme.Name, svg)
		}
	}

	if err := os.WriteFile(filepath.Join(dir, "showcase.yml"), []byte(strings.Replace(config, "downloads:\n  top: 2\n  position: bottom\n", "", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := runIn(dir); err != nil {
		t.Fatalf("second run: %v\n%s", err, out)
	}
	for _, theme := range themes {
		if _, err := os.Stat(filepath.Join(dir, "cards", downloadsFile(theme.Name))); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s graph was not removed: %v", theme.Name, err)
		}
	}
	readme, _ = os.ReadFile(filepath.Join(dir, "README.md"))
	if strings.Contains(string(readme), "downloads-") {
		t.Fatal("README still shows the downloads graph")
	}
}

func TestRunPutsTheDownloadsGraphAfterTheStarHistoryWhenBothAreOnTop(t *testing.T) {
	config := `
starHistory:
  repos: [card]
downloads: {}
sections:
  - groups:
      - title: Tools
        repos:
          - name: card
`
	fake := &fakeGitHub{stars: map[string]int{"me/card": 2}, publicRepos: []string{"me/card"}, downloads: map[string][]int{"me/card": {4}}}
	dir := setupRun(t, config, fake)
	if out, err := runIn(dir); err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	readme, err := os.ReadFile(filepath.Join(dir, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := string(readme)
	stars, downloads, card := strings.Index(doc, "star-history-dark.svg"), strings.Index(doc, "downloads-dark.svg"), strings.Index(doc, "me__card.svg")
	if stars < 0 || downloads < 0 || card < 0 || stars > downloads || downloads > card {
		t.Fatalf("want star history, then downloads, then cards:\n%s", doc)
	}
}
