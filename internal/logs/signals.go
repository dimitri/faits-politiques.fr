package logs

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// Stopping cleanly, which for fpctl means two things: say what happened,
// and leave the database/site in a state the next run can continue
// from.
//
// The second is the one worth care. An ingestion or a build killed
// halfway through has touched some tables/files and not others, and
// what decides whether the next run picks up correctly is whether it
// recorded anything it should not have:
//
//   - A section checksum must not be written for a source that did not
//     finish ingesting. It never is — recalculerEmpreintes runs only
//     after every requested source has returned nil — so an interrupted
//     source is simply retried next time, from wherever its own
//     idempotent upserts left off.
//
//   - cmd/build's mettreEnPlace never sees a construction that did not
//     finish: a killed build leaves <out>.construction/ half-written and
//     <out> itself untouched, exactly the same guarantee an ordinary
//     build failure already gives.
//
//   - What does need doing is anything registered here: a compiled
//     fpbuild left running after fpctl exits, a scratch file nobody
//     will clean up otherwise.
//
// So this is a small thing on purpose. The design that makes an
// interruption safe is "record after success", not a handler that
// tidies up.
//
// The four signals, and why each:
//
//	SIGINT   Ctrl-C. Stop, and say so.
//	SIGTERM  a supervisor or a container runtime asking. Same.
//	SIGHUP   the terminal went away. Nothing here reloads configuration
//	         on HUP, and a run whose terminal has closed should stop
//	         rather than carry on writing to a pipe nobody holds.
//	SIGQUIT  deliberately NOT handled: Ctrl-\ is how a person asks Go
//	         for a stack dump of every goroutine, which is exactly what
//	         someone does when a run seems stuck. Catching it would take
//	         away the one debugging tool that needs no preparation.
type shutdown struct {
	mu    sync.Mutex
	tasks []func()
	hard  []func()
}

var onStop shutdown

// OnStop registers work to do before exiting on a signal: killing a
// child, removing a scratch file. Called in reverse order, like defer,
// because later registrations tend to depend on earlier ones.
func OnStop(fn func()) {
	onStop.mu.Lock()
	defer onStop.mu.Unlock()
	onStop.tasks = append(onStop.tasks, fn)
}

// OnHardStop registers work to do on the SECOND signal, immediately
// before exiting.
//
// The first signal asks politely and unwinds; the second is somebody
// who has decided not to wait, and a tool that ignores the second press
// is the reason people reach for kill -9. Whatever is registered here
// has one job: kill what this process started, since after os.Exit
// nothing else can — see internal/toolrun, which registers exactly this
// for the external commands fpctl shells out to.
func OnHardStop(fn func()) {
	onStop.mu.Lock()
	defer onStop.mu.Unlock()
	onStop.hard = append(onStop.hard, fn)
}

// Context returns a context cancelled by the first signal, having said
// what arrived. A second signal exits immediately: someone pressing
// Ctrl-C twice means it, and a tool that ignores the second press is
// the reason people reach for kill -9.
func Context() (context.Context, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan os.Signal, 2)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	go func() {
		sig := <-ch
		slog.Warn("stopping", "signal", sig.String(),
			"note", "work already finished is kept; the rest is simply still to do")
		cancel()
		run()

		// From here a second signal is fatal. The first has asked
		// politely and work is unwinding; the second is someone who
		// has decided not to wait.
		sig = <-ch
		fmt.Fprintf(os.Stderr, "\n%s again: exiting now\n", sig)
		hardStop()
		os.Exit(ExitCode)
	}()

	return ctx, func() {
		signal.Stop(ch)
		cancel()
	}
}

// run executes the registered cleanups, newest first.
func run() {
	onStop.mu.Lock()
	tasks := onStop.tasks
	onStop.tasks = nil
	onStop.mu.Unlock()

	for i := len(tasks) - 1; i >= 0; i-- {
		tasks[i]()
	}
}

// ExitCode is the conventional status for "killed by SIGINT", which is
// what a shell reports when Ctrl-C reaches a program.
const ExitCode = 130

// hardStop runs the second-signal cleanups. Newest first, like run, and
// it does not clear them: nothing runs after this.
func hardStop() {
	onStop.mu.Lock()
	tasks := onStop.hard
	onStop.mu.Unlock()
	for i := len(tasks) - 1; i >= 0; i-- {
		tasks[i]()
	}
}
