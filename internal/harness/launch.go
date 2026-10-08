package harness

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// Launch runs the agent as a child and waits, so guards can undo config the agent
// persisted during the session (e.g. `/model`). nvc stays resident but idle.
// It returns the exit code to use for nvc itself.
func Launch(p Plan) (int, error) {
	path, err := exec.LookPath(p.Binary)
	if err != nil {
		return 1, err
	}
	for _, g := range p.Guards {
		if err := g.Snapshot(); err != nil {
			// Can't guarantee a clean revert → refuse rather than risk leaving traces.
			return 1, fmt.Errorf("cannot snapshot %s: %w", g.Describe(), err)
		}
	}

	cmd := exec.Command(path, p.Args...)
	cmd.Env = MergedEnv(p)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr

	// The child shares our process group, so the terminal already delivers Ctrl+C / Ctrl+\
	// to it directly; we just must not die from them. SIGTERM/SIGHUP come only to us → forward.
	signal.Ignore(os.Interrupt, syscall.SIGQUIT)
	fwd := make(chan os.Signal, 4)
	signal.Notify(fwd, syscall.SIGTERM, syscall.SIGHUP)

	if err := cmd.Start(); err != nil {
		return 1, err
	}
	go func() {
		for s := range fwd {
			_ = cmd.Process.Signal(s)
		}
	}()
	waitErr := cmd.Wait()
	signal.Stop(fwd)

	for _, g := range p.Guards {
		changed, err := g.Restore()
		switch {
		case err != nil:
			fmt.Fprintf(os.Stderr, "nvc: could not restore %s: %v\n", g.Describe(), err)
		case changed:
			fmt.Fprintf(os.Stderr, "nvc: restored %s (changed during the session)\n", g.Describe())
		}
	}

	if waitErr == nil {
		return 0, nil
	}
	var ee *exec.ExitError
	if errors.As(waitErr, &ee) {
		return exitCode(ee), nil
	}
	return 1, waitErr
}
