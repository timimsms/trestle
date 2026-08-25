package directive_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/timimsms/trestle/internal/directive"
)

// parseOne runs a single line through the scanner and asserts it produced
// either exactly one directive or exactly one syntax error.
func parseOne(t *testing.T, line string) directive.Result {
	t.Helper()
	res := directive.Parse("test.d2", []byte(line+"\n"))
	if n := len(res.Directives) + len(res.Syntax); n > 1 {
		t.Fatalf("line %q produced %d results, want at most 1: %+v", line, n, res)
	}
	return res
}

func TestParseWellFormed(t *testing.T) {
	tests := []struct {
		name string
		line string
		want directive.Directive
	}{
		{
			name: "bind",
			line: "# @bind svc_billing app/services/billing/**",
			want: directive.Directive{Kind: directive.KindBind, Node: "svc_billing", Glob: "app/services/billing/**"},
		},
		{
			name: "bind with column alignment",
			line: "#   @bind    svc_billing     app/services/billing/**",
			want: directive.Directive{Kind: directive.KindBind, Node: "svc_billing", Glob: "app/services/billing/**"},
		},
		{
			name: "bind indented",
			line: "\t  # @bind svc_billing app/services/billing/**",
			want: directive.Directive{Kind: directive.KindBind, Node: "svc_billing", Glob: "app/services/billing/**"},
		},
		{
			name: "bind no space after hash",
			line: "#@bind svc_billing app/services/billing/**",
			want: directive.Directive{Kind: directive.KindBind, Node: "svc_billing", Glob: "app/services/billing/**"},
		},
		{
			name: "bind double hash",
			line: "## @bind svc_billing app/services/billing/**",
			want: directive.Directive{Kind: directive.KindBind, Node: "svc_billing", Glob: "app/services/billing/**"},
		},
		{
			name: "bind container-qualified node",
			line: "# @bind platform.svc_billing app/services/billing/**",
			want: directive.Directive{Kind: directive.KindBind, Node: "platform.svc_billing", Glob: "app/services/billing/**"},
		},
		{
			name: "external",
			line: "# @external ext_stripe",
			want: directive.Directive{Kind: directive.KindExternal, Node: "ext_stripe"},
		},
		{
			name: "infra",
			line: "# @infra db_primary",
			want: directive.Directive{Kind: directive.KindInfra, Node: "db_primary"},
		},
		{
			name: "ignore",
			line: `# @ignore legacy_reporting "deleted Q3, kept for the migration narrative"`,
			want: directive.Directive{
				Kind:   directive.KindIgnore,
				Node:   "legacy_reporting",
				Reason: "deleted Q3, kept for the migration narrative",
			},
		},
		{
			name: "ignore reason with trailing whitespace outside quotes",
			line: `# @ignore legacy "why"   `,
			want: directive.Directive{Kind: directive.KindIgnore, Node: "legacy", Reason: "why"},
		},
		{
			name: "ignore reason containing a hash",
			line: `# @ignore legacy "see #412 for context"`,
			want: directive.Directive{Kind: directive.KindIgnore, Node: "legacy", Reason: "see #412 for context"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := parseOne(t, tc.line)
			if len(res.Syntax) != 0 {
				t.Fatalf("unexpected syntax error: %v", res.Syntax[0])
			}
			if len(res.Directives) != 1 {
				t.Fatalf("got %d directives, want 1", len(res.Directives))
			}
			got := res.Directives[0]
			if got.Kind != tc.want.Kind || got.Node != tc.want.Node ||
				got.Glob != tc.want.Glob || got.Reason != tc.want.Reason {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
			if got.Source.File != "test.d2" || got.Source.Line != 1 {
				t.Errorf("got source %v, want test.d2:1", got.Source)
			}
			if got.Raw != strings.TrimSpace(tc.line) {
				t.Errorf("got raw %q, want %q", got.Raw, strings.TrimSpace(tc.line))
			}
		})
	}
}

// TestParseSyntax covers every malformed case named in docs/DESIGN.md §2 §2.1,
// plus the near-misses that share their code paths.
func TestParseSyntax(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		wantDetail string // substring
		wantWant   string // expected canonical form, "" when unknown directive
	}{
		// --- the phase file's table, in order ---
		{
			name:       "ignore without reason",
			line:       "# @ignore node",
			wantDetail: "requires a quoted reason",
			wantWant:   `# @ignore <node_id> "<reason>"`,
		},
		{
			name:       "ignore with unquoted reason",
			line:       "# @ignore node unquoted words",
			wantDetail: "must be a quoted string",
			wantWant:   `# @ignore <node_id> "<reason>"`,
		},
		{
			name:       "bind without glob",
			line:       "# @bind node",
			wantDetail: "requires a glob",
			wantWant:   `# @bind <node_id> <glob>`,
		},
		{
			name:       "bind without node ID",
			line:       "# @bind",
			wantDetail: "requires a node ID and a glob",
			wantWant:   `# @bind <node_id> <glob>`,
		},
		{
			name:       "external with trailing tokens",
			line:       "# @external node extra",
			wantDetail: "unexpected trailing tokens",
			wantWant:   `# @external <node_id>`,
		},
		{
			name:       "unknown directive is not fuzzy-matched",
			line:       "# @bnid node glob",
			wantDetail: `unknown directive "@bnid"`,
			wantWant:   "",
		},

		// --- same code paths, other kinds ---
		{
			name:       "bind with trailing tokens",
			line:       "# @bind node a/** b/**",
			wantDetail: "unexpected trailing tokens after the glob",
			wantWant:   `# @bind <node_id> <glob>`,
		},
		{
			name:       "external without node ID",
			line:       "# @external",
			wantDetail: "@external requires a node ID",
			wantWant:   `# @external <node_id>`,
		},
		{
			name:       "infra without node ID",
			line:       "# @infra",
			wantDetail: "@infra requires a node ID",
			wantWant:   `# @infra <node_id>`,
		},
		{
			name:       "infra with trailing tokens",
			line:       "# @infra db_primary postgres",
			wantDetail: "unexpected trailing tokens after the node ID",
			wantWant:   `# @infra <node_id>`,
		},
		{
			name:       "ignore without node ID",
			line:       "# @ignore",
			wantDetail: "requires a node ID and a quoted reason",
			wantWant:   `# @ignore <node_id> "<reason>"`,
		},
		{
			name:       "ignore with unterminated reason",
			line:       `# @ignore node "why`,
			wantDetail: "unterminated",
			wantWant:   `# @ignore <node_id> "<reason>"`,
		},
		{
			name:       "ignore with tokens after the reason",
			line:       `# @ignore node "why" and more`,
			wantDetail: "unexpected trailing tokens after the reason string",
			wantWant:   `# @ignore <node_id> "<reason>"`,
		},
		{
			name:       "ignore with empty reason",
			line:       `# @ignore node ""`,
			wantDetail: "must not be empty",
			wantWant:   `# @ignore <node_id> "<reason>"`,
		},
		{
			name:       "ignore with whitespace-only reason",
			line:       `# @ignore node "   "`,
			wantDetail: "must not be empty",
			wantWant:   `# @ignore <node_id> "<reason>"`,
		},
		{
			name:       "bare at-sign",
			line:       "# @",
			wantDetail: `unknown directive "@"`,
			wantWant:   "",
		},
		{
			name:       "case-sensitive: @Bind is unknown",
			line:       "# @Bind node glob",
			wantDetail: `unknown directive "@Bind"`,
			wantWant:   "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := parseOne(t, tc.line)
			if len(res.Directives) != 0 {
				t.Fatalf("got a directive for malformed line: %+v", res.Directives[0])
			}
			if len(res.Syntax) != 1 {
				t.Fatalf("got %d syntax errors, want 1", len(res.Syntax))
			}
			got := res.Syntax[0]
			if !strings.Contains(got.Detail, tc.wantDetail) {
				t.Errorf("detail = %q, want it to contain %q", got.Detail, tc.wantDetail)
			}
			if got.Want != tc.wantWant {
				t.Errorf("want form = %q, want %q", got.Want, tc.wantWant)
			}
			if got.Raw != strings.TrimSpace(tc.line) {
				t.Errorf("raw = %q, want %q", got.Raw, strings.TrimSpace(tc.line))
			}
			if got.Source.File != "test.d2" || got.Source.Line != 1 {
				t.Errorf("source = %v, want test.d2:1", got.Source)
			}
			if !strings.Contains(got.Error(), "test.d2:1") {
				t.Errorf("Error() = %q, want it to carry the position", got.Error())
			}
		})
	}
}

// TestParseNotDirectives asserts the scanner keeps its hands off everything
// that is not a directive comment. It never parses D2 syntax.
func TestParseNotDirectives(t *testing.T) {
	lines := []string{
		"# just a note",
		"#",
		"##",
		"# TODO: bind this later",
		"",
		"   ",
		"svc_billing: Billing Service {",
		"  shape: rectangle",
		"}",
		"tenant -> platform.svc_work_orders: submits request",
		"  tooltip: app/services/work_orders",
		// A trailing comment is not a directive: a directive owns its line.
		"svc_billing: Billing # @bind svc_billing app/**",
		// An email address in prose must not be mistaken for a directive.
		"# ping ops@example.com about this box",
	}
	for _, line := range lines {
		t.Run(line, func(t *testing.T) {
			res := parseOne(t, line)
			if len(res.Directives) != 0 || len(res.Syntax) != 0 {
				t.Errorf("line %q produced %+v, want nothing", line, res)
			}
		})
	}
}

func TestParsePositions(t *testing.T) {
	src := "" +
		"# header\n" + // 1
		"# @bind a a/**\n" + // 2
		"\n" + // 3
		"# @bnid b b/**\n" + // 4
		"a: A\n" + // 5
		"# @infra db\r\n" + // 6, CRLF
		"# @bind c\n" // 7

	res := directive.Parse("docs/architecture/system.d2", []byte(src))

	wantDirectives := []struct {
		line int
		node string
	}{{2, "a"}, {6, "db"}}
	if len(res.Directives) != len(wantDirectives) {
		t.Fatalf("got %d directives, want %d: %+v", len(res.Directives), len(wantDirectives), res.Directives)
	}
	for i, w := range wantDirectives {
		got := res.Directives[i]
		if got.Source.Line != w.line || got.Node != w.node {
			t.Errorf("directive %d = %s at line %d, want %s at line %d", i, got.Node, got.Source.Line, w.node, w.line)
		}
		if got.Source.File != "docs/architecture/system.d2" {
			t.Errorf("directive %d file = %q", i, got.Source.File)
		}
	}
	if strings.HasSuffix(res.Directives[1].Raw, "\r") {
		t.Errorf("CRLF not normalized: raw = %q", res.Directives[1].Raw)
	}

	wantSyntax := []int{4, 7}
	if len(res.Syntax) != len(wantSyntax) {
		t.Fatalf("got %d syntax errors, want %d: %+v", len(res.Syntax), len(wantSyntax), res.Syntax)
	}
	for i, line := range wantSyntax {
		if res.Syntax[i].Source.Line != line {
			t.Errorf("syntax %d at line %d, want %d", i, res.Syntax[i].Source.Line, line)
		}
	}
}

// A malformed directive must not discard the well-formed ones around it: a
// broken binding can never be allowed to take a diagram down with it.
func TestSyntaxErrorDoesNotDiscardValidDirectives(t *testing.T) {
	src := "# @bind a a/**\n# @bind\n# @bind b b/**\n"
	res := directive.Parse("x.d2", []byte(src))
	if len(res.Directives) != 2 {
		t.Fatalf("got %d directives, want 2", len(res.Directives))
	}
	if len(res.Syntax) != 1 {
		t.Fatalf("got %d syntax errors, want 1", len(res.Syntax))
	}
}

func TestRepeatedBindIsLegal(t *testing.T) {
	src := "# @bind svc_billing app/services/billing/**\n# @bind svc_billing app/jobs/billing/**\n"
	res := directive.Parse("x.d2", []byte(src))
	if len(res.Syntax) != 0 {
		t.Fatalf("unexpected syntax errors: %v", res.Syntax)
	}
	binds := res.OfKind(directive.KindBind)
	if len(binds) != 2 {
		t.Fatalf("got %d binds, want 2", len(binds))
	}
	if binds[0].Node != binds[1].Node {
		t.Errorf("expected both binds on the same node, got %q and %q", binds[0].Node, binds[1].Node)
	}
}

func TestResultHelpers(t *testing.T) {
	res := directive.Parse("x.d2", []byte("# @bind a a/**\n# @infra db\n"))
	if got := res.Count(directive.KindBind); got != 1 {
		t.Errorf("Count(@bind) = %d, want 1", got)
	}
	if got := res.Count(directive.KindIgnore); got != 0 {
		t.Errorf("Count(@ignore) = %d, want 0", got)
	}
	var merged directive.Result
	merged.Merge(res)
	merged.Merge(res)
	if len(merged.Directives) != 4 {
		t.Errorf("Merge produced %d directives, want 4", len(merged.Directives))
	}
}

func TestDirectiveString(t *testing.T) {
	res := directive.Parse("x.d2", []byte(
		"# @bind a a/**\n# @external e\n# @infra i\n# @ignore g \"because\"\n"))
	want := []string{`@bind a a/**`, `@external e`, `@infra i`, `@ignore g "because"`}
	for i, d := range res.Directives {
		if d.String() != want[i] {
			t.Errorf("String() = %q, want %q", d.String(), want[i])
		}
	}
}

func TestPositionString(t *testing.T) {
	if got := (directive.Position{File: "a.d2", Line: 7}).String(); got != "a.d2:7" {
		t.Errorf("got %q", got)
	}
	if got := (directive.Position{Line: 7}).String(); got != "line 7" {
		t.Errorf("got %q", got)
	}
}

func TestFormCoversEveryKind(t *testing.T) {
	for _, k := range directive.Kinds {
		if directive.Form(k) == "" {
			t.Errorf("Form(%s) is empty", k)
		}
	}
	if directive.Form(directive.Kind("@bnid")) != "" {
		t.Error("Form of an unknown kind should be empty")
	}
}

// Acceptance: docs/DESIGN.md §2 — the worked example yields exactly
// 6 binds, 2 external, 2 infra, 1 ignore, and no syntax errors.
func TestWorkedExample(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "repairs-platform", "system.d2")
	res, err := directive.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(res.Syntax) != 0 {
		t.Fatalf("worked example has syntax errors: %v", res.Syntax)
	}
	want := map[directive.Kind]int{
		directive.KindBind:     6,
		directive.KindExternal: 2,
		directive.KindInfra:    2,
		directive.KindIgnore:   1,
	}
	for k, n := range want {
		if got := res.Count(k); got != n {
			t.Errorf("%s: got %d, want %d", k, got, n)
		}
	}
	if len(res.Directives) != 11 {
		t.Errorf("got %d directives total, want 11", len(res.Directives))
	}
	// Every directive carries its source. Without it the hints are useless.
	for _, d := range res.Directives {
		if d.Source.File != path || d.Source.Line == 0 {
			t.Errorf("directive %s has no usable source: %v", d.Node, d.Source)
		}
	}
	// The `# @bind svc_work_orders app/models/work_order*.rb` line proves globs
	// are not required to end in `/**`.
	var globs []string
	for _, d := range res.OfKind(directive.KindBind) {
		if d.Node == "svc_work_orders" {
			globs = append(globs, d.Glob)
		}
	}
	if len(globs) != 2 {
		t.Fatalf("svc_work_orders has %d binds, want 2 (ORed)", len(globs))
	}
}

func TestParseFileMissing(t *testing.T) {
	if _, err := directive.ParseFile(filepath.Join(t.TempDir(), "nope.d2")); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}

// A scoped package name opens a comment; it is not a mistyped directive.
//
// `# @astrojs/language-server is a devDependency` used to fail the check with
// `unknown directive`. In an npm repo `@types/`, `@babel/` and every workspace
// scope start a sentence the same way, so this fired on ordinary prose in
// essentially every JS/TS repo — found on a real one.
func TestScopedPackageNamesAreProse(t *testing.T) {
	for _, line := range []string{
		"# @astrojs/language-server is a devDependency bundled at build time",
		"# @types/node is only needed for the CLI",
		"## @babel/core does the transform",
		"#   @scope/pkg/deep is still prose",
	} {
		res := directive.Parse("system.d2", []byte(line))
		if len(res.Syntax) != 0 {
			t.Errorf("%q reported SYNTAX: %v", line, res.Syntax[0].Detail)
		}
		if len(res.Directives) != 0 {
			t.Errorf("%q parsed as a directive", line)
		}
	}
}

// The discriminator is a slash, not "anything unrecognized". A genuine typo has
// no slash and must still be reported — that is the case the unknown-directive
// error exists for, and silently ignoring it would let a misspelled @bind
// disappear.
func TestTyposAreStillReported(t *testing.T) {
	for _, line := range []string{
		"# @bnid svc_billing app/services/billing/**",
		"# @externl ext_stripe",
		"# @infr db_primary",
	} {
		res := directive.Parse("system.d2", []byte(line))
		if len(res.Syntax) != 1 {
			t.Errorf("%q was not reported as SYNTAX", line)
		}
	}
}
