package walk

import (
	"fmt"
	"testing"

	"github.com/bmatcuk/doublestar/v4"
)

// The matcher rewrites common exclude shapes into cheaper equivalents. Cheaper
// is worthless if it is not identical: a pattern that quietly stops matching
// turns vendored code into architecture, and one that starts matching hides a
// real subsystem. This test is the proof, run over the cross product of every
// pattern shape the rewrites recognize and a corpus of awkward paths.
func TestMatcherMatchesDoublestar(t *testing.T) {
	patterns := []string{
		// literal
		"node_modules", "vendor", "a/b/c.go", "tmp",
		// **/x
		"**/vendor", "**/*_test.*", "**/*.min.js", "**/node_modules", "**/*",
		// **/x/**
		"**/vendor/**", "**/node_modules/**", "**/*_gen/**",
		// x/**
		"vendor/**", "app/services/**", "*/vendor/**",
		// no rewrite
		"app/**/*.rb", "**", "app/*/models", "**/{a,b}/**", "a[bc]d",
	}
	paths := []string{
		"vendor",
		"vendor/x.go",
		"vendor/a/b/x.go",
		"a/vendor",
		"a/vendor/x.go",
		"a/b/vendor/x.go",
		"node_modules",
		"node_modules/left-pad/index.js",
		"app",
		"app/services",
		"app/services/billing",
		"app/services/billing/charge.rb",
		"app/services/billing/charge_test.rb",
		"charge_test.rb",
		"a/b/c.go",
		"a/b/c.min.js",
		"pkg_gen",
		"pkg_gen/out.go",
		"a/pkg_gen/out.go",
		"vendored",
		"vendorx/y",
		"a",
		"a/b",
		"abd",
		"a/models",
		"app/x/models",
	}

	for _, pat := range patterns {
		m, err := newMatcher([]string{pat})
		if err != nil {
			t.Fatalf("newMatcher(%q): %v", pat, err)
		}
		for _, p := range paths {
			want, err := doublestar.Match(pat, p)
			if err != nil {
				t.Fatalf("doublestar.Match(%q, %q): %v", pat, p, err)
			}
			if got := m.match(p); got != want {
				t.Errorf("pattern %q (kind %d) path %q: matcher = %v, doublestar = %v",
					pat, m.pats[0].kind, p, got, want)
			}
		}
	}
}

// The rewrites only pay off if they actually fire on the shapes people write.
// If a refactor drops one to kindGlob the walk silently gets slower, which is
// exactly the kind of regression that never gets noticed.
func TestMatcherPicksExpectedKind(t *testing.T) {
	tests := []struct {
		pattern string
		want    patternKind
	}{
		{"node_modules", kindLiteral},
		{"node_modules/", kindLiteral}, // trailing slash trimmed
		{"docs/architecture", kindLiteral},
		{"**/vendor", kindBaseLiteral},
		{"**/*_test.*", kindBaseGlob},
		{"**/*.min.js", kindBaseGlob},
		{"**/vendor/**", kindSegmentLiteral},
		{"**/node_modules/**", kindSegmentLiteral},
		{"vendor/**", kindPrefixLiteral},
		{"app/services/**", kindPrefixLiteral},
		{"**/*_gen/**", kindGlob}, // inner is not literal
		{"app/**/*.rb", kindGlob}, // embedded **
		{"**", kindGlob},          // degenerate
		{"*/vendor/**", kindGlob}, // prefix is not literal
		{"**/a/b", kindGlob},      // inner contains a separator
		{"a/b/c.go", kindLiteral}, //
		{"**/{a,b}/**", kindGlob}, // brace expansion
	}
	for _, tc := range tests {
		t.Run(tc.pattern, func(t *testing.T) {
			m, err := newMatcher([]string{tc.pattern})
			if err != nil {
				t.Fatalf("newMatcher: %v", err)
			}
			if len(m.pats) != 1 {
				t.Fatalf("got %d compiled patterns, want 1", len(m.pats))
			}
			if got := m.pats[0].kind; got != tc.want {
				t.Errorf("kind = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestHasSegment(t *testing.T) {
	tests := []struct {
		path, seg string
		want      bool
	}{
		{"vendor", "vendor", true},
		{"vendor/a", "vendor", true},
		{"a/vendor", "vendor", true},
		{"a/vendor/b", "vendor", true},
		{"vendored", "vendor", false},
		{"a/vendored/b", "vendor", false},
		{"xvendor", "vendor", false},
		{"", "vendor", false},
		{"a/b/c", "b", true},
		{"a/b/c", "c", true},
		{"a/b/c", "a", true},
	}
	for _, tc := range tests {
		if got := hasSegment(tc.path, tc.seg); got != tc.want {
			t.Errorf("hasSegment(%q, %q) = %v, want %v", tc.path, tc.seg, got, tc.want)
		}
	}
}

func TestEmptyMatcherMatchesNothing(t *testing.T) {
	m, err := newMatcher(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"", "a", "a/b", "node_modules"} {
		if m.match(p) {
			t.Errorf("empty matcher matched %q", p)
		}
	}
}

func BenchmarkMatcher(b *testing.B) {
	paths := make([]string, 0, 1000)
	for i := 0; i < 1000; i++ {
		paths = append(paths, fmt.Sprintf("app/services/group%02d/unit%03d/file%03d.go", i/50, i/10, i))
	}
	pats := []string{"node_modules", "**/*_test.*", "**/vendor/**", "**/*.min.js",
		"**/dist/**", "**/*.generated.*", "tmp/**", "**/*.snap"}

	for _, n := range []int{1, 4, 8} {
		b.Run(fmt.Sprintf("compiled/patterns=%d", n), func(b *testing.B) {
			m, err := newMatcher(pats[:n])
			if err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for _, p := range paths {
					m.match(p)
				}
			}
		})
		b.Run(fmt.Sprintf("doublestar/patterns=%d", n), func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for _, p := range paths {
					for _, pat := range pats[:n] {
						if ok, _ := doublestar.Match(pat, p); ok {
							break
						}
					}
				}
			}
		})
	}
}
