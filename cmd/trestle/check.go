package main

import (
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/timimsms/trestle/internal/check"
	"github.com/timimsms/trestle/internal/report"
	"github.com/timimsms/trestle/internal/run"
)

// newCheckCmd builds `trestle check`.
//
// The order of operations is PHASE_4 §"Command wiring" and every step's failure
// is exit 2: find `.trestle.yml` by walking up from CWD, load and validate it,
// walk the repo once, resolve `diagrams:` against that one listing, parse each
// diagram, run the engine, format, exit. `internal/run` performs the first five
// so that the loop is written once for the commands that come after this one.
func newCheckCmd(stdout io.Writer, exit *int) *cobra.Command {
	var (
		format string
		strict bool
	)

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Validate bindings against the repo",
		Long: "Validate every @bind, @external, @infra and @ignore directive against the\n" +
			"files actually on disk, and every `discover:` rule against the diagrams.\n\n" +
			"--strict promotes warnings to failures for the exit code only; it does not\n" +
			"change the severity a violation reports in --format=json.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// From here on the error is about the repo, not about how the
			// command was typed, and dumping usage would bury it.
			cmd.SilenceUsage = true

			f, err := report.ParseFormat(format)
			if err != nil {
				return err
			}

			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			ctx, err := run.Load(cwd)
			if err != nil {
				return err
			}

			violations := ctx.Check()
			opt := report.Options{
				Root:   ctx.Config.Root,
				Strict: strict,
				Color:  f == report.FormatHuman && report.UseColor(stdout),
				// Without this, a config that sets codes to `off` yields a
				// bare "0 failures, 0 warnings" from a check that inspected
				// nothing.
				Disabled: check.DisabledCodes(ctx.Config),
				// Without this, a config watching 4% of the repo produces a
				// green indistinguishable from one watching all of it.
				Coverage: ctx.Coverage(),
			}
			if err := report.Write(stdout, violations, f, opt); err != nil {
				return err
			}

			*exit = report.ExitCode(violations, strict)
			return nil
		},
	}

	cmd.Flags().StringVar(&format, "format", string(report.FormatHuman), "output format: human or json")
	cmd.Flags().BoolVar(&strict, "strict", false, "promote warnings to failures for the exit code")
	return cmd
}
