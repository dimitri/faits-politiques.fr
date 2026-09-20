// Package toolrun runs an external program fpctl shells out to —
// fpbuild (the binary behind "fpctl build site"), docker (behind
// "fpctl provision db/store") — so that a Ctrl-C or a supervisor's
// SIGTERM actually stops it, instead of leaving it running after fpctl
// itself has exited.
//
// Adapted from app.taop.xyz's internal/toolrun, written for a very
// noisy toolchain (latexmk, xelatex) that had to be captured and
// silenced on success. fpctl's own external programs are not that: a
// site build already narrates itself in milestones ("cartographie
// éditoriale", "empreintes des sections") that are exactly what someone
// watching the terminal wants to see while it runs, so this package
// keeps that live — output goes straight to the parent's own stdout and
// stderr, unlike the original, which captured everything and printed
// only a diagnosis on failure. What is worth the same care either way
// is the part that has nothing to do with how noisy the tool is: making
// sure the process this project started does not outlive fpctl.
package toolrun

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/logs"
)

// Cmd is one external program run.
type Cmd struct {
	// Name and Args are the program and its arguments.
	Name string
	Args []string
	// Dir to run in, if not the current one.
	Dir string
	// Env replaces the environment, when the program needs one it
	// would not otherwise inherit — execBinaire sets FPCTL_LOG_ACTOR
	// here so a spawned fpbuild logs under its own name rather than its
	// pid. Empty means inherit this process's own.
	Env []string
	// Ctx cancels the command. Without one, a Ctrl-C during "fpctl
	// build site" left fpbuild — and anything it had itself started,
	// such as the OG-image rasterizer in cmd/build/social.go — running
	// after fpctl had exited. exec.CommandContext kills the child when
	// the context is done, which is the one piece of OS cleanup a
	// signal here genuinely has to do.
	Ctx context.Context
}

// killDelay is how long a program has to put itself away after SIGTERM
// before the group is killed outright.
const killDelay = 3 * time.Second

// live is every process group this package has started and not yet
// reaped, so a second Ctrl-C can kill them on the way out.
//
// Registered once, and only once anything has actually been run: a
// command that shells out to nothing has no groups and needs no hook.
var live = struct {
	sync.Mutex
	pgids map[int]bool
	once  sync.Once
}{pgids: map[int]bool{}}

func trackGroup(pgid int) {
	live.once.Do(func() { logs.OnHardStop(killTrackedGroups) })
	live.Lock()
	live.pgids[pgid] = true
	live.Unlock()
}

func untrackGroup(pgid int) {
	live.Lock()
	delete(live.pgids, pgid)
	live.Unlock()
}

// killTrackedGroups is what the second signal does: SIGKILL, no asking.
// The first signal already asked.
func killTrackedGroups() {
	live.Lock()
	defer live.Unlock()
	for pgid := range live.pgids {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	}
}

// Run executes the command, its output going straight to fpctl's own
// stdout/stderr, and returns its error unchanged (an *exec.ExitError
// carrying the exit code included) — callers that forward the child's
// exit status, like execBinaire, need it exactly as exec would have
// returned it.
func (c Cmd) Run() error {
	ctx := c.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	cmd.Dir = c.Dir
	cmd.Env = c.Env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr

	// Its own process group, and the whole group is killed rather than
	// just the child.
	//
	// exec.CommandContext kills only the process it started. fpbuild
	// itself shells out (cmd/build/social.go's OG-image rasterizer,
	// some ingest connectors' own external tools), so killing only
	// fpbuild would leave those running — confirmed the same way
	// app.taop.xyz's own version of this package confirmed it for
	// latexmk/xelatex. Setpgid puts the child and everything it spawns
	// in one group, and a signal to -pgid reaches all of them.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// TERM first: a program gets the chance to leave a resumable
		// state (cmd/build never publishes a half-built site regardless,
		// but its own children — a rasterizer, a downloader — may still
		// have a temp file worth closing cleanly).
		pgid := cmd.Process.Pid
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
		// ...and SIGKILL to the GROUP if the polite signal was not
		// enough.
		//
		// Ours, not cmd.WaitDelay's escalation: that kills cmd.Process
		// alone, which is exactly the orphan this Setpgid dance exists
		// to prevent for anything cmd.Process itself started.
		time.AfterFunc(killDelay, func() { _ = syscall.Kill(-pgid, syscall.SIGKILL) })
		return nil
	}
	// A backstop for the pipes rather than for the process: Wait blocks
	// on the output readers until they close, and a grandchild holding
	// the write end would hold Wait open past the kill above.
	cmd.WaitDelay = killDelay + time.Second

	// Avant, pas après : l'annonce existe pour l'attente qui suit — sans
	// elle, "fpctl provision db" ne dit jamais lui-même qu'il est en train
	// de lancer docker, seule la propre sortie de docker apparaît.
	//
	// NOTICE, pas INFO (le choix d'app.taop.xyz pour ce même paquet) :
	// là-bas la sortie de l'outil est capturée et tue jusqu'à l'échec, donc
	// l'annonce est la seule chose que l'INFO par défaut y masquerait vaut
	// la peine de garder visible ; ici la sortie de l'outil est déjà en
	// direct sur le terminal (fpbuild, docker), donc l'annonce n'a de sens
	// qu'au même niveau par défaut que le reste de la narration de fpctl.
	logs.Notice("$ " + CommandLine(c.Name, c.Args...))

	runErr := cmd.Start()
	if runErr != nil {
		slog.Error(c.Name+" : impossible de démarrer", "erreur", runErr)
		return runErr
	}
	trackGroup(cmd.Process.Pid)
	runErr = cmd.Wait()
	untrackGroup(cmd.Process.Pid)

	// Le contexte annulé (signal) a déjà son propre message
	// (internal/logs.Context, "stopping") : le répéter ici serait un echo,
	// pas une information de plus.
	if runErr != nil && ctx.Err() == nil {
		slog.Error(c.Name+" : terminé en échec", "erreur", runErr)
	}
	return runErr
}

// CommandLine renders a command the way a reader could retype it, quoting
// any argument that would otherwise be ambiguous.
func CommandLine(name string, args ...string) string {
	out := make([]string, 0, len(args)+1)
	out = append(out, name)
	for _, a := range args {
		if a == "" || strings.ContainsAny(a, " \t\"'$&|<>();*?[]#~") {
			out = append(out, strconv.Quote(a))
			continue
		}
		out = append(out, a)
	}
	return strings.Join(out, " ")
}
