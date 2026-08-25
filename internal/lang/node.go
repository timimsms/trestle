package lang

// Node is the JavaScript and TypeScript ecosystem's conventions.
//
// Included on the strength of the conventions rather than a field trial: npm
// workspaces, turbo and nx all standardize on `packages/` and `apps/`, and
// `src/*/` is the single-package equivalent. Marked here as the least-evidenced
// entry in this package — no repo of this shape has been probed yet, so if a
// trial contradicts these shapes, believe the trial.
var Node = Lang{
	Name:    "JavaScript / TypeScript",
	Markers: []string{"package.json"},
	Discover: []string{
		"packages/*/",
		"apps/*/",
		"src/*/",
		// A JS repo that has not adopted workspaces keeps its shared layer in
		// lib/ like everyone else. Without this a repo with lib/http-client and
		// lib/logging at the root gets nothing proposed for either — the shared
		// layer is undiscoverable, UNMAPPED can never fire on it, and the
		// coverage clause does not catch the gap because the other rules matched
		// fine. That is the "green while watching nothing" shape in the one form
		// the safety net misses.
		"lib/*/",
	},
	TestGlobs: []string{"**/*.test.ts", "**/*.test.js", "**/*.spec.ts"},
	// No prefixes: a workspace directory name is the package name, the same
	// argument as Go.
	Prefixes: nil,
}
