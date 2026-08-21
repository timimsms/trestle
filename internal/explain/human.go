package explain

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/timimsms/trestle/internal/config"
	"github.com/timimsms/trestle/internal/directive"
)

// The human view borrows its grammar from `internal/report` on purpose — two
// spaces of indent, a ten-wide first column, body text aligned under the subject
// — so that `check` and `explain` read as one tool rather than two. What sits in
// the first column is a binding status and never a violation code; the taxonomy
// is closed at five and this is not an application to reopen it.
const (
	indent      = "  "
	statusWidth = 10
	// maxList caps how many paths a single glob or overlap prints. An inventory
	// that dumps 40,000 filenames into a terminal is not observable either, and
	// the machine format is right there and prints all of them.
	maxList = 10
)

var (
	bodyIndent = strings.Repeat(" ", len(indent)+statusWidth)
	listIndent = bodyIndent + indent
)

// out buffers human output and drops trailing whitespace on every line.
//
// The trimming is not tidiness. This output is column-aligned, so a node with an
// empty detail column ends its line in ten spaces; a golden file full of
// invisible trailing whitespace is one nobody can review, which is the same
// argument `internal/report` makes about its bare `hint:` line.
//
// bufio.Writer latches the first write error and returns it from Flush, so the
// individual writes genuinely have nothing to handle.
type out struct{ w *bufio.Writer }

func (o out) printf(format string, a ...any) {
	lines := strings.Split(fmt.Sprintf(format, a...), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	_, _ = o.w.WriteString(strings.Join(lines, "\n"))
}

func writeHuman(w io.Writer, v *View) error {
	o := out{w: bufio.NewWriter(w)}
	switch v.Kind {
	case KindOverlaps:
		writeOverlaps(o, v)
	case KindNode:
		writeNodeView(o, v)
	default:
		writeInventory(o, v)
	}
	writeDisabled(o, v.Report)
	return o.w.Flush()
}

// --- inventory ---------------------------------------------------------

func writeInventory(o out, v *View) {
	widths := idWidths(v.Nodes)
	file := ""
	for i, n := range v.Nodes {
		if i == 0 || n.Diagram != file {
			file = n.Diagram
			if i > 0 {
				o.printf("\n")
			}
			o.printf("%s\n\n", file)
		}
		o.printf("%s%s%s%s\n",
			indent, pad(string(n.Status), statusWidth), pad(n.ID, widths[file]), inventoryDetail(n))
	}
	if len(v.Nodes) == 0 {
		o.printf("no nodes\n")
	}

	writeUnresolved(o, v.Report)

	o.printf("\n%s\n", nodeCounts(v.Report))
	if v.Report.Counts.Overlaps > 0 {
		o.printf("%s claimed by more than one node — `trestle explain --overlaps`\n",
			plural(v.Report.Counts.Overlaps, "path"))
	}
	if s := v.Report.Counts.Failures + v.Report.Counts.Warnings; s > 0 {
		o.printf("%s, %s — `trestle check`\n",
			plural(v.Report.Counts.Failures, "failure"), plural(v.Report.Counts.Warnings, "warning"))
	}
}

// inventoryDetail is the one-line answer to "what is behind this box": the glob
// and its current match count for a bound node, and whatever else stands in for
// that for a node with no glob. A zero here is the finding.
func inventoryDetail(n *Node) string {
	var detail string
	switch {
	case len(n.Bindings) == 1:
		detail = fmt.Sprintf("%s — %s", n.Bindings[0].Glob, plural(n.Bindings[0].Matches(), "file"))
	case len(n.Bindings) > 1:
		detail = fmt.Sprintf("%s — %s", plural(len(n.Bindings), "glob"), plural(n.Files(), "file"))
	case n.Status == StatusIgnored:
		detail = fmt.Sprintf("%q", ignoreReason(n))
	case n.Status == StatusContainer:
		detail = plural(len(n.Children), "child")
	}
	codes := violationCodes(n)
	if codes == "" {
		return detail
	}
	if detail == "" {
		return codes
	}
	return detail + "  " + codes
}

func violationCodes(n *Node) string {
	seen := map[string]bool{}
	var out []string
	for _, v := range n.Violations {
		if seen[string(v.Code)] {
			continue
		}
		seen[string(v.Code)] = true
		out = append(out, string(v.Code))
	}
	return strings.Join(out, " ")
}

func ignoreReason(n *Node) string {
	for _, m := range n.Marks {
		if m.Kind == directive.KindIgnore {
			return m.Reason
		}
	}
	return ""
}

// idWidths sizes the ID column per diagram, so the columns line up within the
// block someone is reading rather than being dictated by the longest ID
// anywhere in the repo.
func idWidths(ns []*Node) map[string]int {
	w := map[string]int{}
	for _, n := range ns {
		if len(n.ID)+2 > w[n.Diagram] {
			w[n.Diagram] = len(n.ID) + 2
		}
	}
	return w
}

func nodeCounts(r *Report) string {
	var parts []string
	for _, s := range Statuses {
		if n := r.Counts.Status[s]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, s))
		}
	}
	if len(parts) == 0 {
		return plural(r.Counts.Nodes, "node")
	}
	return fmt.Sprintf("%s: %s", plural(r.Counts.Nodes, "node"), strings.Join(parts, ", "))
}

// --- unresolved --------------------------------------------------------

// writeUnresolved lists the directive lines that named no single node.
//
// They are the missing half of the inventory: a binding that resolved to
// nothing is invisible among the nodes, so an inventory without this section
// would look complete while a dead `@bind` sat in the file.
func writeUnresolved(o out, r *Report) {
	if len(r.Unresolved) == 0 {
		return
	}
	o.printf("\nunresolved directives\n")
	for _, u := range r.Unresolved {
		subject := u.Node
		status := "dangling"
		detail := u.Detail
		switch {
		case u.Kind == "":
			status, subject = "malformed", "(line did not parse)"
		case len(u.Candidates) > 0:
			status = "ambiguous"
			detail = fmt.Sprintf("%s; candidates: %s", u.Detail, strings.Join(u.Candidates, ", "))
		}
		o.printf("\n%s%s%s\n", indent, pad(status, statusWidth), subject)
		o.printf("%s%s (%s)\n", bodyIndent, unresolvedLine(u), u.Source)
		o.printf("%s%s\n", bodyIndent, detail)
	}
}

func unresolvedLine(u Unresolved) string {
	if u.Kind == "" {
		return "`" + strings.TrimSpace(u.Raw) + "`"
	}
	if u.Glob != "" {
		return fmt.Sprintf("%s %s %s", u.Kind, u.Node, u.Glob)
	}
	return fmt.Sprintf("%s %s", u.Kind, u.Node)
}

// --- node view ---------------------------------------------------------

func writeNodeView(o out, v *View) {
	if v.Ambiguous() {
		// O8 again, from the other side: `check` refuses to pick and reports
		// SYNTAX, so the debugging command must show the same set rather than
		// resolving it for the reader.
		o.printf("`%s` is ambiguous: it names %s\n\n", v.Query, plural(len(v.Nodes), "node"))
	}
	file := ""
	for i, n := range v.Nodes {
		if i > 0 {
			o.printf("\n")
		}
		if n.Diagram != file {
			file = n.Diagram
			o.printf("%s\n\n", file)
		}
		writeNodeBlock(o, v, n)
	}
	if v.Ambiguous() {
		o.printf("\nhint: qualify it — `trestle explain %s`\n", v.Nodes[0].ID)
	}
}

func writeNodeBlock(o out, v *View, n *Node) {
	o.printf("%s%s%s\n", indent, pad(string(n.Status), statusWidth), n.ID)
	o.printf("%s%s\n", bodyIndent, nodeMeta(n))

	for _, b := range n.Bindings {
		o.printf("%s@bind %s (%s) matches %s\n", bodyIndent, b.Glob, b.Source, plural(b.Matches(), "file"))
		writeList(o, b.Files)
	}
	for _, m := range n.Marks {
		line := fmt.Sprintf("%s%s (%s)", bodyIndent, m.Kind, m.Source)
		if m.Reason != "" {
			line = fmt.Sprintf("%s%s %q (%s)", bodyIndent, m.Kind, m.Reason, m.Source)
		}
		o.printf("%s\n", line)
	}
	if len(n.Bindings) == 0 && len(n.Marks) == 0 {
		o.printf("%sno @bind, @external, @infra or @ignore\n", bodyIndent)
	}

	for _, vio := range n.Violations {
		o.printf("%s%s%s\n", bodyIndent, pad(string(vio.Code), statusWidth), vio.Detail)
		// The hint prints here as it does in `check`. It is a contract: a
		// finding that does not say what to type is one people route around,
		// and `explain` is where someone has come to find out what to type.
		o.printf("%s%shint: %s\n", bodyIndent, strings.Repeat(" ", statusWidth), vio.Hint)
	}
	if len(n.Violations) == 0 {
		o.printf("%sno violations\n", bodyIndent)
	}

	writeNodeOverlaps(o, v, n)
}

func nodeMeta(n *Node) string {
	parts := []string{fmt.Sprintf("label %q", n.Label)}
	if n.Shape != "" {
		parts = append(parts, "shape "+n.Shape)
	}
	if n.Parent != "" {
		parts = append(parts, "inside "+n.Parent)
	}
	if len(n.Children) > 0 {
		parts = append(parts, "contains "+strings.Join(n.Children, ", "))
	}
	if n.Line > 0 {
		parts = append(parts, fmt.Sprintf("declared line %d", n.Line))
	}
	return strings.Join(parts, ", ")
}

func writeNodeOverlaps(o out, v *View, n *Node) {
	var paths []string
	others := map[string]bool{}
	for _, ov := range v.Overlaps {
		mine := false
		for _, c := range ov.Claims {
			if c.Node == n.ID {
				mine = true
			}
		}
		if !mine {
			continue
		}
		paths = append(paths, ov.Path)
		for _, c := range ov.Claims {
			if c.Node != n.ID {
				others[c.Node] = true
			}
		}
	}
	if len(paths) == 0 {
		return
	}
	names := make([]string, 0, len(others))
	for id := range others {
		names = append(names, id)
	}
	sort.Strings(names)
	// Not a violation and not phrased as one. L12 says two nodes may honestly
	// share a directory; the reader decides whether this pair does.
	o.printf("%s%s also claimed by %s\n", bodyIndent, plural(len(paths), "path"), strings.Join(names, ", "))
	writeList(o, paths)
}

// --- overlaps ----------------------------------------------------------

func writeOverlaps(o out, v *View) {
	groups := groupOverlaps(v.Overlaps)
	if len(groups) == 0 {
		o.printf("no path is claimed by more than one node\n")
		return
	}
	o.printf("overlapping bindings\n\n")
	width := 0
	for _, g := range groups {
		for _, c := range g.Claims {
			if len(c.Node) > width {
				width = len(c.Node)
			}
		}
	}
	for _, g := range groups {
		o.printf("%s%s\n", indent, strings.Join(g.Nodes, ", "))
		for _, c := range g.Claims {
			o.printf("%s%s@bind %s (%s)\n", bodyIndent, pad(c.Node, width+2), c.Glob, c.Source)
		}
		o.printf("%s%s:\n", bodyIndent, plural(len(g.Paths), "path"))
		writeList(o, g.Paths)
		o.printf("\n")
	}
	o.printf("%s claimed by more than one node — legal, and never a failure\n",
		plural(len(v.Overlaps), "path"))
}

// --- shared ------------------------------------------------------------

// writeDisabled names the codes config switched off, in every view.
//
// `check` cannot report what it has been told not to report, so a repo that sets
// ORPHAN to `off` gets a clean run out of a check that is no longer looking.
// This is the command where that has to be visible, and it prints even when the
// view is about one node, because the fact is about the run.
func writeDisabled(o out, r *Report) {
	if len(r.Disabled) == 0 {
		return
	}
	names := make([]string, 0, len(r.Disabled))
	for _, c := range r.Disabled {
		names = append(names, string(c))
	}
	verb := "is"
	if len(names) > 1 {
		verb = "are"
	}
	o.printf("\nnote: %s %s off in %s — `trestle check` reports nothing for %s\n",
		strings.Join(names, ", "), verb, config.Filename, pronoun(len(names)))
}

func pronoun(n int) string {
	if n == 1 {
		return "it"
	}
	return "them"
}

func writeList(o out, items []string) {
	for i, it := range items {
		if i == maxList && len(items) > maxList+1 {
			o.printf("%s... and %d more (`--format=json` lists all)\n", listIndent, len(items)-maxList)
			return
		}
		o.printf("%s%s\n", listIndent, it)
	}
}

func pad(s string, w int) string {
	if len(s) >= w {
		return s + " "
	}
	return s + strings.Repeat(" ", w-len(s))
}

// plural renders a count with its noun. The nouns are regular except "child",
// which is spelled out rather than pluralized by rule; this is not a general
// inflector.
func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	if noun == "child" {
		return fmt.Sprintf("%d children", n)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
