package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/timimsms/trestle/internal/scaffold"
)

// newInitCmd builds `trestle init`.
//
// Fourth and last of the four commands. It is the only one that writes to the
// repo rather than reading it, which is why it is the only one that asks first:
// the `discover:` rules it proposes decide what every later `trestle check` will
// complain about, and a rule nobody agreed to is a source of noise that gets the
// check bypassed. So the proposal is printed, with what each rule matches today,
// and nothing is written until the answer is yes.
//
// Exit codes match `render`: 0 on success, 2 on a tool error. Declining the
// proposal is a success — the user was asked a question and answered it.
func newInitCmd(stdout io.Writer) *cobra.Command {
	var (
		yes    bool
		dryRun bool
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold .trestle.yml, the agent contract and a starter diagram",
		Long: "Set up Trestle in the current directory, which becomes the repo root.\n\n" +
			"init writes four things: `.trestle.yml` with `discover:` rules seeded from\n" +
			"the layout it finds, an empty starter diagram, CONVENTIONS.md — the\n" +
			"diagram-authoring contract, embedded in this binary — and a stanza appended\n" +
			"to AGENTS.md.\n\n" +
			"It proposes and does not impose: the rules and what they currently match are\n" +
			"printed first, and nothing is written until you agree. Nothing existing is\n" +
			"ever overwritten, so running init twice is safe.\n\n" +
			"The starter diagram has no nodes in it. Trestle will not invent boxes for\n" +
			"you — a diagram generated from your directory listing would pass its own\n" +
			"check while telling you nothing.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Past this point every error is about the repo, not about how the
			// command was typed, and the usage text would bury it.
			cmd.SilenceUsage = true

			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			plan, err := scaffold.Prepare(cwd)
			if err != nil {
				return err
			}

			if err := plan.WriteProposal(stdout); err != nil {
				return err
			}

			switch {
			case plan.Writes() == 0:
				_, _ = fmt.Fprint(stdout, "\nNothing to do — this repo is already set up.\n"+
					"Run `trestle check` to see where the diagrams and the code disagree.\n")
				return nil
			case dryRun:
				_, _ = fmt.Fprintf(stdout, "\n--dry-run: %d files would be written, none were.\n", plan.Writes())
				return nil
			}

			if !yes {
				ok, err := confirm(cmd.InOrStdin(), stdout, plan.Writes())
				if err != nil {
					return err
				}
				if !ok {
					_, _ = fmt.Fprint(stdout, "\nNothing written.\n"+
						"Edit nothing and re-run, or write `.trestle.yml` by hand — "+
						"`trestle init --dry-run` prints what this would have said.\n")
					return nil
				}
			}

			if err := plan.Apply(); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(stdout)
			return plan.WriteResult(stdout)
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "accept the proposal without prompting")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the proposal and write nothing")
	return cmd
}

// confirm asks the question and reads one line.
//
// An unanswerable prompt — stdin closed, empty, or redirected from /dev/null,
// which is what a CI runner looks like — is a tool error rather than a silent
// decline. `trestle init` in a script that then reports success while having
// written nothing is the kind of quiet no-op this repo keeps refusing to ship.
func confirm(r io.Reader, w io.Writer, files int) (bool, error) {
	noun := "files"
	if files == 1 {
		noun = "file"
	}
	_, _ = fmt.Fprintf(w, "\nWrite %d %s? [y/N] ", files, noun)

	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil && strings.TrimSpace(line) == "" {
		if errors.Is(err, io.EOF) {
			return false, errors.New("nothing to read on stdin, so the proposal cannot be confirmed\n" +
				"  hint: re-run with `--yes` to accept it, or `--dry-run` to see it and write nothing")
		}
		return false, err
	}

	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}
