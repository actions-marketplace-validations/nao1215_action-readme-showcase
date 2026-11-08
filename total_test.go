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
	"time"
)

func TestPublicTotalPaginationAndScope(t *testing.T) {
	base := time.Date(2024, 1, 7, 0, 0, 0, 0, time.UTC)
	pages, histories := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/users/ME/repos" {
			pages++
			if r.URL.Query().Get("type") != "owner" || r.URL.Query().Get("per_page") != "100" {
				t.Error("unexpected repository listing parameters")
			}
			repo := func(owner, name string, private bool) map[string]any {
				return map[string]any{"name": name, "owner": map[string]string{"login": owner}, "private": private, "fork": true, "archived": true}
			}
			if r.URL.Query().Get("page") == "1" {
				var repos []map[string]any
				for i := range 100 {
					repos = append(repos, repo("me", fmt.Sprintf("r%d", i), false))
				}
				_ = json.NewEncoder(w).Encode(repos)
			} else {
				_ = json.NewEncoder(w).Encode([]map[string]any{repo("me", "last", false), repo("me", "private", true), repo("other", "external", false), repo("ME", "R0", false)})
			}
			return
		}
		histories++
		if strings.Contains(r.URL.Path, "private") || strings.Contains(r.URL.Path, "external") {
			t.Error("total fetched an excluded repository")
		}
		week := base.AddDate(0, 0, 7)
		if strings.Contains(r.URL.Path, "/last/") {
			week = base
		}
		_, _ = fmt.Fprintf(w, `[{"week":%d,"days":[1,0,0,0,0,0,0]}]`, week.Unix())
	}))
	defer srv.Close()
	old := restEndpoint
	restEndpoint = srv.URL
	t.Cleanup(func() { restEndpoint = old })
	cfg := &Config{Owner: "ME", StarHistory: &StarHistory{Total: true, Repos: []string{"r0"}}}
	series, err := fetchStarSeries(context.Background(), srv.Client(), "test-token", cfg, map[string]RepoInfo{"r0": {Owner: "me", Name: "r0"}})
	if err != nil {
		t.Fatal(err)
	}
	if pages != 2 || histories != 101 || len(series) != 2 || len(series[0].Dates) != 101 || !series[0].Dates[0].Equal(base) {
		t.Fatalf("pages=%d histories=%d series=%+v", pages, histories, series)
	}
}

func TestPublicTotalFailsInsteadOfReturningPartialHistory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/repos") {
			_, _ = fmt.Fprint(w, `[{"name":"ok","owner":{"login":"me"}},{"name":"denied","owner":{"login":"me"}}]`)
		} else if strings.Contains(r.URL.Path, "/denied/") {
			w.WriteHeader(http.StatusForbidden)
			_, _ = fmt.Fprint(w, `{"message":"Resource not accessible"}`)
		} else {
			_, _ = fmt.Fprint(w, `[]`)
		}
	}))
	defer srv.Close()
	old := restEndpoint
	restEndpoint = srv.URL
	t.Cleanup(func() { restEndpoint = old })
	cfg := &Config{Owner: "me", StarHistory: &StarHistory{Total: true}}
	series, err := fetchStarSeries(context.Background(), srv.Client(), "token", cfg, nil)
	if !errors.Is(err, errStarHistoryAccess) || series != nil || !strings.Contains(err.Error(), "public total for me") {
		t.Fatalf("series=%v error=%v", series, err)
	}
}

func TestRunTotalIncludesRepositoriesBeyondDisplayLimit(t *testing.T) {
	config := `
starHistory:
  repos: [r1, other/external, r2, r3, r4, r5, r6, empty, private]
  total: true
sections:
  - groups:
      - title: Tools
        repos:
          - name: card-only
`
	fake := &fakeGitHub{stars: map[string]int{"me/card-only": 100, "me/unlisted": 7, "me/empty": 0, "me/private": 40, "other/external": 50}, publicRepos: []string{"me/card-only", "me/unlisted", "me/empty"}}
	for i := 1; i <= 6; i++ {
		fake.stars[fmt.Sprintf("me/r%d", i)] = i
		fake.publicRepos = append(fake.publicRepos, fmt.Sprintf("me/r%d", i))
	}
	dir := setupRun(t, config, fake)
	out, err := runIn(dir)
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if !strings.Contains(out, "first 5 of 9 repositories") {
		t.Fatalf("missing display-limit warning: %s", out)
	}
	if fake.starRequests != 11 {
		t.Fatalf("history requests = %d, want 11", fake.starRequests)
	}
	for _, theme := range themes {
		data, err := os.ReadFile(filepath.Join(dir, "cards", starGraphFile(theme.Name)))
		if err != nil {
			t.Fatal(err)
		}
		svg := string(data)
		if !strings.Contains(svg, "all public repositories owned by me, including forks and archived repositories") {
			t.Fatal("graph omits total scope")
		}
		if !strings.Contains(svg, "Star history: total (all public) 128, r1 1, other/external 50, r2 2, r3 3, r4 4") || strings.Contains(svg, "r6") || strings.Contains(svg, "card-only") {
			t.Fatalf("unexpected total or displayed repositories: %s", svg)
		}
	}
}
