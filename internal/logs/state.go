package logs

import (
	"log/slog"
	"os"
	"sync"
)

// The installed handler's options, so SetLevel can change one field
// without re-deriving the rest of what Setup worked out.
var (
	optsMu   sync.Mutex
	lastOpts = Options{Level: LevelNotice, TimeFormat: TimeShort}
)

func rememberOptions(o Options) {
	optsMu.Lock()
	defer optsMu.Unlock()
	lastOpts = o
}

func currentOptions() Options {
	optsMu.Lock()
	defer optsMu.Unlock()
	return lastOpts
}

// Notice logs at the default level: a milestone, one per phase.
func Notice(msg string, args ...any) { slog.Default().Log(nil, LevelNotice, msg, args...) }

// Trace is the firehose.
func Trace(msg string, args ...any) { slog.Default().Log(nil, LevelTrace, msg, args...) }

// Fatal logs and exits non-zero. The exit is the point: a caller that
// wanted to carry on would have used Error.
func Fatal(msg string, args ...any) {
	slog.Default().Log(nil, LevelFatal, msg, args...)
	os.Exit(1)
}
