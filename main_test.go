package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// fakeGitHub answers the two API calls the program makes: the batched
// GraphQL repository query and the REST stargazers list.
type fakeGitHub struct {
	stars        map[string]int // owner/name -> stars
	refuseStars  bool           // answer stargazers like the Actions token does
	starRequests int
}

var aliasRE = regexp.MustCompile(`(r\d+): repository\(owner: "([^"]+)", name: "([^"]+)"\)`)

func (f *fakeGitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer test-token" {
		http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
		return
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/graphql":
		var req struct{ Query string }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		data := map[string]any{}
		for _, m := range aliasRE.FindAllStringSubmatch(req.Query, -1) {
			data[m[1]] = map[string]any{
				"name":            m[3],
				"owner":           map[string]any{"login": m[2]},
				"description":     "from github",
				"stargazerCount":  f.stars[m[2]+"/"+m[3]],
				"forkCount":       1,
				"isArchived":      false,
				"primaryLanguage": map[string]any{"name": "Go", "color": "#00ADD8"},
				"latestRelease":   map[string]any{"tagName": "v1.0.0"},
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/stargazers"):
		f.starRequests++
		if f.refuseStars {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"message":"Resource not accessible by integration"}`)
			return
		}
		repo := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/repos/"), "/stargazers")
		n := f.stars[repo]
		var page int
		_, _ = fmt.Sscan(r.URL.Query().Get("page"), &page)
		var out []map[string]string
		base := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)
		for i := (page - 1) * 100; i < n && i < page*100; i++ {
			out = append(out, map[string]string{"starred_at": base.Add(time.Duration(i) * 24 * time.Hour).Format(time.RFC3339)})
		}
		_ = json.NewEncoder(w).Encode(out)
	default:
		http.NotFound(w, r)
	}
}

// setupRun prepares a README, a config, and a fake API, and points the
// program at them. It cannot run in parallel: it sets environment variables
// and the endpoint variables.
func setupRun(t *testing.T, config string, fake *fakeGitHub) string {
	t.Helper()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	oldREST, oldGraphQL := restEndpoint, graphqlEndpoint
	t.Cleanup(func() { restEndpoint, graphqlEndpoint = oldREST, oldGraphQL })
	t.Setenv("GITHUB_API_URL", srv.URL)
	t.Setenv("GITHUB_GRAPHQL_URL", srv.URL+"/graphql")
	t.Setenv("GITHUB_TOKEN", "test-token")
	t.Setenv("GITHUB_REPOSITORY_OWNER", "me")
	t.Setenv("GITHUB_OUTPUT", "")

	dir := t.TempDir()
	readme := "# Hi\n\n" + markerStart + "\n" + markerEnd + "\n\nbye\n"
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "showcase.yml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func runIn(dir string, extra ...string) (string, error) {
	var out, errOut bytes.Buffer
	args := append([]string{"-config", filepath.Join(dir, "showcase.yml"), "-readme", filepath.Join(dir, "README.md")}, extra...)
	err := run(args, &out, &errOut)
	return out.String() + errOut.String(), err
}

const twoRepoConfig = `
starHistory:
  minStars: 150
sections:
  - groups:
      - title: Tools
        repos:
          - name: big
            featured: true
          - name: small
            featured: true
          - name: other/plain
`

func TestRunWritesCardsGraphAndReadme(t *testing.T) {
	fake := &fakeGitHub{stars: map[string]int{"me/big": 250, "me/small": 3, "other/plain": 7}}
	dir := setupRun(t, twoRepoConfig, fake)
	outputs := filepath.Join(dir, "github_output")
	if err := os.WriteFile(outputs, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_OUTPUT", outputs)

	if out, err := runIn(dir); err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}

	for _, f := range []string{"light/me__big.svg", "dark/me__small.svg", "light/other__plain.svg", "star-history-light.svg", "star-history-dark.svg"} {
		if _, err := os.Stat(filepath.Join(dir, "cards", f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
	graph, _ := os.ReadFile(filepath.Join(dir, "cards", "star-history-light.svg"))
	if !strings.Contains(string(graph), "big 250") || strings.Contains(string(graph), "small") {
		t.Errorf("graph should show only big (250 >= 150):\n%s", graph)
	}
	// Two pages of stargazers for big, none for the repos under minStars.
	if fake.starRequests != 3 {
		t.Errorf("stargazer requests = %d, want 3", fake.starRequests)
	}
	readme, _ := os.ReadFile(filepath.Join(dir, "README.md"))
	for _, want := range []string{"# Hi\n", "cards/star-history-dark.svg", `alt="me/big"`, `alt="other/plain"`, "bye\n"} {
		if !strings.Contains(string(readme), want) {
			t.Errorf("README lacks %q:\n%s", want, readme)
		}
	}
	if got, _ := os.ReadFile(outputs); string(got) != "changed=true\n" {
		t.Errorf("GITHUB_OUTPUT = %q", got)
	}

	// A second run with the same data changes nothing.
	if err := os.WriteFile(outputs, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := runIn(dir); err != nil {
		t.Fatalf("second run: %v\n%s", err, out)
	}
	if got, _ := os.ReadFile(outputs); string(got) != "changed=false\n" {
		t.Errorf("second run GITHUB_OUTPUT = %q", got)
	}
}

func TestRunRemovesStaleFiles(t *testing.T) {
	fake := &fakeGitHub{stars: map[string]int{"me/big": 250, "me/small": 3, "other/plain": 7}}
	dir := setupRun(t, twoRepoConfig, fake)
	if out, err := runIn(dir); err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}

	// An image of the user's own in the same directory must survive.
	own := filepath.Join(dir, "cards", "light", "logo.svg")
	if err := os.WriteFile(own, []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Drop starHistory and a repository.
	cfg := "sections:\n  - groups:\n      - title: Tools\n        repos:\n          - name: big\n"
	if err := os.WriteFile(filepath.Join(dir, "showcase.yml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := runIn(dir); err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	for _, f := range []string{"light/me__small.svg", "dark/other__plain.svg", "star-history-light.svg"} {
		if _, err := os.Stat(filepath.Join(dir, "cards", f)); !os.IsNotExist(err) {
			t.Errorf("%s was not removed: %v", f, err)
		}
	}
	if _, err := os.Stat(own); err != nil {
		t.Errorf("an image that is not a card was removed: %v", err)
	}
	readme, _ := os.ReadFile(filepath.Join(dir, "README.md"))
	if strings.Contains(string(readme), "star-history") {
		t.Errorf("README still shows the graph:\n%s", readme)
	}
}

func TestRunExplainsMissingPersonalToken(t *testing.T) {
	fake := &fakeGitHub{stars: map[string]int{"me/big": 250}, refuseStars: true}
	dir := setupRun(t, twoRepoConfig, fake)
	_, err := runIn(dir)
	if err == nil || !strings.Contains(err.Error(), "personal access token") {
		t.Fatalf("want a hint about the personal access token, got %v", err)
	}
	// The refusal is not retried.
	if fake.starRequests != 1 {
		t.Errorf("stargazer requests = %d, want 1", fake.starRequests)
	}
}

func TestRunRejectsBadInput(t *testing.T) {
	fake := &fakeGitHub{}
	dir := setupRun(t, twoRepoConfig, fake)

	if _, err := runIn(dir, "-cards", "../outside"); err == nil {
		t.Error("cards directory outside the README accepted")
	}
	if _, err := runIn(dir, "extra"); err == nil {
		t.Error("stray argument accepted")
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("no markers\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runIn(dir); err == nil || !strings.Contains(err.Error(), "markers") {
		t.Errorf("README without markers: %v", err)
	}
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	if _, err := runIn(dir); err == nil || !strings.Contains(err.Error(), "GITHUB_TOKEN") {
		t.Errorf("missing token: %v", err)
	}
}

func TestRunPrintsVersion(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"-version"}, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != version {
		t.Errorf("version output = %q", out.String())
	}
}
