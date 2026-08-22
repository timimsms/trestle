package check

import (
	"fmt"
	"path"

	"github.com/timimsms/trestle/internal/lang"
	"sort"
	"strings"

	"github.com/timimsms/trestle/internal/directive"
	"github.com/timimsms/trestle/internal/nodes"
)

// Hints are a contract, not a nicety. Every violation carries one and every one
// of them names something the author can run or paste. A failing check that
// does not tell you what to type is one people learn to route around, so the
// shapes below are fixed by PHASE_3 §"Every violation carries a hint" and
// golden-tested in Phase 4.

// syntaxTarget recovers the node ID a malformed directive was probably about,
// for reporting only.
//
// directive.SyntaxError deliberately carries no Node: the line did not parse,
// so nothing on it is trustworthy. That is exactly why this value never feeds a
// decision — it is not used to account for a node, to suppress anything, or to
// resolve against the AST. It exists so the violation has something to point at
// other than a line number.
func syntaxTarget(raw string) string {
	body := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(raw), "#"))
	fields := strings.Fields(body)
	if len(fields) < 2 {
		return ""
	}
	known := false
	for _, k := range directive.Kinds {
		if string(k) == fields[0] {
			known = true
			break
		}
	}
	if !known || strings.HasPrefix(fields[1], `"`) {
		return ""
	}
	return fields[1]
}

func syntaxHint(se directive.SyntaxError) string {
	if se.Want == "" {
		return fmt.Sprintf("%s is not a directive; the four are %s", quoteLine(se.Raw), joinKinds())
	}
	return fmt.Sprintf("%s should be `%s`", quoteLine(se.Raw), se.Want)
}

func quoteLine(raw string) string {
	return "`" + strings.TrimSpace(raw) + "`"
}

func joinKinds() string {
	out := make([]string, 0, len(directive.Kinds))
	for _, k := range directive.Kinds {
		out = append(out, "`"+string(k)+"`")
	}
	return strings.Join(out, ", ")
}

// ambiguousHint lists every candidate so the fix is a copy-paste, and shows the
// directive rewritten with one of them. It never picks for the author — the
// example is illustrative and all candidates are named.
func ambiguousHint(d directive.Directive, cands []string) string {
	quoted := make([]string, 0, len(cands))
	for _, c := range cands {
		quoted = append(quoted, "`"+c+"`")
	}
	rewritten := d
	rewritten.Node = cands[0]
	return fmt.Sprintf("qualify it — candidates: %s; e.g. `# %s`",
		strings.Join(quoted, ", "), rewritten.String())
}

// orphanHint points at the deleted directory. A rename is the usual cause and
// git already knows about it.
func orphanHint(glob string) string {
	return fmt.Sprintf("renamed? `git log --diff-filter=D -- %s`", globAnchor(glob))
}

// placeholderOrphanHint is for a binding whose directory is right there and
// holds nothing but a `.keep`. "renamed?" would send the author looking for
// something that was never moved; the honest reading is that the box was drawn
// before the code was written.
func placeholderOrphanHint(glob string) string {
	return fmt.Sprintf("`%s` exists but holds only placeholder files — nothing to bind yet. "+
		"Remove the node until the code lands, or `# @ignore <node> \"declared, not built\"` "+
		"if the box is load-bearing on the diagram now", globAnchor(glob))
}

func sharedOrphanHint(entry string) string {
	return fmt.Sprintf("renamed? `git log --diff-filter=D -- %s` — otherwise drop `%s` from `shared:` in %s",
		globAnchor(entry), entry, "`.trestle.yml`")
}

func discoverOrphanHint(rule string) string {
	return fmt.Sprintf("no directory matches it: `ls -d %s` — fix or drop the rule in `discover:`, "+
		"because a discover rule that matches nothing makes UNMAPPED silently stop firing",
		strings.TrimSuffix(strings.TrimSpace(rule), "/")+"/")
}

// unmappedHint produces the `# @bind ...` line to paste, with a node ID derived
// from the unit's own path. DESIGN §5 shows exactly this hint for
// app/services/notifications/.
func unmappedHint(unit string, langs []lang.Lang) string {
	return fmt.Sprintf("add `# @bind %s %s/**` to a diagram, or add `%s/**` to `shared:`",
		suggestNodeID(unit, langs), unit, unit)
}

func emptyUnitHint(unit string) string {
	return fmt.Sprintf("`%s/` contains no files — delete the empty directory, "+
		"or narrow the `discover:` rule that matches it", unit)
}

func unboundHint(id string) string {
	return fmt.Sprintf("add one of `# @bind %s <glob>`, `# @infra %s`, `# @external %s`, "+
		"or `# @ignore %s \"<reason>\"` — for a database or queue the answer is usually `@infra`",
		id, id, id, id)
}

// danglingHint names the closest existing node IDs by edit distance, because a
// rename is the usual cause and the new name is usually one keystroke away.
func danglingHint(d directive.Directive, dg *nodes.Diagram) string {
	near := nearest(d.Node, dg, 3)
	if len(near) == 0 {
		return fmt.Sprintf("no node named `%s` in %s — delete the directive at %s, or add the node",
			d.Node, dg.Path, d.Source)
	}
	quoted := make([]string, 0, len(near))
	for _, n := range near {
		quoted = append(quoted, "`"+n+"`")
	}
	return fmt.Sprintf("did you mean %s? update the directive at %s", strings.Join(quoted, " or "), d.Source)
}

// nearest returns up to max node IDs closest to token, comparing against both
// the fully-qualified ID and its final segment, since directives are usually
// written unqualified.
func nearest(token string, dg *nodes.Diagram, max int) []string {
	if dg == nil || token == "" {
		return nil
	}
	type scored struct {
		id   string
		dist int
	}
	limit := len(token)/2 + 1
	if limit > 4 {
		limit = 4
	}
	var out []scored
	for _, id := range dg.IDs {
		n, ok := dg.Node(id)
		if !ok {
			continue
		}
		d := levenshtein(token, id)
		if e := levenshtein(token, n.Name); e < d {
			d = e
		}
		if d <= limit {
			out = append(out, scored{id: id, dist: d})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].dist != out[j].dist {
			return out[i].dist < out[j].dist
		}
		return out[i].id < out[j].id
	})
	if len(out) > max {
		out = out[:max]
	}
	ids := make([]string, 0, len(out))
	for _, s := range out {
		ids = append(ids, s.id)
	}
	return ids
}

func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 {
		return len(rb)
	}
	if len(rb) == 0 {
		return len(ra)
	}
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min3(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

// globAnchor is the deepest real directory a glob is anchored on — what to hand
// to `git log`. `app/services/billing/**` anchors on app/services/billing;
// `app/serv*/**` anchors on app, because app/serv is not a path.
func globAnchor(glob string) string {
	glob = strings.TrimSpace(glob)
	p := literalPrefix(glob)
	if p == glob {
		return strings.TrimSuffix(glob, "/")
	}
	if strings.HasSuffix(p, "/") {
		return strings.TrimSuffix(p, "/")
	}
	if i := strings.LastIndexByte(p, '/'); i > 0 {
		return p[:i]
	}
	return "."
}

// suggestNodeID derives a node ID for an unmapped unit from its path:
// app/services/notifications -> svc_notifications, per CONVENTIONS.md's
// prefix-by-kind rule.
//
// The prefix depends on the ecosystem, which is why langs is threaded in rather
// than read from a package-level map. The prefixes are Rails vocabulary, and on
// a Go repo they were actively wrong: `svc_db` for a package named `db` appears
// nowhere in the repo, and `db_` is what CONVENTIONS reserves for datastores.
// Go and Node contribute no prefixes, so the ID stays the package name — which
// is already the identifier somebody would grep for.
func suggestNodeID(unit string, langs []lang.Lang) string {
	base := path.Base(unit)
	if base == "." || base == "/" || base == "" {
		return "node_id"
	}
	parent := path.Base(path.Dir(unit))
	id := lang.Prefix(langs, parent) + base
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		default:
			return '_'
		}
	}, id)
}
