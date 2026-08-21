package render

import (
	"context"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// DebounceWindow is how long to wait for a burst of filesystem events to settle
// before re-rendering.
//
// Editors do not write a file once. Atomic-save implementations write a temp
// file, rename it over the target, and often touch the mode afterwards — three
// events for one Cmd-S. Rendering each of them makes the tool look broken and
// wastes the most expensive thing it does.
const DebounceWindow = 150 * time.Millisecond

// Event reports one render triggered by a file change.
//
// Err is non-nil when that diagram failed to render. It is delivered rather
// than returned because a watch must survive it: a diagram is unparseable for
// every keystroke between `a ->` and `a -> b`, and a watcher that exited on the
// first syntax error would be unusable for the editing it exists to support.
type Event struct {
	Result *Result
	Err    error
}

// Watch re-renders paths whenever they change, until ctx is cancelled.
//
// It watches the containing directories rather than the files themselves.
// Watching a file inode misses atomic saves entirely — the editor replaces the
// file, the watch stays bound to the old inode, and nothing fires again after
// the first save.
func Watch(ctx context.Context, paths []string, opt Options, on func(Event)) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer func() { _ = w.Close() }()

	watched := make(map[string]bool, len(paths))
	dirs := make(map[string]bool)
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return err
		}
		watched[abs] = true
		dirs[filepath.Dir(abs)] = true
	}
	for d := range dirs {
		if err := w.Add(d); err != nil {
			return err
		}
	}

	// Pending changes accumulate here until the burst stops. A map rather than
	// a slice so a file saved three times in one window renders once.
	pending := make(map[string]bool)
	var timer <-chan time.Time

	flush := func() {
		for p := range pending {
			delete(pending, p)
			res, err := File(ctx, p, opt)
			on(Event{Result: res, Err: err})
		}
	}

	for {
		select {
		case <-ctx.Done():
			return nil

		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			// Writes to the output directory must not retrigger the render that
			// produced them. Only files we were asked to watch count, which
			// rules that out by construction.
			abs, err := filepath.Abs(ev.Name)
			if err != nil || !watched[abs] {
				continue
			}
			if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
				continue
			}
			pending[abs] = true
			timer = time.After(DebounceWindow)

		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			on(Event{Err: err})

		case <-timer:
			timer = nil
			flush()
		}
	}
}
