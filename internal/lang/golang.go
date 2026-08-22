package lang

// Go is the Go ecosystem's conventions.
//
// Packages are Go's unit of modularity and depth 2 lands on exactly that:
// `internal/db`, `cmd/reconciler`. Depth 3 fragments — one real repo went to 32
// units with 19 holding two files or fewer.
//
// Note the module marker is not an anchor. go.mod sits at the module root, and
// the source tree is routinely a directory below it: a real repo kept go.mod at
// the top with every package under api/, which made these shapes match nothing
// and `init` propose no rules at all.
var Go = Lang{
	Name:    "Go",
	Markers: []string{"go.mod"},
	Discover: []string{
		"internal/*/",
		"pkg/*/",
		"cmd/*/",
	},
	// Offered with a caveat in the emitted config, because this one is a trap.
	// An external test package (`package foo_test`) can be the only file in its
	// directory, so excluding *_test.go turns a healthy unit into an empty one
	// — on a real repo, uncommenting this line took the check from 7 failures
	// to 9.
	TestGlobs: []string{"**/*_test.go"},
	// Empty on purpose. A Go package name is a single lowercase word that is
	// already the identifier you would grep for — `db`, `auth`, `rig`. Prefixing
	// it produces `svc_db`, which appears nowhere in the repo, and `db_` is
	// reserved by CONVENTIONS for datastores, so the collision is worse than
	// the redundancy.
	Prefixes: nil,
}
