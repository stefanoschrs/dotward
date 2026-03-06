package version

import "fmt"

var (
	// Version is the semantic version (for example v1.2.3). Set via -ldflags.
	Version = "dev"
	// Commit is the VCS revision. Set via -ldflags.
	Commit = "unknown"
	// BuildTime is the UTC build timestamp (RFC3339). Set via -ldflags.
	BuildTime = "unknown"
	// BuildDate is the UTC build timestamp (RFC3339). Set via -ldflags.
	// Deprecated: use BuildTime.
	BuildDate = "unknown"
	// BuiltBy identifies the build system. Set via -ldflags.
	BuiltBy = "local"
)

// String returns a concise version string.
func String() string {
	return fmt.Sprintf("%s (%s)", Version, Commit)
}

// Detailed returns extended build metadata for a component.
func Detailed(component string) string {
	built := BuildTime
	if built == "unknown" {
		built = BuildDate
	}
	return fmt.Sprintf("%s %s\ncommit: %s\nbuilt: %s\nbuilder: %s", component, Version, Commit, built, BuiltBy)
}
