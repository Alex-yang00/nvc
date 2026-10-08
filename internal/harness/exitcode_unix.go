//go:build !windows

package harness

import (
	"os/exec"
	"syscall"
)

// exitCode mirrors a shell: 128+signal when the child was killed by a signal.
func exitCode(ee *exec.ExitError) int {
	if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal())
	}
	return ee.ExitCode()
}
