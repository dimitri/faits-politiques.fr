// Package logs gives fpctl one logging system, modelled on the one
// pgcopydb uses — ported from app.taop.xyz's internal/logs, which
// already made this same choice for a very similar tool (a CLI that
// wraps ingestion, a build pipeline, and long external commands).
//
// # What pgcopydb does
//
// A level, a timestamp, and a colour, on every line:
//
//	00:00:24.349 12345 NOTICE  captured 95 queries
//
// The details worth copying, all from src/bin/lib/log and main.c's
// set_logger():
//
//   - The level is the filter AND the vocabulary. Nine of them, each with
//     a colour, printed in a fixed-width column so the eye finds ERROR
//     without reading.
//   - Colour only when stderr is a terminal (isatty). A log redirected to
//     a file or scraped by CI gets no escape codes.
//   - The time format follows from the same question: short (%H:%M:%S)
//     interactively, where the date is obvious, and long
//     (%Y-%m-%d %H:%M:%S) when redirected, where it is not.
//   - So does source location: file:line is shown when NOT interactive,
//     or whenever the level is DEBUG or lower. Nobody debugging wants it
//     hidden; nobody watching a build wants it shown.
//   - Everything goes to stderr, so stdout stays a data channel.
//   - -v steps the level down one notch at a time, -q pins it to ERROR.
//
// # The level ordering
//
// pgcopydb orders NOTICE *below* INFO, so its INFO default hides NOTICE
// and -v reveals it. That is backwards from syslog and from Go, and this
// package does not copy it: here NOTICE sits between INFO and WARN, as
// syslog has it, and the default level is NOTICE.
//
// Which means silence unless something is worth saying. That is the
// whole point of the exercise: "fpctl ingest systeme checksums" printing
// ten lines of "ok" is ten lines nobody reads, and the eleventh line,
// the one that says a checksum could not be written, is the only one
// that ever mattered.
//
// # How to categorise a message
//
// The test is what a reader does about it.
//
//	ERROR   the command failed, and here is why. Always shown.
//	WARN    it worked, but something is wrong and a person should look:
//	        a source skipped because it changed shape, a rasterizer that
//	        fell back, a cache that could not be written.
//	NOTICE  the milestones of a run. One per phase, not one per file:
//	        "senat : 971 sénateurs, 4764 scrutins", never one line per
//	        row inserted. The default, so this is the whole story
//	        someone gets for free.
//	INFO    the per-item detail behind those milestones. Useful when
//	        something looks wrong, noise when it does not.
//	DEBUG   why the tool decided what it decided: cache hits, checksums
//	        compared, commands about to be run.
//	TRACE   the firehose.
//
// The rule that keeps NOTICE useful: anything that happens once per row
// or per file is INFO or below. If a message can be emitted hundreds of
// times in one run, it is not a milestone.
//
// # Go's own tools
//
// log/slog does all of this, and the parts that look like they need a
// library do not. Levels are plain ints rather than a closed enum
// (slog.Level is an int, with Debug=-4, Info=0, Warn=4, Error=8), so
// NOTICE is a value between two of them and needs no fork of anything.
// A Handler is one small interface, so the pgcopydb line format is one
// type in this file. JSON output is slog.NewJSONHandler, already
// written. What is left to supply is the format, the colours and the
// vocabulary — which is exactly the part that is this project's choice
// and nobody else's.
package logs

import (
	"fmt"
	"log/slog"
	"strings"
)

// The levels, as slog levels. slog's own four keep their standard values
// so that any library logging through slog (pgx's tracer, for instance)
// lands where it should.
const (
	LevelTrace  = slog.Level(-8)
	LevelDebug  = slog.LevelDebug // -4
	LevelInfo   = slog.LevelInfo  // 0
	LevelNotice = slog.Level(2)
	LevelWarn   = slog.LevelWarn  // 4
	LevelError  = slog.LevelError // 8
	LevelFatal  = slog.Level(12)
)

var levelNames = map[slog.Level]string{
	LevelTrace:  "TRACE",
	LevelDebug:  "DEBUG",
	LevelInfo:   "INFO",
	LevelNotice: "NOTICE",
	LevelWarn:   "WARN",
	LevelError:  "ERROR",
	LevelFatal:  "FATAL",
}

// levelColors: green NOTICE, cyan INFO, yellow WARN, red ERROR, magenta
// FATAL, blue DEBUG, grey TRACE.
//
// Colour follows the level ordering rather than the name: NOTICE is
// above INFO here and is the headline — the source about to load, the
// count just computed — while INFO is the per-item detail behind it.
// Green reads as the spine of the run; INFO's cooler cyan is what fades
// into the background when scanning for the headline.
var levelColors = map[slog.Level]string{
	LevelTrace:  "\x1b[90m",
	LevelDebug:  "\x1b[34m",
	LevelInfo:   "\x1b[36m",
	LevelNotice: "\x1b[32m",
	LevelWarn:   "\x1b[33m",
	LevelError:  "\x1b[31m",
	LevelFatal:  "\x1b[35m",
}

const reset = "\x1b[0m"

// ParseLevel maps a spelling to a level, for a flag or an environment
// variable.
func ParseLevel(s string) (slog.Level, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "trace":
		return LevelTrace, true
	case "debug":
		return LevelDebug, true
	case "info":
		return LevelInfo, true
	case "notice":
		return LevelNotice, true
	case "warn", "warning":
		return LevelWarn, true
	case "error":
		return LevelError, true
	case "fatal":
		return LevelFatal, true
	}
	return 0, false
}

// LevelName spells a level for output.
func LevelName(l slog.Level) string {
	if n, ok := levelNames[l]; ok {
		return n
	}
	return l.String()
}

// Plural counts a thing in a sentence rather than in a field, because
// "1 sources" is the sort of detail that says nobody read the output.
func Plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %s", n, plural(noun))
}

func plural(noun string) string {
	if noun == "" {
		return ""
	}
	last := noun[len(noun)-1]
	switch {
	case last == 'y' && len(noun) > 1 && !isVowel(noun[len(noun)-2]):
		return noun[:len(noun)-1] + "ies"
	case last == 's' || last == 'x' || last == 'z',
		strings.HasSuffix(noun, "ch"), strings.HasSuffix(noun, "sh"):
		return noun + "es"
	}
	return noun + "s"
}

func isVowel(c byte) bool { return strings.IndexByte("aeiouAEIOU", c) >= 0 }
