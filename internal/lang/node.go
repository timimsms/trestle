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
	},
	TestGlobs: []string{"**/*.test.ts", "**/*.test.js", "**/*.spec.ts"},
	// No prefixes: a workspace directory name is the package name, the same
	// argument as Go.
	Prefixes: nil,
}
