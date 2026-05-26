// Package version exposes the single source of truth for the go-eventkit
// build version. The value is overridden at build time via
//
//	-ldflags "-X github.com/dillonbrowne/go-eventkit/internal/version.Version=v0.1.0"
//
// All cmd/ binaries and the server package read from here so the
// OpenAPI metadata, the audit log, the .app bundle's Info.plist, and
// any other version surface report one identifier.
package version

// Version is the build identifier. Default "dev" is what unreleased
// developer builds show; release tags inject the actual version via
// ldflags (see .goreleaser.yml and Formula/eventkit-server.rb).
var Version = "dev"
