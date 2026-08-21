package expected

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Expectation
	}{
		{
			name: "clean",
			in: "exit: 0\n" +
				"violations: none\n" +
				"warnings: 0\n" +
				"strict_exit: 0\n",
			want: Expectation{Exit: 0, Warnings: 0, StrictExit: 0, HasStrictExit: true},
		},
		{
			name: "single violation with detail",
			in: "exit: 1\n" +
				"violations:\n" +
				"  ORPHAN  svc_billing  app/services/billing/**\n" +
				"warnings: 0\n" +
				"strict_exit: 1\n",
			want: Expectation{
				Exit: 1,
				Violations: []Violation{
					{Code: "ORPHAN", Target: "svc_billing", Detail: "app/services/billing/**"},
				},
				StrictExit: 1, HasStrictExit: true,
			},
		},
		{
			name: "violation with no detail",
			in: "exit: 1\n" +
				"violations:\n" +
				"  UNMAPPED  app/services/notifications/\n" +
				"warnings: 0\n",
			want: Expectation{
				Exit:       1,
				Violations: []Violation{{Code: "UNMAPPED", Target: "app/services/notifications/"}},
			},
		},
		{
			name: "detail keeps interior spaces, collapsed",
			in: "exit: 1\n" +
				"violations:\n" +
				"  SYNTAX  svc_search   ambiguous:  billing.svc_search   support.svc_search\n" +
				"warnings: 0\n",
			want: Expectation{
				Exit: 1,
				Violations: []Violation{{
					Code:   "SYNTAX",
					Target: "svc_search",
					Detail: "ambiguous: billing.svc_search support.svc_search",
				}},
			},
		},
		{
			name: "warning counted and listed",
			in: "exit: 0\n" +
				"violations:\n" +
				"  UNBOUND  queue_dispatch\n" +
				"warnings: 1\n" +
				"strict_exit: 1\n",
			want: Expectation{
				Exit:       0,
				Violations: []Violation{{Code: "UNBOUND", Target: "queue_dispatch"}},
				Warnings:   1, StrictExit: 1, HasStrictExit: true,
			},
		},
		{
			name: "multiple violations, block ends at flush-left key",
			in: "violations:\n" +
				"  SYNTAX  svc_search          @bind with no glob\n" +
				"  SYNTAX  svc_legacy_tickets  @ignore with no reason string\n" +
				"exit: 1\n" +
				"warnings: 0\n",
			want: Expectation{
				Exit: 1,
				Violations: []Violation{
					{Code: "SYNTAX", Target: "svc_search", Detail: "@bind with no glob"},
					{Code: "SYNTAX", Target: "svc_legacy_tickets", Detail: "@ignore with no reason string"},
				},
			},
		},
		{
			name: "comments and blank lines ignored",
			in: "# what this fixture is\n" +
				"\n" +
				"exit: 2\n" +
				"   # indented comment inside nothing\n" +
				"violations: none\n" +
				"warnings: 0\n",
			want: Expectation{Exit: 2},
		},
		{
			name: "tab indent is indent",
			in:   "exit: 1\nviolations:\n\tDANGLING\tsvc_invoicing\nwarnings: 0\n",
			want: Expectation{
				Exit:       1,
				Violations: []Violation{{Code: "DANGLING", Target: "svc_invoicing"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(strings.NewReader(tt.in))
			if err != nil {
				t.Fatalf("Parse: unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse =\n  %+v\nwant\n  %+v", got, tt.want)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantSub string
	}{
		{
			name:    "sixth violation code",
			in:      "exit: 1\nviolations:\n  OVERLAP  svc_billing\nwarnings: 0\n",
			wantSub: "unknown violation code",
		},
		{
			name:    "lowercase code",
			in:      "exit: 1\nviolations:\n  orphan  svc_billing\nwarnings: 0\n",
			wantSub: "unknown violation code",
		},
		{
			name:    "violation missing target",
			in:      "exit: 1\nviolations:\n  ORPHAN\nwarnings: 0\n",
			wantSub: "at least",
		},
		{
			name:    "unknown key",
			in:      "exit: 0\nviolations: none\nwarnings: 0\nexit_code: 0\n",
			wantSub: "unknown key",
		},
		{
			name:    "missing exit",
			in:      "violations: none\nwarnings: 0\n",
			wantSub: `missing required key "exit"`,
		},
		{
			name:    "missing violations",
			in:      "exit: 0\nwarnings: 0\n",
			wantSub: `missing required key "violations"`,
		},
		{
			name:    "missing warnings",
			in:      "exit: 0\nviolations: none\n",
			wantSub: `missing required key "warnings"`,
		},
		{
			name:    "exit out of range",
			in:      "exit: 3\nviolations: none\nwarnings: 0\n",
			wantSub: "must be 0, 1 or 2",
		},
		{
			name:    "exit not a number",
			in:      "exit: fail\nviolations: none\nwarnings: 0\n",
			wantSub: "not a number",
		},
		{
			name:    "strict_exit out of range",
			in:      "exit: 0\nviolations: none\nwarnings: 0\nstrict_exit: 9\n",
			wantSub: "must be 0, 1 or 2",
		},
		{
			name:    "duplicate key",
			in:      "exit: 0\nexit: 1\nviolations: none\nwarnings: 0\n",
			wantSub: "duplicate key",
		},
		{
			name:    "violations junk on header line",
			in:      "exit: 0\nviolations: []\nwarnings: 0\n",
			wantSub: "expected `none` or an indented list",
		},
		{
			name:    "warnings exceed listed violations",
			in:      "exit: 0\nviolations: none\nwarnings: 1\n",
			wantSub: "must also appear under",
		},
		{
			name:    "not a key value line",
			in:      "exit: 0\nviolations: none\nwarnings: 0\nnonsense\n",
			wantSub: "not a `key: value` line",
		},
		{
			name:    "flush-left violation is not in the block",
			in:      "exit: 1\nviolations:\nORPHAN  svc_billing\nwarnings: 0\n",
			wantSub: "not a `key: value` line",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(tt.in))
			if err == nil {
				t.Fatalf("Parse: want error containing %q, got nil", tt.wantSub)
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("Parse error = %q, want it to contain %q", err, tt.wantSub)
			}
		})
	}
}

func TestDiff(t *testing.T) {
	want := Expectation{
		Violations: []Violation{
			{Code: "ORPHAN", Target: "svc_billing", Detail: "app/services/billing/**"},
			{Code: "UNBOUND", Target: "queue_dispatch"},
		},
	}

	tests := []struct {
		name        string
		got         []Violation
		wantMissing []Violation
		wantExtra   []Violation
	}{
		{
			name: "exact match",
			got: []Violation{
				{Code: "UNBOUND", Target: "queue_dispatch"},
				{Code: "ORPHAN", Target: "svc_billing", Detail: "app/services/billing/**"},
			},
		},
		{
			name: "order independent with one missing",
			got:  []Violation{{Code: "UNBOUND", Target: "queue_dispatch"}},
			wantMissing: []Violation{
				{Code: "ORPHAN", Target: "svc_billing", Detail: "app/services/billing/**"},
			},
		},
		{
			name: "extra violation",
			got: []Violation{
				{Code: "ORPHAN", Target: "svc_billing", Detail: "app/services/billing/**"},
				{Code: "UNBOUND", Target: "queue_dispatch"},
				{Code: "DANGLING", Target: "svc_invoicing"},
			},
			wantExtra: []Violation{{Code: "DANGLING", Target: "svc_invoicing"}},
		},
		{
			name: "wrong detail counts as both missing and extra",
			got: []Violation{
				{Code: "ORPHAN", Target: "svc_billing", Detail: "app/services/bill/**"},
				{Code: "UNBOUND", Target: "queue_dispatch"},
			},
			wantMissing: []Violation{
				{Code: "ORPHAN", Target: "svc_billing", Detail: "app/services/billing/**"},
			},
			wantExtra: []Violation{
				{Code: "ORPHAN", Target: "svc_billing", Detail: "app/services/bill/**"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			missing, extra := want.Diff(tt.got)
			if !reflect.DeepEqual(missing, tt.wantMissing) {
				t.Errorf("missing = %v, want %v", missing, tt.wantMissing)
			}
			if !reflect.DeepEqual(extra, tt.wantExtra) {
				t.Errorf("extra = %v, want %v", extra, tt.wantExtra)
			}
		})
	}
}

func TestDiffCodesIgnoresDetail(t *testing.T) {
	want := Expectation{
		Violations: []Violation{{Code: "ORPHAN", Target: "svc_billing", Detail: "app/services/billing/**"}},
	}
	missing, extra := want.DiffCodes([]Violation{
		{Code: "ORPHAN", Target: "svc_billing", Detail: "matches 0 files"},
	})
	if len(missing) != 0 || len(extra) != 0 {
		t.Errorf("DiffCodes = missing %v, extra %v; want both empty", missing, extra)
	}
}

func TestCount(t *testing.T) {
	e := Expectation{Violations: []Violation{
		{Code: "SYNTAX", Target: "a"},
		{Code: "SYNTAX", Target: "b"},
		{Code: "ORPHAN", Target: "c"},
	}}
	if got := e.Count("SYNTAX"); got != 2 {
		t.Errorf("Count(SYNTAX) = %d, want 2", got)
	}
	if got := e.Count("UNBOUND"); got != 0 {
		t.Errorf("Count(UNBOUND) = %d, want 0", got)
	}
}

// reposDir is the fixture root relative to this package.
const reposDir = "../../testdata/repos"

// TestFixtures is the format's regression test: every committed fixture must
// parse, and the set of fixtures must not silently shrink.
func TestFixtures(t *testing.T) {
	want := map[string]Expectation{
		"clean":         {Exit: 0, Warnings: 0, StrictExit: 0, HasStrictExit: true},
		"orphan":        {Exit: 1, Warnings: 0, StrictExit: 1, HasStrictExit: true},
		"orphan_shared": {Exit: 1, Warnings: 0, StrictExit: 1, HasStrictExit: true},
		"unmapped":      {Exit: 1, Warnings: 0, StrictExit: 1, HasStrictExit: true},
		"dangling":      {Exit: 1, Warnings: 0, StrictExit: 1, HasStrictExit: true},
		"unbound":       {Exit: 0, Warnings: 1, StrictExit: 1, HasStrictExit: true},
		"overlap":       {Exit: 0, Warnings: 0, StrictExit: 0, HasStrictExit: true},
		"syntax":        {Exit: 1, Warnings: 0, StrictExit: 1, HasStrictExit: true},
		"nested":        {Exit: 0, Warnings: 0, StrictExit: 0, HasStrictExit: true},
		"ambiguous":     {Exit: 1, Warnings: 0, StrictExit: 1, HasStrictExit: true},
	}
	wantViolations := map[string][]Violation{
		"clean":         nil,
		"orphan":        {{Code: "ORPHAN", Target: "svc_reporting", Detail: "app/services/reporting/**"}},
		"orphan_shared": {{Code: "ORPHAN", Target: "lib/legacy_pdf/**", Detail: "shared"}},
		"unmapped":      {{Code: "UNMAPPED", Target: "app/services/notifications/"}},
		"dangling":      {{Code: "DANGLING", Target: "svc_invoicing", Detail: "app/legacy/invoicing/**"}},
		"unbound":       {{Code: "UNBOUND", Target: "queue_dispatch"}},
		"overlap":       nil,
		"syntax": {
			{Code: "SYNTAX", Target: "svc_search", Detail: "@bind with no glob"},
			{Code: "SYNTAX", Target: "svc_legacy_tickets", Detail: "@ignore with no reason string"},
		},
		"nested": nil,
		"ambiguous": {{
			Code:   "SYNTAX",
			Target: "svc_search",
			Detail: "ambiguous: billing.svc_search support.svc_search",
		}},
	}

	entries, err := os.ReadDir(reposDir)
	if err != nil {
		t.Fatalf("reading %s: %v", reposDir, err)
	}
	found := map[string]bool{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		found[entry.Name()] = true
	}
	for name := range want {
		if !found[name] {
			t.Errorf("fixture %q is missing from %s", name, reposDir)
		}
	}
	for name := range found {
		if _, ok := want[name]; !ok {
			t.Errorf("fixture %q exists on disk but is not declared in this test", name)
		}
	}

	for name, wantExp := range want {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(reposDir, name)
			got, err := Load(dir)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}

			wantExp.Violations = wantViolations[name]
			if !reflect.DeepEqual(got, wantExp) {
				t.Errorf("EXPECTED =\n  %+v\nwant\n  %+v", got, wantExp)
			}

			// Every fixture is a real tree with a config and a diagram.
			for _, rel := range []string{".trestle.yml", "docs/architecture/system.d2"} {
				if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
					t.Errorf("fixture %s: %v", name, err)
				}
			}
		})
	}
}

// TestUnboundStaysAWarning guards O3 specifically. UNBOUND defaults to `warn`
// and the fixture exits 0; `--strict` promotes it to 1. Getting this backwards
// is the documented common error, so it gets its own test rather than living
// inside a table.
func TestUnboundStaysAWarning(t *testing.T) {
	e, err := Load(filepath.Join(reposDir, "unbound"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if e.Exit != 0 {
		t.Errorf("unbound/ exit = %d, want 0 (O3: UNBOUND defaults to warn)", e.Exit)
	}
	if e.Warnings != 1 {
		t.Errorf("unbound/ warnings = %d, want 1", e.Warnings)
	}
	if e.Count("UNBOUND") != 1 {
		t.Errorf("unbound/ UNBOUND count = %d, want 1", e.Count("UNBOUND"))
	}
	if !e.HasStrictExit || e.StrictExit != 1 {
		t.Errorf("unbound/ strict_exit = %d (set=%v), want 1 (--strict promotes warnings)",
			e.StrictExit, e.HasStrictExit)
	}
}
