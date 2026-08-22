package lang

import "testing"

func has(names ...string) func(string) bool {
	set := map[string]bool{}
	for _, n := range names {
		set[n] = true
	}
	return func(n string) bool { return set[n] }
}

// Marker gating is a correctness fix, not tidying: without it a Ruby repo that
// happens to hold a `cmd/` directory is offered `cmd/*/`, and the author has to
// work out the tool guessed a language wrong before dismissing the rule.
func TestDetectedGatesOnMarkers(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files []string
		want  []string
	}{
		{"go", []string{"go.mod"}, []string{"Go"}},
		{"rails", []string{"Gemfile"}, []string{"Ruby on Rails"}},
		{"node", []string{"package.json"}, []string{"JavaScript / TypeScript"}},
		{"rails with a js build", []string{"Gemfile", "package.json"},
			[]string{"Ruby on Rails", "JavaScript / TypeScript"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Detected(has(tc.files...))
			if len(got) != len(tc.want) {
				t.Fatalf("detected %d languages, want %d: %v", len(got), len(tc.want), got)
			}
			for i, l := range got {
				if l.Name != tc.want[i] {
					t.Errorf("[%d] = %q, want %q", i, l.Name, tc.want[i])
				}
			}
		})
	}
}

// No marker is not the same as no layout, and proposing a shape that turns out
// not to match costs nothing — a rule is only ever offered when it matches a
// directory with files in it.
func TestDetectedFallsBackToEverything(t *testing.T) {
	if got := Detected(has()); len(got) != len(All) {
		t.Errorf("detected %d languages with no markers, want all %d", len(got), len(All))
	}
}

// The prefixes are Rails vocabulary. On a Go repo they were actively wrong:
// `svc_db` for a package named `db` appears nowhere in the repo, and `db_` is
// what CONVENTIONS reserves for datastores, so the collision is worse than the
// redundancy.
func TestPrefixesAreRailsOnly(t *testing.T) {
	if got := Prefix([]Lang{Rails}, "services"); got != "svc_" {
		t.Errorf("Rails services prefix = %q, want svc_", got)
	}
	if got := Prefix([]Lang{Go}, "services"); got != "" {
		t.Errorf("Go contributed the prefix %q; Go package names are already the identifier", got)
	}
	if got := Prefix([]Lang{Node}, "packages"); got != "" {
		t.Errorf("Node contributed the prefix %q", got)
	}
}

// Where shapes nest, the specific one must come first so it claims its
// directories before the general one sees them. Rails is the case: a repo with
// app/services/billing/ must get app/services/*/ before app/*/, or it is
// offered both `app/services` and `app/services/billing` as units and the outer
// one can never be satisfied.
func TestRailsOrdersSpecificShapesFirst(t *testing.T) {
	specific, general := -1, -1
	for i, g := range Rails.Discover {
		switch g {
		case "app/services/*/":
			specific = i
		case "app/*/":
			general = i
		}
	}
	if specific < 0 || general < 0 {
		t.Fatalf("Rails.Discover is missing a shape: %v", Rails.Discover)
	}
	if specific > general {
		t.Errorf("app/*/ precedes app/services/*/; the general shape would claim the container")
	}
}

// Every language must be usable: a Lang with no shapes proposes nothing and a
// Lang with no markers can never be detected except by fallback.
func TestEveryLangIsUsable(t *testing.T) {
	for _, l := range All {
		if l.Name == "" {
			t.Error("a Lang has no name")
		}
		if len(l.Markers) == 0 {
			t.Errorf("%s has no marker files, so it can only ever be detected by fallback", l.Name)
		}
		if len(l.Discover) == 0 {
			t.Errorf("%s proposes no shapes", l.Name)
		}
	}
}
