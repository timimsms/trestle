package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/timimsms/trestle/internal/explain"
	"github.com/timimsms/trestle/internal/report"
	"github.com/timimsms/trestle/internal/run"
)

// newExplainCmd builds `trestle explain`.
//
// It loads the repo exactly as `check` does — same config discovery, same single
// walk, same parse — and then reports instead of judging. Two consequences the
// code below exists to keep true:
//
//   - **Exit 0 always.** A node with three failing violations still exits 0.
//     `explain` is a debugging surface; if it could fail, people would start
//     running it in CI and it would become a second `check` with a different
//     opinion. The only non-zero exit is 2, and only when Trestle could not do
//     the job at all.
//   - **`--overlaps` is a flag, not a command.** The command surface is capped
//     at four (GAMEPLAN §6), and "list the paths two nodes both claim" is a
//     question about the same loaded repo.
func newExplainCmd(stdout io.Writer) *cobra.Command {
	var (
		format   string
		overlaps bool
	)

	cmd := &cobra.Command{
		Use:   "explain [node_id]",
		Short: "Show what Trestle parsed: nodes, bindings and what they match",
		Long: "With no argument, list every node in every configured diagram with its\n" +
			"binding status — the inventory, which is how you confirm the tool sees what\n" +
			"you think it sees. With a node ID, show that node's bindings, how many files\n" +
			"each glob matches right now, and any violations against it.\n\n" +
			"--overlaps lists paths claimed by more than one node. Overlap is legal and\n" +
			"gets no violation code; this is where it is visible.\n\n" +
			"explain reports, it does not judge: it exits 0 whatever it finds.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Checked before usage is silenced: this one is a mistake about how
			// the command was typed, which is the only class of error the usage
			// text answers.
			if overlaps && len(args) > 0 {
				return fmt.Errorf("--overlaps takes no node ID; run `trestle explain %s` for one node", args[0])
			}
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

			rep := explain.Build(ctx.Input())

			var view *explain.View
			switch {
			case overlaps:
				view = rep.OverlapView()
			case len(args) == 1:
				view = rep.NodeView(args[0])
				// An ID that resolves to several nodes is shown, not refused —
				// it is the SYNTAX `check` reports and the candidates are the
				// fix. An ID that resolves to nothing is a different thing: the
				// command was asked about a node that does not exist, and
				// answering that with a clean exit is how a misspelled ID reads
				// as confirmation.
				if !view.Found() {
					return &explain.UnknownNodeError{Query: args[0], Diagrams: rep.Diagrams}
				}
			default:
				view = rep.Inventory()
			}

			return explain.Write(stdout, view, f)
		},
	}

	cmd.Flags().StringVar(&format, "format", string(report.FormatHuman), "output format: human or json")
	cmd.Flags().BoolVar(&overlaps, "overlaps", false, "list paths claimed by more than one node")
	return cmd
}
