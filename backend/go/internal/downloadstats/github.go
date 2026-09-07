package downloadstats

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
)

var githubAssetPattern = regexp.MustCompile(`^aegisadmin_([^/_]+)_([^/_]+)\.deb$`)

type githubRelease struct {
	Assets []struct {
		Name          string `json:"name"`
		DownloadCount int64  `json:"download_count"`
	} `json:"assets"`
}

func AddGitHub(ctx context.Context, client *http.Client, apiURL, token string, stats *Stats) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "AegisAdmin-download-stats")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub API returned HTTP %d", response.StatusCode)
	}
	var releases []githubRelease
	if err := json.NewDecoder(response.Body).Decode(&releases); err != nil {
		return fmt.Errorf("decode GitHub response: %w", err)
	}
	counts := map[string]int64{}
	for _, release := range releases {
		for _, asset := range release.Assets {
			parts := githubAssetPattern.FindStringSubmatch(asset.Name)
			if len(parts) == 3 {
				counts[parts[1]+"\x00"+parts[2]] += asset.DownloadCount
				stats.TotalGitHubDownloads += asset.DownloadCount
			}
		}
	}
	index := map[string]int{}
	for position, release := range stats.Releases {
		index[release.Version+"\x00"+release.Architecture] = position
	}
	for key, count := range counts {
		if position, found := index[key]; found {
			stats.Releases[position].GitHubDownloads = count
			continue
		}
		parts := strings.SplitN(key, "\x00", 2)
		stats.Releases = append(stats.Releases, Release{Version: parts[0], Architecture: parts[1], GitHubDownloads: count})
	}
	sort.Slice(stats.Releases, func(i, j int) bool {
		if stats.Releases[i].Version == stats.Releases[j].Version {
			return stats.Releases[i].Architecture < stats.Releases[j].Architecture
		}
		return versionLess(stats.Releases[j].Version, stats.Releases[i].Version)
	})
	stats.GitHubAvailable = true
	return nil
}
