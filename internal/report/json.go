package report

import (
	"encoding/json"
	"io"

	"github.com/timimsms/trestle/internal/check"
)

// document is the `--format=json` payload. Its shape is fixed by PHASE_4
// §"JSON output" and versioned by [SchemaVersion].
//
// Field-level notes, because each one is a decision:
//
//   - severity is the violation's own severity, always. `--strict` appears as
//     the sibling `strict` flag and never rewrites this field, so a consumer can
//     still tell a warning from a failure in a strict run.
//   - node and path are null rather than "" when absent. A violation is about a
//     node or about a path, and an empty string reads as "the node named
//     nothing".
//   - violations is always an array, never null, so a clean run and a failing
//     run have the same shape.
type document struct {
	Version int         `json:"version"`
	Strict  bool        `json:"strict"`
	Summary summaryJSON `json:"summary"`
	// Disabled names codes set to `off` in config. Always present, `[]` when
	// none, so a consumer can tell "nothing disabled" from an older payload
	// that could not say. A CI gate reading `summary.failures == 0` without
	// checking this field is trusting a check that may have inspected nothing.
	Disabled []string `json:"disabled"`
	// Coverage is how much of the repo `discover:` watches. Always present, so
	// a CI gate reading `summary.failures == 0` can tell a real green from one
	// produced by watching nothing.
	Coverage   coverageJSON    `json:"coverage"`
	Violations []violationJSON `json:"violations"`
}

type coverageJSON struct {
	Rules      int `json:"rules"`
	Units      int `json:"units"`
	Files      int `json:"files"`
	TotalFiles int `json:"total_files"`
}

type summaryJSON struct {
	Failures int `json:"failures"`
	Warnings int `json:"warnings"`
}

type violationJSON struct {
	Code     string     `json:"code"`
	Severity string     `json:"severity"`
	Node     *string    `json:"node"`
	Path     *string    `json:"path"`
	Source   sourceJSON `json:"source"`
	Detail   string     `json:"detail"`
	Hint     string     `json:"hint"`
}

// sourceJSON locates the violation. Line is 0 when the source has no line —
// which is every config-sourced violation, since `discover:` and `shared:`
// entries reach the engine without their line numbers. It stays an integer
// rather than becoming null so the field's type never varies.
type sourceJSON struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

func writeJSON(w io.Writer, vs []check.Violation, opt Options) error {
	doc := document{
		Version:  SchemaVersion,
		Strict:   opt.Strict,
		Summary:  summaryJSON(Summarize(vs)),
		Disabled: make([]string, 0, len(opt.Disabled)),
		Coverage: coverageJSON{
			Rules:      opt.Coverage.Rules,
			Units:      opt.Coverage.Units,
			Files:      opt.Coverage.Files,
			TotalFiles: opt.Coverage.TotalFiles,
		},
		Violations: make([]violationJSON, 0, len(vs)),
	}
	for _, c := range opt.Disabled {
		doc.Disabled = append(doc.Disabled, string(c))
	}
	for _, v := range vs {
		doc.Violations = append(doc.Violations, violationJSON{
			Code:     string(v.Code),
			Severity: string(v.Severity),
			// Node is whatever the engine resolved, which for every directive
			// that resolved at all is the fully-qualified AST ID — a directive
			// may write `svc_billing` and this field still says
			// `platform.svc_billing`. The human format may echo what the author
			// typed; the machine format has to be unambiguous.
			Node:   optional(v.Node),
			Path:   optional(v.Path),
			Source: sourceJSON{File: v.Source.File, Line: v.Source.Line},
			Detail: v.Detail,
			Hint:   v.Hint,
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(doc)
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
