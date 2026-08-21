package nodes

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// examplePath is the worked example. It is a live test input, not a doc.
const examplePath = "../../examples/repairs-platform/system.d2"

// TestVersionCanary pins the d2 v0.7.2 AST behavior that everything downstream
// assumes. Gate B established these exact numbers by hand; this test is what
// tells you a d2 upgrade moved the AST, instead of a fixture failing three
// phases later with no obvious cause.
//
// If this test fails after a dependency bump, the correct response is to
// re-run Gate B, not to adjust the numbers.
func TestVersionCanary(t *testing.T) {
	d := parseExample(t)

	if got := d.Len(); got != 12 {
		t.Fatalf("node count = %d, want 12; d2 AST behavior changed\ngot: %v", got, d.IDs)
	}

	wantIDs := []string{
		"tenant",
		"platform",
		"platform.svc_work_orders",
		"platform.svc_dispatch",
		"platform.svc_vendor_registry",
		"platform.svc_notifications",
		"platform.job_sla_monitor",
		"platform.svc_legacy_tickets",
		"db_primary",
		"queue_dispatch",
		"ext_lula",
		"ext_twilio",
	}
	if !reflect.DeepEqual(d.IDs, wantIDs) {
		t.Errorf("IDs (declaration order) mismatch\n got: %v\nwant: %v", d.IDs, wantIDs)
	}

	// The six platform.* IDs must keep their container qualification. Losing it
	// is the failure mode O8 exists to handle, and it must be visible here.
	var qualified int
	for _, id := range d.IDs {
		if strings.HasPrefix(id, "platform.") {
			qualified++
		}
	}
	if qualified != 6 {
		t.Errorf("platform.* node count = %d, want 6 (container qualification lost?)", qualified)
	}

	// D2 keywords must not become nodes: `vars` and `direction` are set in the
	// example and neither is a box.
	for _, notANode := range []string{"vars", "direction", "d2-config"} {
		if d.Has(notANode) {
			t.Errorf("%q surfaced as a node; d2 keyword handling changed", notANode)
		}
	}

	if len(d.Roots) != 6 {
		t.Errorf("roots = %d (%v), want 6", len(d.Roots), d.Roots)
	}
}

func TestCanaryContainerRelation(t *testing.T) {
	d := parseExample(t)

	platform, ok := d.Node("platform")
	if !ok {
		t.Fatal("platform missing")
	}
	if !platform.IsContainer() {
		t.Error("platform should be a container")
	}
	if got := len(platform.Children); got != 6 {
		t.Errorf("platform children = %d, want 6", got)
	}
	if got := len(d.Descendants("platform")); got != 6 {
		t.Errorf("platform descendants = %d, want 6", got)
	}

	tenant, ok := d.Node("tenant")
	if !ok {
		t.Fatal("tenant missing")
	}
	if tenant.IsContainer() {
		t.Error("tenant is a leaf; O9 must not suppress its UNBOUND")
	}
	if tenant.Parent != "" {
		t.Errorf("tenant.Parent = %q, want \"\"", tenant.Parent)
	}

	parents := d.Parents()
	if len(parents) != d.Len() {
		t.Fatalf("Parents() has %d entries, want %d", len(parents), d.Len())
	}
	if got := parents["platform.svc_work_orders"]; got != "platform" {
		t.Errorf("parent of platform.svc_work_orders = %q, want %q", got, "platform")
	}
	if got := parents["db_primary"]; got != "" {
		t.Errorf("parent of db_primary = %q, want \"\"", got)
	}
}

func TestCanaryNodeDetail(t *testing.T) {
	d := parseExample(t)

	n, ok := d.Node("platform.svc_work_orders")
	if !ok {
		t.Fatal("platform.svc_work_orders missing")
	}
	if n.Name != "svc_work_orders" {
		t.Errorf("Name = %q, want %q", n.Name, "svc_work_orders")
	}
	if n.Label != "Work Orders" {
		t.Errorf("Label = %q, want %q", n.Label, "Work Orders")
	}
	// 1-based, and the declaration is on line 30 of the example.
	if n.Line != 30 {
		t.Errorf("Line = %d, want 30 (1-based source line)", n.Line)
	}

	if got := mustNode(t, d, "db_primary").Shape; got != "cylinder" {
		t.Errorf("db_primary shape = %q, want cylinder", got)
	}
	if got := mustNode(t, d, "queue_dispatch").Shape; got != "queue" {
		t.Errorf("queue_dispatch shape = %q, want queue", got)
	}
}

// TestEveryExampleDirectiveResolves is the O8 acceptance in miniature: every
// node ID written unqualified in the shipped example must resolve to exactly
// one node. If it does not, the reference diagram fails its own check.
func TestEveryExampleDirectiveResolves(t *testing.T) {
	d := parseExample(t)

	directiveIDs := []string{
		"svc_work_orders", "svc_dispatch", "svc_vendor_registry",
		"svc_notifications", "job_sla_monitor", "svc_legacy_tickets",
		"db_primary", "queue_dispatch", "ext_lula", "ext_twilio",
	}
	for _, id := range directiveIDs {
		got := d.Candidates(id)
		if len(got) != 1 {
			t.Errorf("Candidates(%q) = %v, want exactly one match", id, got)
		}
	}
}

func TestCandidates(t *testing.T) {
	src := `
platform: {
  svc_orders
  billing: {
    svc_orders
  }
}
svc_unique
`
	d, err := Parse("t.d2", []byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	tests := []struct {
		name string
		id   string
		want []string
	}{
		{"exact fully-qualified", "platform.svc_orders", []string{"platform.svc_orders"}},
		{"exact wins over suffix", "platform.billing.svc_orders", []string{"platform.billing.svc_orders"}},
		{"unique suffix resolves", "svc_unique", []string{"svc_unique"}},
		{"multi-segment suffix", "billing.svc_orders", []string{"platform.billing.svc_orders"}},
		{"ambiguous suffix returns all", "svc_orders", []string{"platform.svc_orders", "platform.billing.svc_orders"}},
		{"container is a candidate", "billing", []string{"platform.billing"}},
		{"substring is not a suffix", "orders", nil},
		{"unknown", "nope", nil},
		{"empty", "", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := d.Candidates(tc.id)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Candidates(%q) = %v, want %v", tc.id, got, tc.want)
			}
		})
	}
}

func TestNestedContainers(t *testing.T) {
	src := `
a: {
  b: {
    c: {
      d
    }
  }
  e
}
`
	d, err := Parse("t.d2", []byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	want := []string{"a", "a.b", "a.b.c", "a.b.c.d", "a.e"}
	if !reflect.DeepEqual(d.IDs, want) {
		t.Fatalf("IDs = %v, want %v", d.IDs, want)
	}
	if got := d.Descendants("a"); !reflect.DeepEqual(got, []string{"a.b", "a.b.c", "a.b.c.d", "a.e"}) {
		t.Errorf("Descendants(a) = %v", got)
	}
	if got := d.Descendants("a.b.c.d"); got != nil {
		t.Errorf("Descendants of a leaf = %v, want nil", got)
	}
	if got := d.Descendants("missing"); got != nil {
		t.Errorf("Descendants of unknown id = %v, want nil", got)
	}
	if got := d.Children("a.b"); !reflect.DeepEqual(got, []string{"a.b.c"}) {
		t.Errorf("Children(a.b) = %v", got)
	}
}

// Nodes that exist only because an edge referenced them still have to appear;
// otherwise a legitimately-bound node vanishes when its only declaration is a
// connection.
func TestEdgeImpliedNodes(t *testing.T) {
	d, err := Parse("t.d2", []byte("x.y -> z\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []string{"x", "x.y", "z"}
	if !reflect.DeepEqual(d.IDs, want) {
		t.Errorf("IDs = %v, want %v", d.IDs, want)
	}
}

func TestEmptyDiagram(t *testing.T) {
	d, err := Parse("empty.d2", []byte("# only a comment\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if d.Len() != 0 {
		t.Errorf("Len = %d, want 0", d.Len())
	}
	if d.Has("anything") {
		t.Error("Has returned true on an empty diagram")
	}
	if got := d.Parents(); len(got) != 0 {
		t.Errorf("Parents = %v, want empty", got)
	}
}

// A diagram that does not compile is a tool error (exit 2), never a violation.
// "Trestle is broken" and "your diagram is wrong" must stay distinguishable, and
// this is the seam where that distinction is created.
func TestCompileErrorIsToolError(t *testing.T) {
	_, err := Parse("bad.d2", []byte("a -> \nb: {{{\n"))
	if err == nil {
		t.Fatal("expected an error from a malformed diagram")
	}
	if !errors.Is(err, ErrCompile) {
		t.Errorf("errors.Is(err, ErrCompile) = false; err = %v", err)
	}

	var ce *CompileError
	if !errors.As(err, &ce) {
		t.Fatalf("errors.As(*CompileError) = false; err type %T", err)
	}
	if ce.Path != "bad.d2" {
		t.Errorf("Path = %q, want bad.d2", ce.Path)
	}
	if len(ce.Diagnostics) == 0 {
		t.Fatal("no diagnostics; the CLI would have nothing to print")
	}
	for _, d := range ce.Diagnostics {
		if d.Line < 1 {
			t.Errorf("diagnostic has no line: %+v", d)
		}
		if strings.Contains(d.Message, "bad.d2:") {
			t.Errorf("position prefix leaked into Message: %q", d.Message)
		}
		if !strings.HasPrefix(d.String(), "bad.d2:") {
			t.Errorf("String() lost its position: %q", d.String())
		}
	}
	if ce.Unwrap() == nil {
		t.Error("Unwrap returned nil; the d2 error was dropped")
	}
}

func TestParseFile(t *testing.T) {
	d, err := ParseFile(examplePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if d.Len() != 12 {
		t.Errorf("Len = %d, want 12", d.Len())
	}
	if d.Path != examplePath {
		t.Errorf("Path = %q, want %q", d.Path, examplePath)
	}

	if _, err := ParseFile(filepath.Join(t.TempDir(), "nope.d2")); err == nil {
		t.Error("expected an error for a missing file")
	} else if errors.Is(err, ErrCompile) {
		t.Error("a missing file must not be reported as a compile error")
	}
}

// Order must come from the AST, not from map iteration, or violation output
// reorders itself between runs and golden files become unusable.
func TestDeterministicOrder(t *testing.T) {
	src, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatal(err)
	}
	first, err := Parse(examplePath, src)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		d, err := Parse(examplePath, src)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(d.IDs, first.IDs) {
			t.Fatalf("run %d produced a different order\n got: %v\nwant: %v", i, d.IDs, first.IDs)
		}
	}
}

func parseExample(t *testing.T) *Diagram {
	t.Helper()
	src, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatalf("read worked example: %v", err)
	}
	d, err := Parse(examplePath, src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return d
}

func mustNode(t *testing.T, d *Diagram, id string) *Node {
	t.Helper()
	n, ok := d.Node(id)
	if !ok {
		t.Fatalf("node %q missing", id)
	}
	return n
}
