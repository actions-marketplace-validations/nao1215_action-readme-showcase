package main

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// RepoDownloads is one bar of the release downloads graph.
type RepoDownloads struct {
	Name  string
	Count int
}

// fetchDownloads sums the download counts of every asset of every release
// of each public repository the owner owns, the same set the star history
// total uses. Repositories without downloads are left out. The result is
// sorted by count, largest first, with ties in case-insensitive name order.
// A failed request fails the whole ranking instead of leaving a repository
// out of it.
func fetchDownloads(ctx context.Context, client *http.Client, token, owner string) ([]RepoDownloads, error) {
	repos, err := fetchPublicRepos(ctx, client, token, owner)
	if err != nil {
		return nil, fmt.Errorf("release downloads for %s: %w", owner, err)
	}
	var out []RepoDownloads
	for _, repo := range repos {
		n, err := fetchRepoDownloads(ctx, client, token, repo.Owner, repo.Name)
		if err != nil {
			return nil, fmt.Errorf("release downloads of %s/%s: %w", repo.Owner, repo.Name, err)
		}
		if n > 0 {
			out = append(out, RepoDownloads{Name: repo.Name, Count: n})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// fetchRepoDownloads pages through a repository's releases, 100 at a time,
// and adds up download_count over their assets.
func fetchRepoDownloads(ctx context.Context, client *http.Client, token, owner, name string) (int, error) {
	total := 0
	for page := 1; page <= 100; page++ {
		endpoint := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=100&page=%d", restEndpoint, url.PathEscape(owner), url.PathEscape(name), page)
		var releases []struct {
			Assets []struct {
				DownloadCount int `json:"download_count"`
			} `json:"assets"`
		}
		if err := getWithRetry(ctx, client, token, endpoint, &releases); err != nil {
			return 0, err
		}
		for _, release := range releases {
			for _, asset := range release.Assets {
				if asset.DownloadCount < 0 {
					return 0, errors.New("a release asset has a negative download count")
				}
				total += asset.DownloadCount
			}
		}
		if len(releases) < 100 {
			return total, nil
		}
	}
	return 0, errors.New("releases exceed 100 pages")
}

// downloadsFile is the file name of the release downloads graph for a theme.
func downloadsFile(theme string) string {
	return "downloads-" + theme + ".svg"
}

const (
	downloadsTop    = 76 // y of the first bar row
	downloadsRow    = 26 // distance between bar rows
	downloadsBar    = 14 // bar thickness
	downloadsLabel  = 168
	downloadsValueW = 72 // room right of the longest bar for its count
	// iconDownload is the Octicons download icon (MIT License), 16px.
	iconDownload = "M2.75 14A1.75 1.75 0 0 1 1 12.25v-2.5a.75.75 0 0 1 1.5 0v2.5c0 .138.112.25.25.25h10.5a.25.25 0 0 0 .25-.25v-2.5a.75.75 0 0 1 1.5 0v2.5A1.75 1.75 0 0 1 13.25 14ZM7.25 7.689V2a.75.75 0 0 1 1.5 0v5.689l1.97-1.969a.749.749 0 1 1 1.06 1.06l-3.25 3.25a.749.749 0 0 1-1.06 0L4.22 6.78a.749.749 0 1 1 1.06-1.06l1.97 1.969Z"
)

// downloadBarWidth scales count against the largest count to span pixels.
// A nonzero count is at least 2px wide so that it stays visible next to a
// much larger one.
func downloadBarWidth(count, most int, span float64) float64 {
	if count <= 0 || most <= 0 {
		return 0
	}
	return max(2, float64(count)/float64(most)*span)
}

// renderDownloadGraph draws rows as a horizontal bar chart, in the order
// given, as a standalone SVG document. owner names the scope in the
// subtitle.
func renderDownloadGraph(rows []RepoDownloads, owner string, t Theme) string {
	var b strings.Builder
	esc := html.EscapeString
	height := downloadsTop + len(rows)*downloadsRow + 8
	if len(rows) == 0 {
		height = 120
	}
	names := make([]string, len(rows))
	for i, r := range rows {
		names[i] = fmt.Sprintf("%s %s", r.Name, grouped(r.Count))
	}
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-label="Release downloads">`+"\n",
		graphWidth, height, graphWidth, height)
	fmt.Fprintf(&b, "<title>Release downloads: %s</title>\n", esc(strings.Join(names, ", ")))
	fmt.Fprintf(&b, `<style>text{font-family:%s}</style>`+"\n", fontStack)
	fmt.Fprintf(&b, `<rect x="0.75" y="0.75" width="%s" height="%s" rx="8" fill="%s" stroke="%s"/>`+"\n",
		num(graphWidth-1.5), num(float64(height)-1.5), t.Bg, t.Border)
	b.WriteString(icon(iconDownload, padX, 17, 1, t.Muted))
	fmt.Fprintf(&b, `<text x="%d" y="30" font-size="%s" font-weight="600" fill="%s">Release downloads</text>`+"\n", padX+22, num(titleSize), t.Text)
	scope := fmt.Sprintf("GitHub release assets of public repositories owned by %s. go install and package managers are not counted.", owner)
	fmt.Fprintf(&b, `<text x="%d" y="52" font-size="11" fill="%s"><title>%s</title>%s</text>`+"\n",
		padX, t.Muted, esc(scope), esc(truncate(scope, 11, graphWidth-2*padX)))

	if len(rows) == 0 {
		fmt.Fprintf(&b, `<text x="%d" y="92" font-size="%s" text-anchor="middle" fill="%s">No release downloads yet</text>`+"\n",
			graphWidth/2, num(metaSize), t.Muted)
		b.WriteString("</svg>\n")
		return b.String()
	}

	barLeft := float64(padX + downloadsLabel)
	span := float64(graphWidth-padX-downloadsValueW) - barLeft
	most := rows[0].Count
	for _, r := range rows {
		most = max(most, r.Count)
	}
	for i, r := range rows {
		y := float64(downloadsTop + i*downloadsRow)
		fmt.Fprintf(&b, `<text x="%d" y="%s" font-size="%s" fill="%s">%s</text>`+"\n",
			padX, num(y+11), num(metaSize), t.Text, esc(truncate(r.Name, metaSize, downloadsLabel-12)))
		w := downloadBarWidth(r.Count, most, span)
		fmt.Fprintf(&b, `<rect class="bar" x="%s" y="%s" width="%s" height="%d" rx="3" fill="%s"/>`+"\n",
			num(barLeft), num(y), num(w), downloadsBar, t.Series[0])
		fmt.Fprintf(&b, `<text x="%s" y="%s" font-size="%s" fill="%s">%s</text>`+"\n",
			num(barLeft+w+6), num(y+11), num(metaSize), t.Muted, grouped(r.Count))
	}
	b.WriteString("</svg>\n")
	return b.String()
}
