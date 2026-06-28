// Package version carries the application version, injected at build time.
package version

// These are overwritten at link time by GoReleaser via -ldflags -X (see
// .goreleaser.yaml), which sources the value from the git tag. They default to
// "dev" for local and unreleased builds.
var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)
