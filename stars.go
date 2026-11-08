package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"
)

// fetchStarSeries returns, for each explicitly selected repository with at least
// cfg.StarHistory.MinStars stars, in graph configuration order,
// the daily star history, oldest first. GitHub supplies daily aggregate counts,
// so these dates do not represent individual stargazers or exact event times.
func fetchStarSeries(ctx context.Context, client *http.Client, token string, cfg *Config, infos map[string]RepoInfo) ([]StarSeries, error) {
	var out []StarSeries
	for _, selected := range cfg.StarHistory.Repos {
		info := infos[selected]
		if info.Stars < cfg.StarHistory.MinStars {
			continue
		}
		dates, err := fetchRepoStarDates(ctx, client, token, info.Owner, info.Name)
		if err != nil {
			return nil, fmt.Errorf("stargazers of %s/%s: %w", info.Owner, info.Name, err)
		}
		sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })
		name := info.Name
		if info.Owner != cfg.Owner {
			name = info.Owner + "/" + info.Name
		}
		out = append(out, StarSeries{Name: name, Dates: dates})
	}
	return out, nil
}

// fetchRepoStarDates reads daily counts from GitHub's aggregate history API.
// Unlike the stargazers list, this endpoint needs only Metadata read access.
func fetchRepoStarDates(ctx context.Context, client *http.Client, token, owner, name string) ([]time.Time, error) {
	var out []time.Time
	for page := 1; page <= 100; page++ {
		url := fmt.Sprintf("%s/repos/%s/%s/stargazers/history?per_page=30&page=%d", restEndpoint, owner, name, page)
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "readme-showcase")

	res, err := client.Do(req)
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
