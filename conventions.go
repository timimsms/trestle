// Package trestle carries the files that ship *inside* the binary.
//
// There is exactly one of them, and it is here rather than under internal/
// because `go:embed` cannot reach outside the directory of the package that
// declares it. CONVENTIONS.md is the agent contract; `trestle init` writes it
// into the repo it is initializing, and the copy it writes has to be the copy
// this repo maintains. The alternative — a second copy under internal/scaffold,
// kept in sync by hand — is a drift surface, which is the one thing this tool
// exists to close. A four-line package at the root is the cheaper price.
package trestle

import _ "embed"

// Conventions is CONVENTIONS.md, the diagram-authoring contract, verbatim.
// `trestle init` writes it into the target repo; nothing else reads it.
//
//go:embed CONVENTIONS.md
var Conventions string
