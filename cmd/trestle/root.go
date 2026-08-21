package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// Exit codes. Three, and 1 and 2 stay distinct: CI has to be able to tell "your
// diagram is wrong" from "Trestle is broken", and conflating them trains people
// to ignore both (DESIGN §3).
const (
	exitClean = 0
	exitFail  = 1
	exitTool  = 2
)

// Main is the whole program, minus os.Exit. Tests call it directly, which is
// what makes the exit-code matrix testable without a subprocess for every case.
//
// The exit code arrives by pointer rather than through the error return because
// cobra's error path means "something went wrong" — exit 2 — and a repo with
// failing violations is not that. A run that finds violations succeeds at its
// job; it just reports 1.
func Main(args []string, stdout, stderr io.Writer) int {
	exit := exitClean
	root := newRootCmd(stdout, stderr, &exit)
	root.SetArgs(args)

	if err := root.Execute(); err != nil {
		// Nothing useful remains if the error report itself cannot be written:
		// the exit code is the message that matters and it still gets out.
		_, _ = fmt.Fprintf(stderr, "trestle: %v\n", err)
		return exitTool
	}
	return exit
}

func newRootCmd(stdout, stderr io.Writer, exit *int) *cobra.Command {
	root := &cobra.Command{
		Use:   "trestle",
		Short: "Bind architecture diagrams to the code they describe",
		Long: "Trestle reads architecture diagrams written in D2, reads the binding\n" +
			"directives embedded in them as comments, walks the repo once, and reports\n" +
			"where the diagram and the filesystem disagree.\n\n" +
			"Exit codes: 0 clean, 1 failing violations, 2 tool error.",
		SilenceErrors: true,
		// Usage is silenced per-command once a run is under way; a flag or
		// argument mistake still gets the usage text, because that is the one
		// class of error the usage text actually answers.
		SilenceUsage: false,
		// Cobra turns this into a --version *flag*, not a subcommand. That
		// distinction is the whole reason to use it: the ledger caps the
		// command surface at four, and `trestle version` would have been a
		// fifth for no benefit.
		Version: versionString(),
	}
	root.SetVersionTemplate("trestle {{.Version}}\n")
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.CompletionOptions.HiddenDefaultCmd = true

	root.AddCommand(newCheckCmd(stdout, exit))
	root.AddCommand(newExplainCmd(stdout))
	return root
}
