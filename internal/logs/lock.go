package logs

import (
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

// Serialising log output across processes, the way pgcopydb does.
//
// pgcopydb's problem, and ours: a parent and its children all write to
// one inherited stderr — fpctl and the fpbuild binary it spawns for
// "fpctl build site", say. Two processes writing the same descriptor can
// interleave inside a line — half of one message, then half of
// another — which is worse than useless, because the result looks like
// a message nobody wrote.
//
// Its answer (src/bin/pgcopydb/lock_utils.c, main.c's set_logger) is a
// SysV semaphore: the parent creates one, publishes its id in
// PGCOPYDB_LOG_SEMAPHORE, every child opens that same id rather than
// creating its own, log_set_lock() makes every log call take it around
// the write, and only the process whose pid owns it unlinks it at exit.
//
// This is the same design with the Unix primitive Go can reach
// portably: an advisory lock (flock) on a file whose path the parent
// publishes in FPCTL_LOG_LOCK. A child inherits the variable, locks the
// same file, and the kernel serialises them. Same guarantee, same
// inheritance, same ownership rule for cleanup.
//
// Not a mutex: a sync.Mutex is per-process and these are separate
// programs. Not "writes under PIPE_BUF are atomic": that is true, and
// every line this package emits is one Write, but it is a property of
// the pipe buffer size rather than a guarantee anyone stated, and it
// stops being true for a long line.
type crossProcessLock struct {
	mu   sync.Mutex
	path string
	f    *os.File
}

// LockEnv is where the lock file's path is published for children.
// Named after pgcopydb's PGCOPYDB_LOG_SEMAPHORE, which does the same job.
const LockEnv = "FPCTL_LOG_LOCK"

// newLock opens the lock named by the environment, or creates one and
// publishes it. The caller is the owner only in the second case, which
// is what decides who removes it.
func newLock() (l *crossProcessLock, owner bool) {
	if p := os.Getenv(LockEnv); p != "" {
		return &crossProcessLock{path: p}, false
	}
	p := filepath.Join(os.TempDir(), "fpctl-log.lock")
	if err := os.Setenv(LockEnv, p); err != nil {
		return nil, false
	}
	return &crossProcessLock{path: p}, true
}

// Lock takes the lock, opening the file on first use. Failure to lock is
// not failure to log: a line printed unserialised beats a line lost.
func (l *crossProcessLock) Lock() {
	if l == nil {
		return
	}
	l.mu.Lock()
	if l.f == nil {
		f, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDWR, 0o666)
		if err != nil {
			return
		}
		l.f = f
	}
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_EX)
}

func (l *crossProcessLock) Unlock() {
	if l == nil {
		return
	}
	if l.f != nil {
		_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	}
	l.mu.Unlock()
}

// IsChild reports whether this process inherited a parent's logging
// setup, which is what makes its pid worth printing: one process needs
// no pid to disambiguate, several sharing a terminal do.
func IsChild() bool { return os.Getenv(LockEnv) != "" }
