// Command trestle binds architecture-diagram nodes to real repo paths and fails
// CI when they diverge.
//
// This package is deliberately thin. It finds the config, loads the repo, calls
// the check engine, formats the result and picks an exit code — and it makes no
// decisions of its own. Every decision lives behind a package boundary:
// `internal/run` owns the pipeline, `internal/report` owns the format and the
// exit-code classification, `internal/check` owns what a violation is. If a
// judgement call appears in this directory, it is in the wrong place.
package main

import (
	"os"
)

func main() {
	os.Exit(Main(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
