package downloadstats

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestAddGitHubMergesReleaseAssets(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Accept") != "application/vnd.github+json" {
			t.Fatal("missing GitHub accept header")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`[{"assets":[{"name":"aegisadmin_0.2.84_amd64.deb","download_count":7},{"name":"checksums.txt","download_count":9}]},{"assets":[{"name":"aegisadmin_0.2.85_arm64.deb","download_count":3}]}]`)), Header: make(http.Header)}, nil
	})}
	stats := Stats{Releases: []Release{{Version: "0.2.84", Architecture: "amd64", APTDownloads: 11}}}
	if err := AddGitHub(context.Background(), client, "https://api.github.test/releases", "", &stats); err != nil {
		t.Fatal(err)
	}
	if !stats.GitHubAvailable || stats.TotalGitHubDownloads != 10 || len(stats.Releases) != 2 {
		t.Fatalf("unexpected GitHub totals: %+v", stats)
	}
	if stats.Releases[1].Version != "0.2.84" || stats.Releases[1].APTDownloads != 11 || stats.Releases[1].GitHubDownloads != 7 {
		t.Fatalf("merged release missing: %+v", stats.Releases)
	}
}

func TestAddGitHubRejectsHTTPFailure(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusForbidden, Body: io.NopCloser(strings.NewReader("rate limited")), Header: make(http.Header)}, nil
	})}
	stats := Stats{}
	if err := AddGitHub(context.Background(), client, "https://api.github.test/releases", "", &stats); err == nil || stats.GitHubAvailable {
		t.Fatalf("GitHub failure not preserved: %v %+v", err, stats)
	}
}
