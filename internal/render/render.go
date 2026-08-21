// Package render turns .d2 files into SVG using the embedded D2 library.
//
// "Embedded, not a subprocess" is the entire argument for L8 and for choosing
// Go: a user running `trestle render` needs no `d2` binary on PATH, and the
// version that lays out the diagram is by construction the same version that
// [internal/nodes] parsed it with. Shelling out would reintroduce an install
// prerequisite for a tool whose pitch is "add it to CI", and would let the
// renderer and the parser drift apart.
//
// Rendering is deliberately kept off the check path. It is orders of magnitude
// slower than everything else Trestle does — layout is the expensive part — and
// `check` has a 200ms budget it has to keep. The two share the config loader
// and nothing else.
package render

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2layouts/d2elklayout"
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	d2log "oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

// Layout engines Trestle exposes. D2 ships others; these two are the ones worth
// offering — elk for the layered service topologies this tool is aimed at, dagre
// as the faster default for everything else.
const (
	LayoutELK   = "elk"
	LayoutDagre = "dagre"
)

// Options controls one render.
type Options struct {
	// Root is the repo root. Out is resolved relative to it.
	Root string

	// Out is the directory SVGs are written to. Created if absent.
	Out string

	// Layout is "elk" or "dagre". Empty leaves the choice to each diagram's
	// own `layout-engine` var, falling back to D2's default.
	Layout string

	// Theme is a D2 theme ID. Zero is D2's default theme, which is also the
	// zero value, so an unset theme needs no special case.
	Theme int64

	// Pad is the SVG padding in pixels. Zero means D2's default.
	Pad int64
}

// Result reports what one input produced.
type Result struct {
	Source string // repo-relative .d2 path
	Out    string // repo-relative .svg path
	Bytes  int
}

// Error is a render failure for one diagram.
//
// It is distinct from a violation: a diagram that will not render is a tool
// error (exit 2), not a statement about whether the architecture is accurate.
type Error struct {
	Path string
	Err  error
}

func (e *Error) Error() string { return fmt.Sprintf("render %s: %v", e.Path, e.Err) }
func (e *Error) Unwrap() error { return e.Err }

// layoutResolver maps a layout name to D2's implementation.
//
// D2 calls this per-diagram rather than taking an engine directly, because a
// diagram can override the engine in its own `vars`. Honoring that is the point:
// the worked example sets `layout-engine: elk` inline, and a resolver that
// ignored the argument would silently lay it out with the wrong engine.
func layoutResolver(configured string) func(string) (d2graph.LayoutGraph, error) {
	return func(engine string) (d2graph.LayoutGraph, error) {
		name := engine
		if name == "" {
			name = configured
		}
		switch strings.ToLower(name) {
		case LayoutELK:
			return d2elklayout.DefaultLayout, nil
		case LayoutDagre, "":
			return d2dagrelayout.DefaultLayout, nil
		default:
			return nil, fmt.Errorf("unknown layout engine %q; use %q or %q",
				name, LayoutELK, LayoutDagre)
		}
	}
}

// File renders one .d2 to SVG and returns what it wrote.
func File(ctx context.Context, path string, opt Options) (*Result, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, &Error{Path: path, Err: err}
	}

	svg, err := SVG(ctx, string(src), path, opt)
	if err != nil {
		return nil, err
	}

	outDir := opt.Out
	if !filepath.IsAbs(outDir) {
		outDir = filepath.Join(opt.Root, outDir)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, &Error{Path: path, Err: err}
	}

	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)) + ".svg"
	outPath := filepath.Join(outDir, name)
	if err := os.WriteFile(outPath, svg, 0o644); err != nil {
		return nil, &Error{Path: path, Err: err}
	}

	return &Result{
		Source: rel(opt.Root, path),
		Out:    rel(opt.Root, outPath),
		Bytes:  len(svg),
	}, nil
}

// SVG compiles and renders D2 source, without touching the filesystem.
//
// Separated from [File] so the rendering itself is testable without a temp dir,
// and so a future preview pane could serve bytes rather than write files.
func SVG(ctx context.Context, src, name string, opt Options) ([]byte, error) {
	ruler, err := textmeasure.NewRuler()
	if err != nil {
		return nil, &Error{Path: name, Err: fmt.Errorf("text measurement: %w", err)}
	}

	renderOpts := &d2svg.RenderOpts{}
	if opt.Theme != 0 {
		theme := opt.Theme
		renderOpts.ThemeID = &theme
	}
	if opt.Pad != 0 {
		pad := opt.Pad
		renderOpts.Pad = &pad
	}

	// D2's layout code logs through a slog.Logger it expects to find on the
	// context, and prints a full goroutine stack trace as a WARN for every
	// diagram if one is missing. That noise is not ours and is not actionable,
	// so supply a logger that discards below Error — a real render failure
	// still comes back through the error return, which is where callers look.
	ctx = d2log.With(ctx, slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
		Level: slog.LevelError,
	})))

	compileOpts := &d2lib.CompileOptions{
		Ruler:          ruler,
		LayoutResolver: layoutResolver(opt.Layout),
		InputPath:      name,
	}

	// Layout is set only when config asked for one, and the distinction is
	// load-bearing in both directions.
	//
	// D2 resolves layout as: passed-in option, else the diagram's own
	// `layout-engine` var, else dagre — "passed-in opts have precedence"
	// (d2lib.applyConfigs). It also only calls LayoutResolver at all when this
	// field is non-nil; leaving it nil silently uses a hardcoded dagre and the
	// resolver never runs. That was a real bug here: `render.layout: elk` was
	// accepted, ignored, and everything came out dagre with no error.
	//
	// So: set it when configured, and `.trestle.yml` wins repo-wide. Leave it
	// nil when not, and each diagram's own declaration applies. Both are
	// predictable; silently ignoring the config was not.
	if opt.Layout != "" {
		layout := opt.Layout
		compileOpts.Layout = &layout
	}

	diagram, _, err := d2lib.Compile(ctx, src, compileOpts, renderOpts)
	if err != nil {
		return nil, &Error{Path: name, Err: err}
	}

	svg, err := d2svg.Render(diagram, renderOpts)
	if err != nil {
		return nil, &Error{Path: name, Err: err}
	}
	return svg, nil
}

func rel(root, path string) string {
	if root == "" {
		return path
	}
	r, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(r)
}
