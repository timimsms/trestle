package render

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// waitFor polls until cond is true or the deadline passes. Filesystem events
// are inherently racy; a fixed sleep either flakes or wastes time.
func waitFor(t *testing.T, d time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}

func watchFixture(t *testing.T) (root, src string) {
	t.Helper()
	root = t.TempDir()
	src = filepath.Join(root, "system.d2")
	if err := os.WriteFile(src, []byte(minimal), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, src
}

func TestWatchRendersOnSave(t *testing.T) {
	root, src := watchFixture(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan Event, 8)
	go func() {
		_ = Watch(ctx, []string{src}, Options{Root: root, Out: "out"}, func(e Event) {
			events <- e
		})
	}()

	// Give the watcher a moment to register before writing, or the event is
	// raised against a directory nobody is listening to yet.
	time.Sleep(150 * time.Millisecond)

	if err := os.WriteFile(src, []byte(minimal+"\nc: Gamma\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-events:
		if ev.Err != nil {
			t.Fatalf("render failed: %v", ev.Err)
		}
		if ev.Result == nil || ev.Result.Bytes == 0 {
			t.Fatal("no SVG produced")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no render within 5s of a save")
	}
}

// Three writes inside the debounce window must produce one render. Editors
// write a file two or three times per save — temp file, rename, chmod — and
// rendering each of them makes the tool look broken.
func TestWatchDebouncesABurst(t *testing.T) {
	root, src := watchFixture(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// atomic: the callback runs on the watcher goroutine, the assertion on this one.
	var count atomic.Int64
	done := make(chan struct{}, 8)
	go func() {
		_ = Watch(ctx, []string{src}, Options{Root: root, Out: "out"}, func(Event) {
			count.Add(1)
			done <- struct{}{}
		})
	}()
	time.Sleep(150 * time.Millisecond)

	for i := 0; i < 3; i++ {
		if err := os.WriteFile(src, []byte(minimal), 0o644); err != nil {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond) // well inside DebounceWindow
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("no render at all")
	}

	// Let any un-coalesced renders arrive before counting.
	time.Sleep(3 * DebounceWindow)
	if n := count.Load(); n != 1 {
		t.Errorf("burst of 3 writes produced %d renders, want 1", n)
	}
}

// A syntax error is a normal state while typing — `a ->` exists for however long
// it takes to type the next character. The watch must report it and keep going.
func TestWatchSurvivesASyntaxError(t *testing.T) {
	root, src := watchFixture(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan Event, 8)
	go func() {
		_ = Watch(ctx, []string{src}, Options{Root: root, Out: "out"}, func(e Event) {
			events <- e
		})
	}()
	time.Sleep(150 * time.Millisecond)

	if err := os.WriteFile(src, []byte("a -> {{{"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-events:
		if ev.Err == nil {
			t.Fatal("want an error for unparseable D2")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no event for the broken save")
	}

	// The watcher must still be alive. This is the whole point.
	if err := os.WriteFile(src, []byte(minimal), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-events:
		if ev.Err != nil {
			t.Fatalf("watch did not recover after a syntax error: %v", ev.Err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("watch stopped after a syntax error; it must survive one")
	}
}

// Writing the SVG must not retrigger the render that produced it. The output
// directory usually sits beside the source, so the watcher sees those writes.
func TestWatchIgnoresItsOwnOutput(t *testing.T) {
	root, src := watchFixture(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var count atomic.Int64
	go func() {
		_ = Watch(ctx, []string{src}, Options{Root: root, Out: "."}, func(Event) {
			count.Add(1)
		})
	}()
	time.Sleep(150 * time.Millisecond)

	if err := os.WriteFile(src, []byte(minimal), 0o644); err != nil {
		t.Fatal(err)
	}
	if !waitFor(t, 5*time.Second, func() bool { return count.Load() >= 1 }) {
		t.Fatal("no render")
	}

	// If the SVG write fed back, this would climb without bound.
	time.Sleep(5 * DebounceWindow)
	if n := count.Load(); n > 1 {
		t.Errorf("render loop: %d renders from one save — the watcher is seeing its own output", n)
	}
}

func TestWatchStopsOnContextCancel(t *testing.T) {
	_, src := watchFixture(t)

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		_ = Watch(ctx, []string{src}, Options{Out: "out"}, func(Event) {})
		close(stopped)
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Watch did not return after context cancellation")
	}
}
