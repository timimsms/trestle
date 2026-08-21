package main

import (
	"runtime/debug"
)

// version is the release version, set by the linker for tagged builds:
//
//	go build -ldflags "-X main.version=v0.2.0" ./cmd/trestle
//
// It stays "dev" for everything else, and the VCS stamp below supplies the
// detail that actually matters in a bug report.
var version = "dev"

// versionString reports the build in one line.
//
// The buildinfo lookup is the point of this function. A `go install`ed binary
// carries no linker stamp, so without it every report from a non-release build
// would say "dev" and identify nothing — which is useless in the one place a
// version string is ever read. Go records the VCS revision automatically when
// building from a checkout, and that revision is what a maintainer needs.
func versionString() string {
	if version != "dev" {
		return version
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return version
	}

	// A module version beats a bare revision: `go install pkg@v0.2.0` records
	// the tag and no VCS stamp at all.
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}

	var rev, dirty string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
			if len(rev) > 12 {
				rev = rev[:12]
			}
		case "vcs.modified":
			if s.Value == "true" {
				// A dirty build is worth shouting about in a bug report: the
				// revision alone would name a tree that never existed.
				dirty = "-dirty"
			}
		}
	}

	if rev != "" {
		return "dev+" + rev + dirty
	}
	return version
}
