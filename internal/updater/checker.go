package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"
)

const defaultReleasesURL = "https://api.github.com/repos/stefanoschrs/dotward/releases"

// ReleaseInfo describes a newer release that can be installed.
type ReleaseInfo struct {
	TagName        string
	PublishedAt    time.Time
	AppDownloadURL string
	CLIDownloadURL string
}

// Checker fetches releases and determines whether an update is available.
type Checker struct {
	client      *http.Client
	releasesURL string
}

// NewChecker creates a release checker using the GitHub releases API.
func NewChecker() *Checker {
	return &Checker{
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
		releasesURL: defaultReleasesURL,
	}
}

type githubAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type githubRelease struct {
	TagName     string        `json:"tag_name"`
	PublishedAt time.Time     `json:"published_at"`
	Assets      []githubAsset `json:"assets"`
}

// Check determines whether a release newer than buildTime is available.
func (c *Checker) Check(ctx context.Context, buildTime string) (ReleaseInfo, bool, error) {
	if strings.TrimSpace(buildTime) == "" || buildTime == "unknown" {
		return ReleaseInfo{}, false, fmt.Errorf("build time is not set")
	}

	builtAt, err := time.Parse(time.RFC3339, buildTime)
	if err != nil {
		return ReleaseInfo{}, false, fmt.Errorf("invalid build time %q: %w", buildTime, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.releasesURL, nil)
	if err != nil {
		return ReleaseInfo{}, false, fmt.Errorf("failed to build releases request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "dotward-app-updater")

	resp, err := c.client.Do(req)
	if err != nil {
		return ReleaseInfo{}, false, fmt.Errorf("failed to fetch releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ReleaseInfo{}, false, fmt.Errorf("unexpected releases status: %s", resp.Status)
	}

	var releases []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return ReleaseInfo{}, false, fmt.Errorf("failed to decode releases response: %w", err)
	}
	if len(releases) == 0 {
		return ReleaseInfo{}, false, nil
	}

	latest := releases[0]
	for _, r := range releases[1:] {
		if r.PublishedAt.After(latest.PublishedAt) {
			latest = r
		}
	}
	if latest.TagName == "" || latest.PublishedAt.IsZero() {
		return ReleaseInfo{}, false, fmt.Errorf("latest release is missing required metadata")
	}
	if !latest.PublishedAt.After(builtAt) {
		return ReleaseInfo{}, false, nil
	}

	appURL := selectAppDownloadAsset(latest.Assets, runtime.GOARCH)
	if appURL == "" {
		return ReleaseInfo{}, false, fmt.Errorf("no suitable app asset found for %s", runtime.GOARCH)
	}
	cliURL := selectCLIDownloadAsset(latest.Assets, runtime.GOARCH)
	if cliURL == "" {
		return ReleaseInfo{}, false, fmt.Errorf("no suitable cli asset found for %s", runtime.GOARCH)
	}

	return ReleaseInfo{
		TagName:        latest.TagName,
		PublishedAt:    latest.PublishedAt,
		AppDownloadURL: appURL,
		CLIDownloadURL: cliURL,
	}, true, nil
}

func selectAppDownloadAsset(assets []githubAsset, arch string) string {
	bestScore := -1
	bestURL := ""
	for _, a := range assets {
		if a.URL == "" {
			continue
		}
		name := strings.ToLower(a.Name)
		if !strings.HasSuffix(name, ".app.zip") {
			continue
		}
		score := 0
		if strings.Contains(name, arch) {
			score += 4
		}
		if arch == "amd64" && (strings.Contains(name, "x86_64") || strings.Contains(name, "x64")) {
			score += 3
		}
		if strings.Contains(name, "darwin") || strings.Contains(name, "mac") || strings.Contains(name, "osx") {
			score += 2
		}
		if strings.Contains(name, "dotward") {
			score++
		}
		if score > bestScore {
			bestScore = score
			bestURL = a.URL
		}
	}
	return bestURL
}

func selectCLIDownloadAsset(assets []githubAsset, arch string) string {
	bestScore := -1
	bestURL := ""
	for _, a := range assets {
		if a.URL == "" {
			continue
		}
		name := strings.ToLower(a.Name)
		if strings.HasSuffix(name, ".app.zip") {
			continue
		}
		if strings.Contains(name, "sha256") || strings.Contains(name, "checksums") || strings.Contains(name, "sig") {
			continue
		}
		if !strings.Contains(name, "dotward") {
			continue
		}
		score := 0
		if strings.Contains(name, "cli") {
			score += 2
		}
		if strings.Contains(name, arch) {
			score += 4
		}
		if arch == "amd64" && (strings.Contains(name, "x86_64") || strings.Contains(name, "x64")) {
			score += 3
		}
		if strings.Contains(name, "darwin") || strings.Contains(name, "mac") || strings.Contains(name, "osx") {
			score += 2
		}
		extScore := 0
		if strings.HasSuffix(name, ".zip") {
			extScore = 1
		}
		if strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tgz") {
			extScore = 1
		}
		score += extScore
		if score > bestScore {
			bestScore = score
			bestURL = a.URL
		}
	}
	return bestURL
}
