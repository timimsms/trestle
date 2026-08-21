package check

// Matcher answers "what does this glob claim right now" using the same index
// [Check] runs on.
//
// It exists for `trestle explain`, which has to show the file list behind a
// binding and cannot get it from a []Violation — a glob that matches four files
// produces no violation at all, and "matches 0 files" is the answer people
// actually come looking for.
//
// It is additive: Check's behavior is untouched and nothing in this file is
// reachable from it. What matters is that the answer comes from [index.eachFile]
// rather than from a second doublestar loop written next door. The rules that
// make a match count what it is — a glob matching a directory claims every file
// beneath it, a file claimed twice counts once — live in one place, and a
// reimplementation in another package would drift from them silently, which
// would make `explain` a tool that confidently describes a check it is not
// running.
//
// A Matcher is not safe for concurrent use: the index carries scratch space.
type Matcher struct{ ix *index }

// NewMatcher prepares a matcher over one listing. As with [Input.Files], the
// listing is expected sorted by path and is copied and re-sorted if it is not.
func NewMatcher(files []Entry) *Matcher { return &Matcher{ix: newIndex(files)} }

// Files returns the repo-relative paths of every file the pattern claims, in
// listing order, each once.
//
// Directories are never returned — ORPHAN is defined in files (DESIGN §3) and
// so is discover coverage (O10), so files are the unit of the answer here too.
func (m *Matcher) Files(pattern string) []string {
	var out []string
	m.ix.eachFile(pattern, func(i int) { out = append(out, m.ix.entries[i].Path) })
	return out
}

// Count returns how many files the pattern claims, without materializing the
// list.
func (m *Matcher) Count(pattern string) int {
	return m.ix.eachFile(pattern, nil)
}
