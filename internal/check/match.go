package check

import (
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// globMeta are the characters doublestar treats specially. A prefix of a
// pattern containing none of them is a plain string, and — because the listing
// is sorted bytewise — that prefix bounds a contiguous run of the listing.
//
// The same constant exists in internal/walk for the same reason. It is not
// shared because check must not import walk (see the package doc).
const globMeta = `*?[]{}\`

// literalPrefix returns the longest leading run of pattern that contains no
// glob metacharacter. Every path a pattern can match starts with it, with one
// exception handled in [index.bounds].
func literalPrefix(pattern string) string {
	if i := strings.IndexAny(pattern, globMeta); i >= 0 {
		return pattern[:i]
	}
	return pattern
}

// prefixUpper returns the smallest string strictly greater than every string
// that starts with p, i.e. the exclusive upper bound of p's prefix range. It
// reports false when no such bound exists (p is empty, or is all 0xFF bytes —
// which cannot occur in valid UTF-8, but is handled rather than assumed away).
func prefixUpper(p string) (string, bool) {
	b := []byte(p)
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] < 0xFF {
			b[i]++
			return string(b[:i+1]), true
		}
	}
	return "", false
}

// index is the one listing, held in sorted order, with the binary-search
// helpers every glob question is answered through.
//
// The reason this type exists at all is cost. Phase 2 measured doublestar.Match
// at roughly 70ns per call, so a naive "every glob against every path" loop is
// ~7ms per glob on a 100k listing — twenty bindings would spend 140ms of a
// 200ms budget deciding what matches. Narrowing to the glob's literal-prefix
// run first turns a binding scoped to one directory into O(log n + files in
// that directory).
//
// The narrowing is a superset filter, never a reinterpretation: whatever
// survives it is still handed to doublestar.Match itself.
// TestEachMatchesDoublestar proves the equivalence differentially.
type index struct {
	entries []Entry
	// marks is scratch space for eachFile's de-duplication. It is always all
	// false between calls; eachFile clears exactly what it set. It makes index
	// non-reentrant, which is fine — one Check call owns one index.
	marks []bool
}

func newIndex(files []Entry) *index {
	if sortedByPath(files) {
		return &index{entries: files, marks: make([]bool, len(files))}
	}
	// walk guarantees sorted order and everything here depends on it, but
	// Input is an ordinary struct anyone can populate. Re-sorting a copy is
	// cheap insurance against silently wrong answers; mutating the caller's
	// slice is not this function's business.
	cp := make([]Entry, len(files))
	copy(cp, files)
	sort.Slice(cp, func(i, j int) bool { return cp[i].Path < cp[j].Path })
	return &index{entries: cp, marks: make([]bool, len(cp))}
}

func sortedByPath(files []Entry) bool {
	for i := 1; i < len(files); i++ {
		if files[i-1].Path > files[i].Path {
			return false
		}
	}
	return true
}

func (ix *index) len() int { return len(ix.entries) }

// search returns the first index whose path is >= s.
func (ix *index) search(s string) int {
	return sort.Search(len(ix.entries), func(i int) bool { return ix.entries[i].Path >= s })
}

// bounds returns the half-open index range that can contain a match for
// pattern.
//
// Correctness argument, since the whole matcher rests on it. Let P be the
// literal prefix of the pattern. Every path doublestar matches starts with P,
// with exactly one exception: a pattern of the form `X/**` also matches `X`
// itself, and when everything before the `**` is literal that path is P with
// its trailing slash removed — which sorts immediately below P. Taking the
// lower bound as strings.TrimSuffix(P, "/") therefore only ever widens the
// range, so the range is a superset of the match set in every case.
func (ix *index) bounds(pattern string) (lo, hi int) {
	p := literalPrefix(pattern)
	if p == "" {
		return 0, len(ix.entries)
	}
	lo = ix.search(strings.TrimSuffix(p, "/"))
	hi = len(ix.entries)
	if up, ok := prefixUpper(p); ok {
		hi = ix.search(up)
	}
	if hi < lo {
		hi = lo
	}
	return lo, hi
}

// each calls fn for every entry the pattern matches. It is exactly
// `for e := range listing { if doublestar.Match(pattern, e.Path) { fn(e) } }`,
// with the listing narrowed first.
func (ix *index) each(pattern string, fn func(i int, e Entry)) {
	lo, hi := ix.bounds(pattern)
	for i := lo; i < hi; i++ {
		if ok, _ := doublestar.Match(pattern, ix.entries[i].Path); ok {
			fn(i, ix.entries[i])
		}
	}
}

// eachFile calls fn for every FILE index a pattern claims and returns how many
// there were. ORPHAN is defined as "matches zero files" (DESIGN §3), and O10
// defines discover coverage in files, so files are the unit of both answers.
//
// A pattern that matches a *directory* claims everything beneath it. This is
// the same rule internal/walk applies to `exclude:` — a bare `node_modules`
// prunes the subtree, not just the directory entry — and without it the bare
// directory forms that config explicitly blesses (`shared: lib/pricing_engine`)
// would match zero files and fail as ORPHAN on their first run.
//
// The subtree sweep is cheap: the listing is sorted, so a directory's
// descendants are one contiguous run, located by binary search.
//
// It is NOT the run that immediately follows the directory entry, which is the
// obvious and wrong shortcut. Bytewise, `app/services/billing-old` and
// `app/services/billing.rb` both sort between `app/services/billing` and its
// own children, because '-' and '.' are below '/'. The run has to be searched
// for, not walked into.
func (ix *index) eachFile(pattern string, fn func(i int)) int {
	var touched []int
	mark := func(i int) {
		if ix.entries[i].IsDir || ix.marks[i] {
			return
		}
		// A placeholder keeps a directory in git; it is not code, so a binding
		// that matches only placeholders matches nothing and is an ORPHAN.
		if isPlaceholder(ix.entries[i].Path) {
			return
		}
		ix.marks[i] = true
		touched = append(touched, i)
	}

	ix.each(pattern, func(i int, e Entry) {
		if !e.IsDir {
			mark(i)
			return
		}
		lo, hi := ix.subtree(i)
		for j := lo; j < hi; j++ {
			mark(j)
		}
	})

	// A pattern can claim the same file twice — `app/**` matches both the
	// directory and the files under it — so report each one once, in listing
	// order.
	sort.Ints(touched)
	for _, i := range touched {
		ix.marks[i] = false
		if fn != nil {
			fn(i)
		}
	}
	return len(touched)
}

// eachUnit calls fn for every directory a `discover:` rule names.
//
// This is the silent-failure trap, and it is worth stating in full. walk emits
// directory paths bare — `app/services/billing`, no trailing slash — while the
// shipped example config writes `discover: app/services/*/` with one, and
// doublestar does not match a trailing-slash pattern against a bare path
// (pinned by integration.TestDiscoverGlobNeedsTrailingSlash). Matching them
// naively yields zero units, so UNMAPPED never fires and `trestle check` exits
// 0 having inspected nothing.
//
// So the slash is synthesized here, on both sides. The rule is also accepted
// without its trailing slash — `app/services/*` and `app/services/**` behave
// the same as `app/services/*/` — because making an author guess which form the
// tool wants is how the trap gets rebuilt by hand.
func (ix *index) eachUnit(rule string, fn func(i int, e Entry)) {
	base := strings.TrimSuffix(strings.TrimSpace(rule), "/")
	if base == "" {
		return
	}
	withSlash := base + "/"
	lo, hi := ix.bounds(base)
	for i := lo; i < hi; i++ {
		e := ix.entries[i]
		if !e.IsDir {
			continue
		}
		if ok, _ := doublestar.Match(withSlash, e.Path+"/"); ok {
			fn(i, e)
			continue
		}
		if ok, _ := doublestar.Match(base, e.Path); ok {
			fn(i, e)
		}
	}
}

// subtree returns the half-open index range of everything beneath entry i.
//
// Both bounds are searched for. The lower one cannot be i+1: sibling paths that
// differ after the directory's name — `billing-old`, `billing.rb` — sort
// between a directory and its own children, because '-' and '.' are below '/'.
func (ix *index) subtree(i int) (lo, hi int) {
	prefix := ix.entries[i].Path + "/"
	lo = ix.search(prefix)
	hi = len(ix.entries)
	if up, ok := prefixUpper(prefix); ok {
		hi = ix.search(up)
	}
	if hi < lo {
		hi = lo
	}
	return lo, hi
}
