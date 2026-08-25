package check

import (
	"strings"
	"testing"
)

// The UNBOUND hint used to close with "for a database or queue the answer is
// usually `@infra`" on every node — including one named `ext_stripe`, where the
// answer is plainly `@external`. Generic advice attached to a specific name
// reads as though it were about that name.
//
// CONVENTIONS asks for prefix-by-kind, so the ID usually already says what the
// node is. The hint reads it.
func TestUnboundHintReadsTheIDPrefix(t *testing.T) {
	for _, tc := range []struct{ id, want string }{
		{"ext_stripe", "@external"},
		{"platform.ext_twilio", "@external"},
		{"db_primary", "@infra"},
		{"queue_dispatch", "@infra"},
		{"svc_billing", "@bind"},
		{"job_sla_monitor", "@bind"},
	} {
		got := unboundHint(tc.id)
		tail := got[strings.LastIndex(got, "—"):]
		if !strings.Contains(tail, tc.want) {
			t.Errorf("%s: advice is %q, want it to point at %s", tc.id, tail, tc.want)
		}
		// All four directives stay listed; the prefix narrows the advice, it
		// does not remove the options.
		for _, d := range []string{"@bind", "@infra", "@external", "@ignore"} {
			if !strings.Contains(got, d) {
				t.Errorf("%s: hint no longer offers %s", tc.id, d)
			}
		}
	}
}

// An ID with no recognized prefix keeps the general advice rather than guessing.
func TestUnboundHintFallsBackForUnprefixedIDs(t *testing.T) {
	got := unboundHint("tenant")
	if !strings.Contains(got, "usually `@infra`") {
		t.Errorf("unprefixed ID lost the general advice: %s", got)
	}
}
