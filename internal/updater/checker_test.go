package updater

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestSelectAppDownloadAsset(t *testing.T) {
	assets := []githubAsset{
		{Name: "Dotward.app.zip", URL: "https://example.com/generic"},
		{Name: "dotward-darwin-amd64.app.zip", URL: "https://example.com/amd64"},
		{Name: "dotward-darwin-arm64.app.zip", URL: "https://example.com/arm64"},
	}

	got := selectAppDownloadAsset(assets, "arm64")
	if got != "https://example.com/arm64" {
		t.Fatalf("asset mismatch got=%q want=%q", got, "https://example.com/arm64")
	}
}

func TestSelectAppDownloadAssetAmd64Alias(t *testing.T) {
	assets := []githubAsset{
		{Name: "dotward-darwin-x86_64.app.zip", URL: "https://example.com/x86_64"},
	}

	got := selectAppDownloadAsset(assets, "amd64")
	if got != "https://example.com/x86_64" {
		t.Fatalf("asset mismatch got=%q want=%q", got, "https://example.com/x86_64")
	}
}

func TestSelectCLIDownloadAsset(t *testing.T) {
	assets := []githubAsset{
		{Name: "dotward-darwin-amd64.app.zip", URL: "https://example.com/app"},
		{Name: "dotward-darwin-arm64", URL: "https://example.com/cli"},
	}

	got := selectCLIDownloadAsset(assets, "arm64")
	if got != "https://example.com/cli" {
		t.Fatalf("asset mismatch got=%q want=%q", got, "https://example.com/cli")
	}
}

func TestPreferenceStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update-preferences.json")
	store, err := LoadPreferenceStore(path)
	if err != nil {
		t.Fatalf("load prefs: %v", err)
	}

	if err := store.SetSkippedVersion("v1.2.3"); err != nil {
		t.Fatalf("set skipped version: %v", err)
	}

	store2, err := LoadPreferenceStore(path)
	if err != nil {
		t.Fatalf("reload prefs: %v", err)
	}
	if got := store2.SkippedVersion(); got != "v1.2.3" {
		t.Fatalf("skipped version mismatch got=%q want=%q", got, "v1.2.3")
	}
}

func TestCheckIgnoresSameVersionPublishedAfterBuild(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{
			"tag_name":"v1.0.9",
			"published_at":"2026-04-30T16:00:00Z",
			"assets":[
				{"name":"Dotward_v1.0.9_darwin_arm64.app.zip","browser_download_url":"https://example.com/app.zip"},
				{"name":"dotward_v1.0.9_darwin_arm64","browser_download_url":"https://example.com/dotward"}
			]
		}]`)
	}))
	defer srv.Close()

	checker := NewChecker()
	checker.releasesURL = srv.URL

	_, ok, err := checker.Check(context.Background(), "v1.0.9", "2026-04-30T15:55:00Z")
	if err != nil {
		t.Fatalf("check update: %v", err)
	}
	if ok {
		t.Fatal("same version should not be reported as an update")
	}
}

func TestCheckReportsNewerVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{
			"tag_name":"v1.0.10",
			"published_at":"2026-04-30T16:00:00Z",
			"assets":[
				{"name":"Dotward_v1.0.10_darwin_arm64.app.zip","browser_download_url":"https://example.com/app.zip"},
				{"name":"dotward_v1.0.10_darwin_arm64","browser_download_url":"https://example.com/dotward"}
			]
		}]`)
	}))
	defer srv.Close()

	checker := NewChecker()
	checker.releasesURL = srv.URL

	release, ok, err := checker.Check(context.Background(), "v1.0.9", "2026-04-30T15:55:00Z")
	if err != nil {
		t.Fatalf("check update: %v", err)
	}
	if !ok {
		t.Fatal("newer version should be reported as an update")
	}
	if release.TagName != "v1.0.10" {
		t.Fatalf("release tag mismatch got=%q want=%q", release.TagName, "v1.0.10")
	}
}
