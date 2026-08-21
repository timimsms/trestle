package main

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/timimsms/trestle/internal/render"
	"github.com/timimsms/trestle/internal/run"
)

// newRenderCmd builds `trestle render`.
//
// Second of the four commands. It renders through the embedded D2 library
// rather than shelling out, which is the whole argument for L8 — no `d2` binary
// on PATH, and the version laying out the diagram is by construction the one
// that parsed it.
//
// Exit codes match `check`: 0 on success, 2 on a tool error. There is no exit 1,
// because rendering makes no claim about whether the architecture is accurate —
// a diagram that renders beautifully can still be a lie. That is `check`'s job.
func newRenderCmd(stdout io.Writer, exit *int) *cobra.Command {
	var quiet bool

	cmd := &cobra.Command{
		Use:   "render",
		Short: "Render diagrams to SVG via the embedded D2 library",
		Long: "Render every diagram matched by `diagrams:` to the directory named by\n" +
			"`render.out`, using the layout engine and theme from `.trestle.yml`.\n\n" +
			"No `d2` binary is required — D2 is embedded, so the renderer and the\n" +
			"parser can never disagree about a version.\n\n" +
			"Rendering says nothing about whether the diagram is accurate. Run\n" +
			"`trestle check` for that.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, err := run.Load("")
			if err != nil {
				return err
			}

			out := ctx.Config.Render.Out
			if out == "" {
				return fmt.Errorf("%s: no `render.out`, so there is nowhere to write SVGs\n"+
					"  hint: add a `render:` block, e.g.\n"+
					"    render:\n      out: docs/architecture/rendered/",
					ctx.Config.Path)
			}

			opt := render.Options{
				Root:   ctx.Config.Root,
				Out:    out,
				Layout: ctx.Config.Render.Layout,
				Theme:  int64(ctx.Config.Render.Theme),
			}

			for _, p := range ctx.Paths {
				res, err := render.File(cmd.Context(), filepath.Join(ctx.Config.Root, p), opt)
				if err != nil {
					return err
				}
				if !quiet {
					_, _ = fmt.Fprintf(stdout, "%s -> %s\n", res.Source, res.Out)
				}
			}

			if !quiet {
				noun := "diagrams"
				if len(ctx.Paths) == 1 {
					noun = "diagram"
				}
				_, _ = fmt.Fprintf(stdout, "%d %s rendered\n", len(ctx.Paths), noun)
			}
			*exit = exitClean
			return nil
		},
	}

	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "print nothing on success")
	return cmd
}
