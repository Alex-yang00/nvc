//go:build windows

package harness

import "os/exec"

func exitCode(ee *exec.ExitError) int { return ee.ExitCode() }
