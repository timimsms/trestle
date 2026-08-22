package check

import (
	"strings"

	"github.com/timimsms/trestle/internal/config"
)

// Coverage reports how much of the repo the `discover:` rules actually watch.
//
// It exists because of a failure found on the first two real repos Trestle was
// ever pointed at, and it is the same failure both times: **a green check that
// inspected almost nothing, with nothing anywhere saying so.**
//
//   - A Go repo whose module sits one directory down got `discover: []` from
//     `init`. Zero rules, zero units, zero files watched — and `trestle check`
//     reported `0 failures, 0 warnings`, exit 0. A permanently passing badge
//     over no coverage at all.
//   - A Rails repo got one rule that happened to match `lib/*/`. Two bindings
//     later it was green while watching **27 of 600 tracked files**. The entire
//     architecture — the provider boundary its own docs call load-bearing, the
//     policy layer, the executor — sat outside the tool's field of view.
//
// Neither is a bug in the engine: it correctly reported nothing wrong with what
// it was asked to look at. The bug is that "what it was asked to look at" was
// invisible, so a 4%-coverage green is indistinguishable from a real one.
//
// This is the same principle as [DisabledCodes], and the same remedy: report
// the scope alongside the result. A green result must never be able to mean
// "nothing was looked at."
//
// It deliberately does not judge. There is no threshold and no warning
// severity, because Trestle cannot know what fraction of a repo *should* be
// architecture — a CLI with one package and a large testdata tree is honestly
// 5%. It states the number and lets the reader see that 27/600 looks wrong.
type Coverage struct {
	// Rules is the number of non-empty `discover:` rules configured.
	Rules int

	// Units is how many directories those rules matched.
	Units int

	// Files is how many non-excluded files live beneath those units.
	Files int

	// TotalFiles is every non-excluded file in the walk.
	TotalFiles int
}

// Watched reports whether UNMAPPED can fire at all.
//
// False means no `discover:` rule matched a single directory, so the half of
// Trestle that catches *new* code is inert. A clean run in that state is not
// evidence of anything.
func (c Coverage) Watched() bool { return c.Units > 0 }

// Percent is the share of non-excluded files under a discover unit, 0–100.
// Zero when the repo is empty, which keeps the caller free of a divide guard.
func (c Coverage) Percent() int {
	if c.TotalFiles == 0 {
		return 0
	}
	return c.Files * 100 / c.TotalFiles
}

// Measure computes coverage from the same listing and config the engine checks.
//
// Pure, like everything else in this package: it reads the listing it is given
// and touches no filesystem.
func Measure(files []Entry, cfg *config.Config) Coverage {
	cov := Coverage{}
	for _, e := range files {
		if !e.IsDir && !isPlaceholder(e.Path) {
			cov.TotalFiles++
		}
	}
	if cfg == nil {
		return cov
	}

	ix := newIndex(files)

	// A file can sit under two overlapping discover rules; counting it twice
	// would let coverage exceed 100% and quietly destroy the one number this
	// type exists to make trustworthy.
	counted := make([]bool, len(files))

	for _, rule := range cfg.Discover {
		if strings.TrimSpace(rule) == "" {
			continue
		}
		cov.Rules++
		ix.eachUnit(rule, func(i int, _ Entry) {
			cov.Units++
			lo, hi := ix.subtree(i)
			for j := lo; j < hi; j++ {
				if ix.entries[j].IsDir || counted[j] || isPlaceholder(ix.entries[j].Path) {
					continue
				}
				counted[j] = true
				cov.Files++
			}
		})
	}
	return cov
}
