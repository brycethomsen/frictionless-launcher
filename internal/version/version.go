// Package version reports which build of the app is running, for the log.
// It has no dependencies, so every process (the tray included) can use it.
package version

import "runtime/debug"

// version is set at build time by the release build (see .goreleaser.yaml:
// -X frictionless-launcher/internal/version.version=...). Empty otherwise.
var version string

// String returns the release version (e.g. "1.1.0") for release builds.
// Other builds report "dev", plus the git commit they were built from when
// the Go toolchain recorded one (go build in a checkout does; go run and
// go test don't), marked "-dirty" if the checkout had uncommitted changes.
func String() string {
	if version != "" {
		return version
	}
	return devVersion(debug.ReadBuildInfo())
}

func devVersion(info *debug.BuildInfo, ok bool) string {
	if !ok {
		return "dev"
	}
	var rev string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		return "dev"
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}
	if dirty {
		rev += "-dirty"
	}
	return "dev+" + rev
}
