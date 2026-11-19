package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// awesomeLists maps a lower-case owner/repo of an awesome list to its README.
type awesomeLists map[string]string

// fetchAwesomeLists reads the README of every awesome list named in the
// config, each once. A list that cannot be read fails the run, so a network
// error never silently takes a badge off a card.
func fetchAwesomeLists(ctx context.Context, client *http.Client, token string, cfg *Config) (awesomeLists, error) {
	lists := awesomeLists{}
	for _, r := range cfg.Repos() {
		if r.Awesome == "" {
			continue
		}
		key := strings.ToLower(r.Awesome)
		if _, ok := lists[key]; ok {
			continue
		}
		owner, name, _ := strings.Cut(r.Awesome, "/")
		endpoint := fmt.Sprintf("%s/repos/%s/%s/readme", restEndpoint, url.PathEscape(owner), url.PathEscape(name))
		var readme struct {
			Content  string `json:"content"`
			Encoding string `json:"encoding"`
		}
		if err := getWithRetry(ctx, client, token, endpoint, &readme); err != nil {
			if errors.Is(err, errStarHistoryAccess) {
				return nil, fmt.Errorf("awesome list %s: the token cannot read its README", r.Awesome)
			}
			return nil, fmt.Errorf("awesome list %s: %w", r.Awesome, err)
		}
		if readme.Encoding != "base64" {
			return nil, fmt.Errorf("awesome list %s: README has encoding %q, want base64", r.Awesome, readme.Encoding)
		}
		data, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, strings.NewReader(strings.ReplaceAll(readme.Content, "\n", ""))))
		if err != nil {
			return nil, fmt.Errorf("awesome list %s: decode README: %w", r.Awesome, err)
		}
		lists[key] = string(data)
	}
	return lists, nil
}

// listedIn reports whether the README of an awesome list links to the
// repository owner/name. The match ignores case, as GitHub URLs do, and
// requires the name to end there, so gup does not match gup-extra.
func listedIn(readme, owner, name string) bool {
	re := regexp.MustCompile(`(?i)github\.com/` + regexp.QuoteMeta(owner) + `/` + regexp.QuoteMeta(name) + `(?:[^A-Za-z0-9._-]|\.git|$)`)
	return re.MatchString(readme)
}
