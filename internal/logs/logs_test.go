package logs

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// The line format, pinned. It is the one thing here a reader looks at
// every day, and the padding is load-bearing: a fixed-width level column
// is what lets the eye find ERROR without reading any of it.
func TestLineFormat(t *testing.T) {
	at := time.Date(2026, 9, 20, 0, 7, 59, 434_000_000, time.UTC)

	for _, tc := range []struct {
		name string
		opts Options
		want string
	}{
		{
			// Interactive: short time, no pid, no source. The date is
			// not in question when someone is watching.
			name: "interactive",
			opts: Options{Level: LevelNotice, TimeFormat: TimeShort},
			want: "00:07:59.434 NOTICE ingested 95 rows cached=386\n",
		},
		{
			// Redirected: the long time and the pid, because this will
			// be read later and possibly interleaved with another
			// process.
			name: "redirected",
			opts: Options{Level: LevelNotice, TimeFormat: TimeLong, PID: 31228},
			want: "2026-09-20 00:07:59.434 31228 NOTICE ingested 95 rows cached=386\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			// The handler directly, with a record whose time is fixed:
			// a logger would stamp it with time.Now and there would be
			// nothing to assert.
			rec := slog.NewRecord(at, LevelNotice, "ingested 95 rows", 0)
			rec.Add("cached", 386)
			if err := NewHandler(&b, tc.opts).Handle(context.Background(), rec); err != nil {
				t.Fatal(err)
			}

			// The level column is padded to six; collapse runs of
			// spaces so the assertion is about the parts, not the
			// arithmetic.
			got := strings.Join(strings.Fields(b.String()), " ") + "\n"
			want := strings.Join(strings.Fields(tc.want), " ") + "\n"
			if got != want {
				t.Errorf("\n got %q\nwant %q", got, want)
			}
		})
	}
}

// The level column is fixed-width so ERROR lines up with NOTICE. Six
// characters, which is what the longest name needs.
func TestLevelColumnIsFixedWidth(t *testing.T) {
	var starts []int
	for _, l := range []slog.Level{LevelTrace, LevelDebug, LevelInfo, LevelNotice, LevelWarn, LevelError, LevelFatal} {
		var b bytes.Buffer
		slog.New(NewHandler(&b, Options{Level: LevelTrace, TimeFormat: TimeShort})).
			Log(nil, l, "m")
		starts = append(starts, strings.Index(b.String(), "m\n"))
	}
	for i, s := range starts {
		if s != starts[0] {
			t.Errorf("level %d starts its message at column %d, the first at %d",
				i, s, starts[0])
		}
	}
}

// NOTICE sits between INFO and WARN, as syslog has it -- pgcopydb orders
// it below INFO instead, and this deliberately does not copy that.
func TestLevelOrdering(t *testing.T) {
	ordered := []slog.Level{LevelTrace, LevelDebug, LevelInfo, LevelNotice, LevelWarn, LevelError, LevelFatal}
	for i := 1; i < len(ordered); i++ {
		if ordered[i] <= ordered[i-1] {
			t.Errorf("%s (%d) does not sort above %s (%d)",
				LevelName(ordered[i]), ordered[i], LevelName(ordered[i-1]), ordered[i-1])
		}
	}
	if LevelNotice <= LevelInfo || LevelNotice >= LevelWarn {
		t.Errorf("NOTICE is %d, want it strictly between INFO (%d) and WARN (%d)",
			LevelNotice, LevelInfo, LevelWarn)
	}
	// slog's own four keep their standard values, so a library logging
	// through slog (pgx's tracer, for instance) lands where it should.
	if LevelDebug != slog.LevelDebug || LevelInfo != slog.LevelInfo ||
		LevelWarn != slog.LevelWarn || LevelError != slog.LevelError {
		t.Error("slog's own levels have been moved")
	}
}

// The default is NOTICE, which is what makes a successful run quiet.
func TestDefaultLevelHidesPerItemDetail(t *testing.T) {
	var b bytes.Buffer
	l := slog.New(NewHandler(&b, Options{Level: LevelNotice, TimeFormat: TimeShort}))

	l.Info("normalized", "table", "core.ballot") // per-item
	l.Debug("cache hit", "section", "communes")  // reasoning
	if b.Len() != 0 {
		t.Errorf("per-item detail leaked at the default level: %q", b.String())
	}

	l.Log(nil, LevelNotice, "ingestion complete", "sources", 7)
	l.Warn("source skipped: no data for this year")
	l.Error("connection failed")
	for _, want := range []string{"ingestion complete", "source skipped", "connection failed"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("%q was suppressed at the default level", want)
		}
	}
}

// -v steps one notch at a time, -q pins to errors -- pgcopydb's own
// stepping, from a NOTICE default rather than its INFO one.
func TestVerbosity(t *testing.T) {
	for _, tc := range []struct {
		v     int
		quiet bool
		want  slog.Level
	}{
		{0, false, LevelNotice},
		{1, false, LevelInfo},
		{2, false, LevelDebug},
		{3, false, LevelTrace},
		{9, false, LevelTrace},
		{0, true, LevelError},
		{3, true, LevelError}, // -q wins over -v
	} {
		if got := Verbosity(tc.v, tc.quiet); got != tc.want {
			t.Errorf("Verbosity(%d, %v) = %s, want %s",
				tc.v, tc.quiet, LevelName(got), LevelName(tc.want))
		}
	}
}

// Colour is for a terminal. A log going to a file or to CI carries no
// escape codes, which is pgcopydb's isatty test.
func TestColorOnlyWhenAsked(t *testing.T) {
	var plain, colored bytes.Buffer
	slog.New(NewHandler(&plain, Options{Level: LevelInfo, TimeFormat: TimeShort})).
		Error("boom")
	slog.New(NewHandler(&colored, Options{Level: LevelInfo, TimeFormat: TimeShort, Color: true})).
		Error("boom")

	if strings.Contains(plain.String(), "\x1b[") {
		t.Errorf("escape codes without Color: %q", plain.String())
	}
	if !strings.Contains(colored.String(), "\x1b[31m") {
		t.Errorf("no red on an ERROR with Color: %q", colored.String())
	}
}

func TestParseLevel(t *testing.T) {
	for in, want := range map[string]slog.Level{
		"notice": LevelNotice, "NOTICE": LevelNotice, " warn ": LevelWarn,
		"warning": LevelWarn, "trace": LevelTrace, "fatal": LevelFatal,
	} {
		got, ok := ParseLevel(in)
		if !ok || got != want {
			t.Errorf("ParseLevel(%q) = %s, %v; want %s", in, LevelName(got), ok, LevelName(want))
		}
	}
	if _, ok := ParseLevel("chatty"); ok {
		t.Error("ParseLevel accepted a level that does not exist")
	}
}

// A child logs under its name where its pid would go, so a line from
// "fpbuild" and a line from the parent stay in one column.
func TestActorReplacesThePIDInItsColumn(t *testing.T) {
	var b bytes.Buffer
	l := slog.New(NewHandler(&b, Options{
		Level: LevelInfo, TimeFormat: TimeShort, PID: 12345, Actor: "fpbuild",
	}))
	l.Info("construction terminée", "pages", 244)
	out := b.String()
	if !strings.Contains(out, "fpbuild ") {
		t.Errorf("no actor in the line: %q", out)
	}
	if strings.Contains(out, "12345") {
		t.Errorf("the pid is still there beside the name: %q", out)
	}
	if strings.Contains(out, "|") {
		t.Errorf("a prefix crept back in: %q", out)
	}
}

// Without a name it is still the pid: a subprocess with nothing better
// to be called must still be attributable.
func TestThePIDIsUsedWhenThereIsNoName(t *testing.T) {
	var b bytes.Buffer
	l := slog.New(NewHandler(&b, Options{Level: LevelInfo, TimeFormat: TimeShort, PID: 12345}))
	l.Info("hello")
	if !strings.Contains(b.String(), "12345") {
		t.Errorf("no pid in the line: %q", b.String())
	}
}

// Both spellings occupy the same width, which is the point of using the
// column rather than a prefix.
func TestActorAndPIDLineUp(t *testing.T) {
	var withName, withPID bytes.Buffer
	slog.New(NewHandler(&withName, Options{
		Level: LevelInfo, TimeFormat: TimeShort, Actor: "fpbuild",
	})).Info("x")
	slog.New(NewHandler(&withPID, Options{
		Level: LevelInfo, TimeFormat: TimeShort, PID: 54647,
	})).Info("x")
	col := func(s string) int { return strings.Index(s, "INFO") }
	if col(withName.String()) != col(withPID.String()) {
		t.Errorf("levels start in different columns:\n%s%s", withName.String(), withPID.String())
	}
}
