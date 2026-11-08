// Command readme-showcase draws a card for each repository listed in a config
// file, optionally a star history graph, and rewrites the part of a README
// between the showcase markers to show them.
//
// It is the program behind the nao1215/action-readme-showcase GitHub Action,
// and runs locally too:
//
//	GITHUB_TOKEN=... readme-showcase -config .github/readme-showcase.yml
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// version is set by the release build.
var version = "dev"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "readme-showcase:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("readme-showcase", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", ".github/readme-showcase.yml", "config file listing the repositories")
	readmePath := fs.String("readme", "README.md", "README to rewrite")
	cardsDir := fs.String("cards", "cards", "directory for the generated SVG files, relative to the README")
	showVersion := fs.Bool("version", false, "print the version and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		fmt.Fprintln(stdout, version)
		return nil
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	if token == "" {
		return errors.New("set GITHUB_TOKEN or GH_TOKEN")
	}

	cfg, err := loadConfig(*configPath, os.Getenv("GITHUB_REPOSITORY_OWNER"))
	if err != nil {
		return err
	}
	if strings.HasPrefix(filepath.ToSlash(*cardsDir), "/") || strings.Contains(filepath.ToSlash(*cardsDir), "..") {
		return fmt.Errorf("cards directory %q must stay inside the README's directory", *cardsDir)
	}

	// GitHub Actions sets these; on GitHub Enterprise Server they point at
	// the server's own API.
	if v := os.Getenv("GITHUB_API_URL"); v != "" {
		restEndpoint = strings.TrimSuffix(v, "/")
	}
	if v := os.Getenv("GITHUB_GRAPHQL_URL"); v != "" {
		graphqlEndpoint = v
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	client := &http.Client{Timeout: 30 * time.Second}
	infos, err := fetchRepos(ctx, client, token, cfg.reposToFetch(), cfg.Owner)
	if err != nil {
		return err
	}

	readmeDir := filepath.Dir(*readmePath)
	outDir := filepath.Join(readmeDir, filepath.FromSlash(*cardsDir))
	w := &writer{}
	if err := writeCards(w, cfg, infos, outDir); err != nil {
		return err
	}

	if cfg.StarHistory != nil {
		series, err := fetchStarSeries(ctx, client, token, cfg, infos)
		if err != nil {
			return err
		}
		if len(series) > maxStarSeries {
			fmt.Fprintf(stderr, "readme-showcase: warning: the star history shows only the first %d of %d repositories\n", maxStarSeries, len(series))
		}
		now := time.Now().UTC()
		for _, t := range themes {
			if err := w.write(filepath.Join(outDir, starGraphFile(t.Name)), []byte(renderStarGraph(series, now, t))); err != nil {
				return err
			}
		}
	} else {
		for _, t := range themes {
			if err := w.remove(filepath.Join(outDir, starGraphFile(t.Name))); err != nil {
				return err
			}
		}
	}

	doc, err := os.ReadFile(*readmePath)
	if err != nil {
		return err
	}
	body := renderShowcase(cfg, infos, filepath.ToSlash(*cardsDir), cfg.StarHistory != nil)
	updated, err := replaceBetweenMarkers(string(doc), body)
	if err != nil {
		return fmt.Errorf("%s: %w", *readmePath, err)
	}
	if err := w.write(*readmePath, []byte(updated)); err != nil {
		return err
	}
	for _, r := range cfg.Repos() {
		if infos[r.Name].Archived {
			fmt.Fprintf(stderr, "readme-showcase: warning: %s is archived\n", r.Name)
		}
	}

	fmt.Fprintf(stdout, "readme-showcase: %d cards, changed=%t\n", len(cfg.Repos()), w.changed)
	return setOutput("changed", fmt.Sprint(w.changed))
}

// writer writes files only when their content differs and remembers whether
// anything on disk changed.
type writer struct {
	changed bool
}

func (w *writer) write(path string, data []byte) error {
	if old, err := os.ReadFile(path); err == nil && string(old) == string(data) {
		return nil
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	w.changed = true
	return nil
}

func (w *writer) remove(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err == nil {
		w.changed = true
	}
	return err
}

// writeCards writes every card in every theme and removes cards of
// repositories that are no longer listed. Only files named like a card
// (owner__name.svg) are removed, so other images that share the directory
// are left alone.
func writeCards(w *writer, cfg *Config, infos map[string]RepoInfo, outDir string) error {
	for _, t := range themes {
		dir := filepath.Join(outDir, t.Name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		keep := map[string]bool{}
		for _, r := range cfg.Repos() {
			info := infos[r.Name]
			file := cardFile(info.Owner, info.Name)
			keep[file] = true
			svg := renderCard(newCardData(r, info, cfg.Owner), t)
			if err := w.write(filepath.Join(dir, file), []byte(svg)); err != nil {
				return err
			}
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if !e.IsDir() && isCardFile(e.Name()) && !keep[e.Name()] {
				if err := w.remove(filepath.Join(dir, e.Name())); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// isCardFile reports whether a file name has the shape cardFile gives.
func isCardFile(name string) bool {
	base, ok := strings.CutSuffix(name, ".svg")
	if !ok {
		return false
	}
	owner, repo, ok := strings.Cut(base, "__")
	return ok && owner != "" && repo != ""
}

// setOutput writes a step output when running inside GitHub Actions.
func setOutput(name, value string) error {
	path := os.Getenv("GITHUB_OUTPUT")
	if path == "" {
		return nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%s=%s\n", name, value)
	return errors.Join(err, f.Close())
}
