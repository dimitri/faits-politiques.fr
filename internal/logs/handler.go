package logs

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Handler writes the pgcopydb line: a timestamp, the level in a
// fixed-width coloured column, then the message and its attributes.
//
//	00:00:24.349 NOTICE  senat : 971 sénateurs, 4764 scrutins  skipped=386
//
// Fixed width on the level so the eye can find ERROR down a column
// without reading any of it, which is the entire reason that padding
// exists in pgcopydb's own format string ("%-6s").
type Handler struct {
	opts Options

	mu  *sync.Mutex
	out io.Writer
	// lock serialises with other processes sharing this stderr. nil
	// when there are none. See lock.go.
	lock *crossProcessLock

	// attrs and groups carry what WithAttrs/WithGroup accumulated, since
	// a Handler must not mutate itself when either is called.
	attrs  []slog.Attr
	groups []string
}

// Options configure a Handler. The zero value is a sane interactive
// handler at NOTICE.
type Options struct {
	// Level is the minimum level to emit.
	Level slog.Level
	// Color writes ANSI colour codes. pgcopydb sets this from
	// isatty(stderr) and so does Setup: a log going to a file or to CI
	// has no business carrying escape codes.
	Color bool
	// TimeFormat is a Go reference layout. Short when interactive (the
	// date is obvious), long when not.
	TimeFormat string
	// Source adds file:line. pgcopydb shows it when NOT interactive, or
	// whenever the level is DEBUG or below — nobody debugging wants it
	// hidden, nobody watching a build wants it shown.
	Source bool
	// PID tags each line with the process id, which matters once
	// subprocesses write to the same stream.
	PID int
	// Actor replaces the pid with a name, for a child that has one.
	//
	// The column exists to answer "which process said this", and a pid
	// answers it badly. A name in the column the pid was already using
	// costs nothing and reads better.
	Actor string
	// Lock serialises writes across processes. Set on every process
	// once one of them has children.
	Lock *crossProcessLock
}

const (
	// TimeShort is what an interactive session gets: today is not in
	// question.
	TimeShort = "15:04:05.000"
	// TimeLong is for a log that will be read later, or by something
	// else.
	TimeLong = "2006-01-02 15:04:05.000"
)

func NewHandler(w io.Writer, opts Options) *Handler {
	if opts.TimeFormat == "" {
		opts.TimeFormat = TimeShort
	}
	return &Handler{opts: opts, mu: &sync.Mutex{}, out: w, lock: opts.Lock}
}

func (h *Handler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.opts.Level
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	c := *h
	c.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &c
}

func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	c := *h
	c.groups = append(append([]string{}, h.groups...), name)
	return &c
}

func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder

	t := r.Time
	if t.IsZero() {
		t = time.Now()
	}
	b.WriteString(t.Format(h.opts.TimeFormat))
	b.WriteByte(' ')

	switch {
	case h.opts.Actor != "":
		// Padded to fit "fpbuild", the widest name this project's own
		// binaries give this column, so a parent's lines and a child's
		// stay in one column whichever of the two a line carries.
		fmt.Fprintf(&b, "%-7s ", h.opts.Actor)
	case h.opts.PID > 0:
		fmt.Fprintf(&b, "%-7s ", strconv.Itoa(h.opts.PID))
	}

	name := LevelName(r.Level)
	if h.opts.Color {
		if c, ok := levelColors[r.Level]; ok {
			fmt.Fprintf(&b, "%s%-6s%s ", c, name, reset)
		} else {
			fmt.Fprintf(&b, "%-6s ", name)
		}
	} else {
		fmt.Fprintf(&b, "%-6s ", name)
	}

	if h.opts.Source && r.PC != 0 {
		if src := source(r.PC); src != "" {
			if h.opts.Color {
				fmt.Fprintf(&b, "\x1b[90m%-25s%s ", src, reset)
			} else {
				fmt.Fprintf(&b, "%-25s ", src)
			}
		}
	}

	b.WriteString(r.Message)

	// Attributes trail the message as key=value, slog's own text shape,
	// so a line stays greppable without a parser.
	r.Attrs(func(a slog.Attr) bool {
		h.appendAttr(&b, h.groups, a)
		return true
	})
	for _, a := range h.attrs {
		h.appendAttr(&b, h.groups, a)
	}

	b.WriteByte('\n')

	// One Write of one whole line, under a lock the other processes
	// sharing this stderr also take. Both halves matter: the single
	// Write keeps a line from being split, the lock keeps two lines
	// from being written at once.
	h.lock.Lock()
	defer h.lock.Unlock()
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.out, b.String())
	return err
}

func (h *Handler) appendAttr(b *strings.Builder, groups []string, a slog.Attr) {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return
	}
	if a.Value.Kind() == slog.KindGroup {
		attrs := a.Value.Group()
		if len(attrs) == 0 {
			return
		}
		next := groups
		if a.Key != "" {
			next = append(append([]string{}, groups...), a.Key)
		}
		for _, sub := range attrs {
			h.appendAttr(b, next, sub)
		}
		return
	}
	b.WriteByte(' ')
	if len(groups) > 0 {
		b.WriteString(strings.Join(groups, "."))
		b.WriteByte('.')
	}
	b.WriteString(a.Key)
	b.WriteByte('=')
	v := a.Value.String()
	// Quote only when it would otherwise be ambiguous, which keeps the
	// common case (a path, a count) readable.
	if strings.ContainsAny(v, " \t\"") {
		v = strconv.Quote(v)
	}
	b.WriteString(v)
}

// source renders the call site the way pgcopydb does, basename:line, and
// capped so the column stays a column.
func source(pc uintptr) string {
	fs := runtimeCallersFrame(pc)
	if fs.File == "" {
		return ""
	}
	return filepath.Base(fs.File) + ":" + strconv.Itoa(fs.Line)
}
