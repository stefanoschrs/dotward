package updater

import (
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
