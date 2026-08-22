package check

import "path"

// placeholderNames are the files whose only job is to make git track an
// otherwise-empty directory.
//
// Git records files, not directories, so an empty directory cannot exist in a
// commit at all. The only way a repo can say "this package is declared and its
// code is coming" is to commit one of these — which makes the placeholder the
// real-world form of the empty directory, not an edge case around it.
var placeholderNames = map[string]bool{
	".keep":        true,
	".gitkeep":     true,
	".placeholder": true,
}

// isPlaceholder reports whether a path is a directory-keeping file rather than
// code.
//
// This distinction closes a silent green found on two real repos. A node bound
// to a directory holding nothing but `.keep` reported `matches 1 file` and
// passed — a box on the diagram claiming a service that does not exist, with
// `explain` confirming "no violations". A placeholder is not code, so a binding
// that matches only placeholders matches nothing, which is what ORPHAN is for.
//
// The same rule answers the other half. A `discover:` unit holding only
// placeholders contains no code, and UNMAPPED means *code exists that the
// diagram never learned about* — so there is nothing to report until a real
// file lands, at which point it fires exactly as it should. That is the signal
// a repo with declared-but-unbuilt packages actually wants, and it needs no new
// directive, config key or violation code to get it.
//
// It cannot be used to hide anything: silencing a real UNMAPPED this way would
// mean deleting the code, which is a larger act than the check was ever going
// to prevent.
func isPlaceholder(p string) bool {
	return placeholderNames[path.Base(p)]
}
