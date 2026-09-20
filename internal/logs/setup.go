package logs

import (
	"log/slog"
	"os"
	"runtime"
	"strings"
)

// Setup installs the process-wide logger and returns it.
//
// The defaults come from the same question pgcopydb's set_logger() asks:
// is a person watching this, or is it going somewhere to be read later?
//
//	interactive     short time, colour, no source, no pid
//	redirected/CI   long time, no colour, source and pid
//
// Both because the answer to "what would a reader want here" differs,
// and a build log in CI needs to say which line of which file complained
// while a terminal does not.
//
// Environment, mirroring PGCOPYDB_*:
//
//	FPCTL_LOG_LEVEL   trace|debug|info|notice|warn|error|fatal
//	FPCTL_LOG_TTY     whether a person is watching, for subprocesses
//	FPCTL_LOG_ACTOR   what to call this process in the log, for a child
//	FPCTL_LOG_JSON    emit slog's JSON instead of the line format
//	NO_COLOR          the usual convention; any value disables colour
func Setup() *slog.Logger {
	interactive := isTerminal(os.Stderr)
	// A child writes to a pipe, so asking its own stderr always says
	// "redirected" — and a child of an interactive fpctl would print
	// long timestamps and source locations beside a parent printing
	// neither. The parent publishes what it decided; the child follows.
	if v := os.Getenv(InteractiveEnv); v != "" {
		interactive = truthy(v)
	} else if interactive {
		_ = os.Setenv(InteractiveEnv, "1")
	}
	lock, _ := newLock()

	level := LevelNotice
	if v := os.Getenv("FPCTL_LOG_LEVEL"); v != "" {
		if l, ok := ParseLevel(v); ok {
			level = l
		}
	}

	var h slog.Handler
	if truthy(os.Getenv("FPCTL_LOG_JSON")) {
		h = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
			Level:       level,
			AddSource:   true,
			ReplaceAttr: jsonLevelNames,
		})
	} else {
		opts := Options{
			Level:      level,
			Color:      interactive && os.Getenv("NO_COLOR") == "",
			TimeFormat: TimeShort,
			Lock:       lock,
		}
		if !interactive {
			opts.TimeFormat = TimeLong
			opts.Source = true
			opts.PID = os.Getpid()
		}
		// A child shares its parent's terminal, so its lines need to
		// say which process they came from even when the terminal is
		// interactive — the one case the rule above does not cover.
		// A name if the parent gave it one, the pid otherwise.
		if IsChild() {
			opts.PID = os.Getpid()
			opts.Actor = os.Getenv(ActorEnv)
		}
		// Debug and below always carry their call site, interactive or
		// not: someone at that level is looking for where, not what.
		if level <= LevelDebug {
			opts.Source = true
		}
		rememberOptions(opts)
		h = NewHandler(os.Stderr, opts)
	}

	l := slog.New(h)
	slog.SetDefault(l)
	return l
}

// ActorEnv is the name a parent gives a child to log under, in place of
// its pid. fpctl build site sets it to "fpbuild" for the binary it
// compiles and execs.
const ActorEnv = "FPCTL_LOG_ACTOR"

// InteractiveEnv carries the parent's answer to "is a person watching
// this" down to its subprocesses, which cannot tell by looking at the
// pipe they were handed.
const InteractiveEnv = "FPCTL_LOG_TTY"

// SetLevel changes the level after Setup, which is what a future -v/-q
// flag would do once parsed.
//
// It also publishes the level in the environment, so a subprocess picks
// up the verbosity that was asked for — without that, a more verbose
// fpctl would turn itself up and leave fpbuild at its own default,
// which is exactly the case this exists for: the detail wanted is
// usually the child's.
func SetLevel(level slog.Level) {
	current := currentOptions()
	current.Level = level
	if level <= LevelDebug {
		current.Source = true
	}
	_ = os.Setenv("FPCTL_LOG_LEVEL", strings.ToLower(LevelName(level)))
	rememberOptions(current)
	slog.SetDefault(slog.New(NewHandler(os.Stderr, current)))
}

// Verbosity turns flag count into a level, the way pgcopydb's -v/-vv/-q
// do: each -v steps one notch more verbose, -q pins it to errors only.
//
// From the NOTICE default: -v is INFO (every source, not just the
// milestones), -vv is DEBUG (why it decided that), -vvv is TRACE.
func Verbosity(v int, quiet bool) slog.Level {
	if quiet {
		return LevelError
	}
	switch {
	case v <= 0:
		return LevelNotice
	case v == 1:
		return LevelInfo
	case v == 2:
		return LevelDebug
	default:
		return LevelTrace
	}
}

// jsonLevelNames makes the JSON handler spell the custom levels, which
// it otherwise renders as "DEBUG+6".
func jsonLevelNames(_ []string, a slog.Attr) slog.Attr {
	if a.Key == slog.LevelKey {
		if l, ok := a.Value.Any().(slog.Level); ok {
			a.Value = slog.StringValue(LevelName(l))
		}
	}
	return a
}

func truthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "0", "false", "no", "off":
		return false
	}
	return true
}

// isTerminal reports whether f is a character device, which is how
// isatty answers the same question.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func runtimeCallersFrame(pc uintptr) runtime.Frame {
	fs := runtime.CallersFrames([]uintptr{pc})
	f, _ := fs.Next()
	return f
}
