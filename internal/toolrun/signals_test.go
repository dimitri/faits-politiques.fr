package toolrun

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A program that ignores SIGTERM must not leave its children behind.
//
// This is the shape a build tool can have: a wrapper that spawns a
// worker as a separate process in the same group (fpbuild spawning an
// OG-image rasterizer, say). Go's own WaitDelay escalation kills
// cmd.Process — the wrapper — and returns, so the worker outlives the
// build. The escalation therefore has to be ours, and it has to go to
// the group.
func TestCancelKillsTheWholeGroupEvenWhenTERMIsIgnored(t *testing.T) {
	dir := t.TempDir()
	stub := writeStub(t, dir)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Cmd{Name: stub, Args: []string{dir}, Ctx: ctx}.Run()
	}()

	parent := waitForPID(t, filepath.Join(dir, "parent.pid"))
	child := waitForPID(t, filepath.Join(dir, "child.pid"))
	if !alive(parent) || !alive(child) {
		t.Fatal("the stub died before the test could cancel it")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("Run never returned after cancel")
	}

	// The escalation is a timer, so allow for it plus slack.
	deadline := time.Now().Add(killDelay + 10*time.Second)
	for time.Now().Before(deadline) {
		if !alive(parent) && !alive(child) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Errorf("survived the cancel: parent alive=%v, child alive=%v", alive(parent), alive(child))
	_ = syscall.Kill(parent, syscall.SIGKILL)
	_ = syscall.Kill(child, syscall.SIGKILL)
}

// The second signal exits the process outright, so whatever it is going
// to kill has to be killed before that — which is what the live group
// registry and logs.OnHardStop are for. This is that killer, tested
// directly, since the exit itself cannot be.
func TestKillTrackedGroupsReachesAStubbornGroup(t *testing.T) {
	dir := t.TempDir()
	stub := writeStub(t, dir)

	done := make(chan error, 1)
	go func() { done <- Cmd{Name: stub, Args: []string{dir}}.Run() }()

	parent := waitForPID(t, filepath.Join(dir, "parent.pid"))
	child := waitForPID(t, filepath.Join(dir, "child.pid"))

	killTrackedGroups()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !alive(parent) && !alive(child) {
			<-done
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Errorf("survived: parent alive=%v, child alive=%v", alive(parent), alive(child))
	_ = syscall.Kill(parent, syscall.SIGKILL)
	_ = syscall.Kill(child, syscall.SIGKILL)
}

// writeStub is a program that refuses to die politely, with a child in
// its own process group — the shape a wrapper-plus-worker tool has.
func writeStub(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "stubborn.sh")
	body := `#!/bin/sh
trap '' TERM
echo $$ > "$1/parent.pid"
sh -c 'trap "" TERM; echo $$ > "'"$1"'/child.pid"; while :; do sleep 0.2; done' &
while :; do sleep 0.2; done
`
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func waitForPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(path); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("%s never appeared", path)
	return 0
}

// alive asks whether a pid still exists. Signal 0 checks without
// delivering anything, which is the only portable way to ask.
func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }
