// Package walk produces the single filesystem listing the rest of Trestle runs
// on.
//
// All filesystem I/O in the product lives here. That is what makes
// `internal/check`'s purity achievable rather than aspirational: check is a
// function of (listing, nodes, directives, config), and the listing is produced
// exactly once, by [Walk], before check runs.
//
// Two rules follow from that and are load-bearing:
//
//   - There is no globbing in this package beyond `exclude:`. The walk produces
//     a listing; Phase 3 applies `discover:`, `shared:` and every `@bind` glob
//     to that one listing. One walk per binding is what the 200ms/100k-file
//     target rules out (DESIGN §7).
//   - `exclude:` is applied *during* the walk, pruning excluded directories
//     rather than filtering the result. On a repo with a large node_modules that
//     is the difference between hitting and missing the target.
package walk

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Options configures a walk.
//
// The exclude patterns are taken as a plain []string rather than as a config
// type on purpose: this package must not depend on `internal/config`, so that
// the walk stays testable and reusable without a .trestle.yml on disk. Phase 4
// wires config.Exclude into this field.
type Options struct {
	// Root is the repo root — the directory containing .trestle.yml. Every path
	// in the resulting listing is relative to it.
	Root string

	// Exclude holds repo-root-relative globs in doublestar syntax, where `**`
	// crosses directory boundaries. A directory matching any pattern is pruned
	// along with its entire subtree; see [Walk] for why.
	Exclude []string

	// FS, when non-nil, is walked instead of Root. Root is then only carried
	// into the listing as metadata. Used by tests; also the seam for a future
	// git-aware or in-memory source.
	FS fs.FS
}

// Entry is one path in the listing.
//
// IsDir is not a detail: `discover: app/services/*/` matches directories and
// `@bind app/services/billing/**` matches files. Collapsing the two breaks
// UNMAPPED, which is why the walk returns directories rather than files alone.
type Entry struct {
	// Path is repo-root-relative and slash-separated, with no leading "./".
	// The root itself is not an entry.
	Path  string
	IsDir bool
}

// Listing is the result of the one walk.
type Listing struct {
	// Root is the directory the walk started from.
	Root string

	// Entries holds every file and directory found, excluding pruned subtrees,
	// sorted bytewise by Path. The sort is part of the contract: violation
	// output must be reproducible run to run and machine to machine, and
	// fs.WalkDir's depth-first order is not the same as sorted order.
	Entries []Entry
}

// Len returns the number of entries.
func (l *Listing) Len() int { return len(l.Entries) }

// Paths returns every path, files and directories alike, in listing order.
func (l *Listing) Paths() []string {
	out := make([]string, len(l.Entries))
	for i, e := range l.Entries {
		out[i] = e.Path
	}
	return out
}

// Files returns the file paths, in listing order.
func (l *Listing) Files() []string { return l.filter(false) }

// Dirs returns the directory paths, in listing order.
func (l *Listing) Dirs() []string { return l.filter(true) }

func (l *Listing) filter(dirs bool) []string {
	out := make([]string, 0, len(l.Entries))
	for _, e := range l.Entries {
		if e.IsDir == dirs {
			out = append(out, e.Path)
		}
	}
	return out
}

// PatternError reports an unusable exclude glob. It is a tool error (exit 2),
// not a violation: the config is wrong, the repo is not.
type PatternError struct {
	Pattern string
	err     error
}

func (e *PatternError) Error() string {
	return fmt.Sprintf("invalid exclude pattern %q: %v", e.Pattern, e.err)
}
func (e *PatternError) Unwrap() error { return e.err }

// Error reports a filesystem failure at a specific path.
//
// The walk deliberately fails rather than skipping an unreadable directory. A
// silently shortened listing turns into phantom ORPHAN and UNMAPPED violations
// three phases downstream, and the author has no way to tell that from real
// drift.
type Error struct {
	Path string
	err  error
}

func (e *Error) Error() string { return fmt.Sprintf("walk %s: %v", e.Path, e.err) }
func (e *Error) Unwrap() error { return e.err }

// Walk performs the single filesystem walk and returns the sorted listing.
//
// Exclusion semantics, stated once because everything downstream depends on it:
//
//   - A file is omitted when its path matches any exclude pattern.
//   - A directory is omitted, *and its whole subtree is skipped*, when its path
//     matches any exclude pattern. This is why the default `exclude: [node_modules]`
//     — a bare name, not `node_modules/**` — prunes rather than merely hiding one
//     directory entry, and it is what makes pruning during the walk correct
//     rather than an optimization that changes the answer.
//   - `.git` is skipped unconditionally, at any depth, whatever the config says.
//
// Symlinks are never followed: fs.WalkDir reports them via Lstat, so a symlink
// to a directory appears as a file entry. That keeps the walk cycle-free and
// its cost bounded by the tree, which the perf target requires.
func Walk(opts Options) (*Listing, error) {
	m, err := newMatcher(opts.Exclude)
	if err != nil {
		return nil, err
	}

	fsys := opts.FS
	root := opts.Root
	if fsys == nil {
		if root == "" {
			root = "."
		}
		fsys = os.DirFS(root)
	}

	entries := make([]Entry, 0, 1024)
	walkErr := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return &Error{Path: p, err: err}
		}
		if p == "." {
			return nil
		}
		isDir := d.IsDir()
		if path.Base(p) == ".git" {
			// A .git *file* is a worktree or submodule pointer, still git
			// metadata; neither form is ever architecturally real.
			if isDir {
				return fs.SkipDir
			}
			return nil
		}
		if m.match(p) {
			if isDir {
				return fs.SkipDir
			}
			return nil
		}
		entries = append(entries, Entry{Path: p, IsDir: isDir})
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	slices.SortFunc(entries, func(a, b Entry) int { return strings.Compare(a.Path, b.Path) })
	return &Listing{Root: root, Entries: entries}, nil
}

// matcher is the compiled exclude set.
//
// It exists for two reasons. First, validation has to happen up front: a bad
// pattern must fail before the walk starts, not on whichever path happens to
// reach it first. Second, cost. Every pattern is tested against every path, so
// the exclude set is multiplied by the size of the repo — on a 100k-file tree a
// naive doublestar.Match per pattern costs roughly 7ms per pattern, which means
// an eight-entry exclude list would spend a third of the entire 200ms check
// budget deciding what not to look at.
//
// The fast paths below are exact rewrites of doublestar's semantics, not
// approximations, and TestMatcherMatchesDoublestar proves it differentially
// against the library over a path corpus. Anything not covered falls through to
// doublestar itself.
type matcher struct {
	pats []pattern
}

type patternKind uint8

const (
	// kindGlob: no rewrite applies; ask doublestar.
	kindGlob patternKind = iota
	// kindLiteral: no metacharacters, so the pattern matches one exact path.
	kindLiteral
	// kindBaseLiteral: "**/name" — matches iff the path's final segment is name.
	kindBaseLiteral
	// kindBaseGlob: "**/expr" where expr has no separator — matches iff the
	// final segment matches expr. This is the big one: "**/*_test.*" becomes a
	// match against one short segment instead of a scan over the whole path.
	kindBaseGlob
	// kindSegmentLiteral: "**/name/**" — matches iff any segment is name.
	kindSegmentLiteral
	// kindPrefixLiteral: "name/**" — matches name and everything beneath it.
	kindPrefixLiteral
)

type pattern struct {
	kind patternKind
	raw  string // the original pattern, for kindGlob
	arg  string // the extracted literal or sub-expression
}

// globMeta are the characters doublestar treats specially. A pattern containing
// none of them is a plain string and can be compared as one.
const globMeta = `*?[]{}\`

func isLiteral(s string) bool { return !strings.ContainsAny(s, globMeta) }

func newMatcher(patterns []string) (*matcher, error) {
	out := make([]pattern, 0, len(patterns))
	for _, raw := range patterns {
		// A trailing slash is how people write "this is a directory". It means
		// nothing to doublestar and would simply never match, so it is dropped
		// rather than silently disabling the pattern.
		raw = strings.TrimSuffix(raw, "/")
		if raw == "" {
			continue
		}
		if !doublestar.ValidatePattern(raw) {
			return nil, &PatternError{Pattern: raw, err: errors.New("not a valid glob")}
		}
		out = append(out, compilePattern(raw))
	}
	return &matcher{pats: out}, nil
}

func compilePattern(raw string) pattern {
	switch {
	case isLiteral(raw):
		return pattern{kind: kindLiteral, raw: raw, arg: raw}

	case strings.HasPrefix(raw, "**/") && strings.HasSuffix(raw, "/**"):
		// "**/x/**": zero or more segments, a segment matching x, then zero or
		// more segments — i.e. some segment matches x.
		inner := raw[3 : len(raw)-3]
		if inner != "" && !strings.Contains(inner, "/") && isLiteral(inner) {
			return pattern{kind: kindSegmentLiteral, raw: raw, arg: inner}
		}

	case strings.HasPrefix(raw, "**/"):
		// "**/x": zero or more leading segments then a final segment matching x.
		inner := raw[3:]
		if inner != "" && !strings.Contains(inner, "/") {
			if isLiteral(inner) {
				return pattern{kind: kindBaseLiteral, raw: raw, arg: inner}
			}
			return pattern{kind: kindBaseGlob, raw: raw, arg: inner}
		}

	case strings.HasSuffix(raw, "/**"):
		// "x/**": x itself plus everything beneath it.
		inner := raw[:len(raw)-3]
		if isLiteral(inner) {
			return pattern{kind: kindPrefixLiteral, raw: raw, arg: inner}
		}
	}
	return pattern{kind: kindGlob, raw: raw, arg: raw}
}

func (m *matcher) match(p string) bool {
	if len(m.pats) == 0 {
		return false
	}
	base := p
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		base = p[i+1:]
	}
	for i := range m.pats {
		if m.pats[i].match(p, base) {
			return true
		}
	}
	return false
}

func (pt *pattern) match(p, base string) bool {
	switch pt.kind {
	case kindLiteral:
		return p == pt.arg
	case kindBaseLiteral:
		return base == pt.arg
	case kindBaseGlob:
		ok, _ := doublestar.Match(pt.arg, base)
		return ok
	case kindSegmentLiteral:
		return hasSegment(p, pt.arg)
	case kindPrefixLiteral:
		return p == pt.arg ||
			(len(p) > len(pt.arg) && p[len(pt.arg)] == '/' && p[:len(pt.arg)] == pt.arg)
	default:
		// Validated in newMatcher, so the only error doublestar can return here
		// is ErrBadPattern, which cannot occur.
		ok, _ := doublestar.Match(pt.raw, p)
		return ok
	}
}

// hasSegment reports whether any slash-delimited segment of p equals seg.
func hasSegment(p, seg string) bool {
	for {
		i := strings.IndexByte(p, '/')
		if i < 0 {
			return p == seg
		}
		if p[:i] == seg {
			return true
		}
		p = p[i+1:]
	}
}
