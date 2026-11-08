package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config lists the repositories shown in the README showcase.
type Config struct {
	// Owner is the default owner for repositories written without "owner/".
	// When empty, the owner of the repository running the action is used.
	Owner string `yaml:"owner"`
	// StarHistory turns on the star history graph. It is nil when the config
	// has no starHistory key.
	StarHistory *StarHistory `yaml:"starHistory"`
	Sections    []Section    `yaml:"sections"`
}

// StarHistory selects graph repositories independently of the cards.
type StarHistory struct {
	// Repos lists names or owner/name pairs in graph order.
	Repos []string `yaml:"repos"`
	// Total adds the combined history of all selected repositories.
	Total bool `yaml:"total"`
}

// Section is a run of groups. A section with a Summary is folded into a
// <details> block; a section without one is shown as is.
type Section struct {
	Summary string  `yaml:"summary"`
	Groups  []Group `yaml:"groups"`
}

// Group is one heading followed by its cards.
type Group struct {
	Title string     `yaml:"title"`
	Repos []RepoSpec `yaml:"repos"`
}

// RepoSpec is one card. Description and Language override what GitHub
// reports, for example when a game built with a YAML engine is detected as
// Python because of its helper scripts.
type RepoSpec struct {
	Name        string    `yaml:"name"`
	Description string    `yaml:"description"`
	Language    *Language `yaml:"language"`
	Featured    bool      `yaml:"featured"`
}

// Language is the label and dot color shown in a card's footer.
type Language struct {
	Name  string `yaml:"name"`
	Color string `yaml:"color"`
}

// OwnerAndName splits Name into owner and repository, using defaultOwner when
// Name has no "owner/" prefix.
func (r RepoSpec) OwnerAndName(defaultOwner string) (string, string) {
	if owner, name, ok := strings.Cut(r.Name, "/"); ok {
		return owner, name
	}
	return defaultOwner, r.Name
}

// loadConfig reads a YAML (or JSON) config. defaultOwner fills in Owner when
// the file leaves it out.
func loadConfig(path, defaultOwner string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg, err := parseConfig(data, defaultOwner)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

func parseConfig(data []byte, defaultOwner string) (*Config, error) {
	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if cfg.Owner == "" {
		cfg.Owner = defaultOwner
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	if c.Owner == "" {
		return errors.New("owner is empty; set owner in the config or run inside GitHub Actions")
	}
	if c.StarHistory != nil {
		if len(c.StarHistory.Repos) == 0 {
			return errors.New("starHistory.repos is empty; list graph repositories explicitly")
		}
		seen := map[string]bool{}
		for _, name := range c.StarHistory.Repos {
			r := RepoSpec{Name: name}
			owner, repo := r.OwnerAndName(c.Owner)
			if strings.TrimSpace(owner) == "" || strings.TrimSpace(repo) == "" || strings.Contains(repo, "/") {
				return fmt.Errorf("invalid starHistory repository name %q", name)
			}
			key := strings.ToLower(owner + "/" + repo)
			if seen[key] {
				return fmt.Errorf("starHistory repository %q is listed twice", name)
			}
			seen[key] = true
		}
	}
	if len(c.Repos()) == 0 {
		return errors.New("no repositories are listed")
	}
	seen := map[string]bool{}
	for _, s := range c.Sections {
		for _, g := range s.Groups {
			if g.Title == "" {
				return errors.New("a group has no title")
			}
			for _, r := range g.Repos {
				owner, name := r.OwnerAndName(c.Owner)
				if owner == "" || name == "" || strings.Contains(name, "/") {
					return fmt.Errorf("invalid repository name %q", r.Name)
				}
				if l := r.Language; l != nil && (l.Name == "" || !isHexColor(l.Color)) {
					return fmt.Errorf("%s: language needs a name and a #rrggbb color", r.Name)
				}
				key := strings.ToLower(owner + "/" + name)
				if seen[key] {
					return fmt.Errorf("repository %q is listed twice", r.Name)
				}
				seen[key] = true
			}
		}
	}
	return nil
}

// Repos returns every repository spec in display order.
func (c *Config) Repos() []RepoSpec {
	var out []RepoSpec
	for _, s := range c.Sections {
		for _, g := range s.Groups {
			out = append(out, g.Repos...)
		}
	}
	return out
}

// reposToFetch includes graph-only repositories without adding cards for them.
func (c *Config) reposToFetch() []RepoSpec {
	repos := c.Repos()
	seen := make(map[string]bool, len(repos))
	for _, r := range repos {
		seen[r.Name] = true
	}
	if c.StarHistory != nil {
		for _, name := range c.StarHistory.Repos {
			if !seen[name] {
				repos = append(repos, RepoSpec{Name: name})
				seen[name] = true
			}
		}
	}
	return repos
}

func isHexColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, c := range s[1:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}
