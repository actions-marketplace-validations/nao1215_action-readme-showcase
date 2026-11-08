package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// fetchStarSeries returns, for each explicitly selected repository in graph order,
// the daily star history, oldest first. GitHub supplies daily aggregate counts,
// so these dates do not represent individual stargazers or exact event times.
// When enabled, the owner-wide public total precedes the individual lines.
func fetchStarSeries(ctx context.Context, client *http.Client, token string, cfg *Config, infos map[string]RepoInfo) ([]StarSeries, error) {
	var out []StarSeries
	cache := map[string][]time.Time{}
	fetch := func(owner, name string) ([]time.Time, error) {
		key := strings.ToLower(owner + "/" + name)
		if dates, ok := cache[key]; ok {
			return dates, nil
		}
		dates, err := fetchRepoStarDates(ctx, client, token, owner, name)
		if err != nil {
			return nil, fmt.Errorf("stargazers of %s/%s: %w", owner, name, err)
		}
		sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })
		cache[key] = dates
		return dates, nil
	}
	if cfg.StarHistory.Total {
		repos, err := fetchPublicRepos(ctx, client, token, cfg.Owner)
		if err != nil {
			return nil, fmt.Errorf("public total for %s: %w", cfg.Owner, err)
		}
		total := StarSeries{
			Name:  "total (all public)",
			Scope: fmt.Sprintf("Total: all public repositories owned by %s, including forks and archived repositories", cfg.Owner),
		}
		for _, repo := range repos {
			dates, err := fetch(repo.Owner, repo.Name)
			if err != nil {
				return nil, fmt.Errorf("public total for %s: %w", cfg.Owner, err)
			}
			total.Dates = append(total.Dates, dates...)
		}
		sort.Slice(total.Dates, func(i, j int) bool { return total.Dates[i].Before(total.Dates[j]) })
		out = append(out, total)
	}
	for _, selected := range cfg.StarHistory.Repos {
		info := infos[selected]
		dates, err := fetch(info.Owner, info.Name)
		if err != nil {
			return nil, err
		}
		name := info.Name
		if info.Owner != cfg.Owner {
			name = info.Owner + "/" + info.Name
		}
		out = append(out, StarSeries{Name: name, Dates: dates})
	}
	return out, nil
}

// fetchPublicRepos lists public repositories owned by the account, including
// forks and archived repositories. The user endpoint also accepts organizations.
// Explicit filtering keeps the total public-only even with a privileged token.
func fetchPublicRepos(ctx context.Context, client *http.Client, token, owner string) ([]RepoInfo, error) {
	var out []RepoInfo
	seen := map[string]bool{}
	for page := 1; ; page++ {
		endpoint := fmt.Sprintf("%s/users/%s/repos?type=owner&sort=full_name&per_page=100&page=%d", restEndpoint, url.PathEscape(owner), page)
		var repos []struct {
			Name    string `json:"name"`
			Private bool   `json:"private"`
			Owner   struct {
				Login string `json:"login"`
			} `json:"owner"`
		}
		if err := getWithRetry(ctx, client, token, endpoint, &repos); err != nil {
			return nil, err
		}
		for _, repo := range repos {
			key := strings.ToLower(repo.Owner.Login + "/" + repo.Name)
			if repo.Private || !strings.EqualFold(repo.Owner.Login, owner) || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, RepoInfo{Owner: repo.Owner.Login, Name: repo.Name})
		}
		if len(repos) < 100 {
			return out, nil
		}
	}
}

// fetchRepoStarDates reads daily counts from GitHub's aggregate history API.
// Unlike the stargazers list, this endpoint needs only Metadata read access.
func fetchRepoStarDates(ctx context.Context, client *http.Client, token, owner, name string) ([]time.Time, error) {
	var out []time.Time
	for page := 1; page <= 100; page++ {
		url := fmt.Sprintf("%s/repos/%s/%s/stargazers/history?per_page=30&page=%d", restEndpoint, url.PathEscape(owner), url.PathEscape(name), page)
		var weeks []struct {
			Week int64 `json:"week"`
			Days []int `json:"days"`
		}
		if err := getWithRetry(ctx, client, token, url, &weeks); err != nil {
			return nil, err
		}
		for _, week := range weeks {
			if len(week.Days) != 7 {
				return nil, fmt.Errorf("star history week %d has %d days, want 7", week.Week, len(week.Days))
			}
			for day, count := range week.Days {
				if count < 0 {
					return nil, fmt.Errorf("star history week %d has a negative count", week.Week)
				}
				date := time.Unix(week.Week, 0).UTC().AddDate(0, 0, day)
				for range count {
					out = append(out, date)
				}
			}
		}
		if len(weeks) < 30 {
			return out, nil
		}
	}
	return nil, errors.New("star history exceeds the API's 100-page limit")
}

var errStarHistoryAccess = errors.New("star history needs a token with read access to the selected repositories; " +
	"check the token input and Metadata read permission")

func getWithRetry(ctx context.Context, client *http.Client, token, url string, out any) error {
	var lastErr error
	for attempt := range 3 {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * 2 * time.Second):
			}
		}
		lastErr = get(ctx, client, token, url, out)
		if lastErr == nil || errors.Is(lastErr, errStarHistoryAccess) {
			return lastErr
		}
	}
	return lastErr
}

func get(ctx context.Context, client *http.Client, token, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil) // #nosec G704 -- Host is the configured GitHub endpoint; API names are escaped path segments.
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "readme-showcase")

	res, err := client.Do(req) // #nosec G704 -- Requests use the configured GitHub endpoint, never URLs returned by the API.
	if err != nil {
		return err
	}
	defer res.Body.Close()

	data, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return err
	}
	if res.StatusCode == http.StatusForbidden && bytes.Contains(data, []byte("Resource not accessible")) {
		return errStarHistoryAccess
	}
	if res.StatusCode == http.StatusUnauthorized {
		return errStarHistoryAccess
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("github rest: %s: %s", res.Status, bytes.TrimSpace(data))
	}
	return json.Unmarshal(data, out)
}
