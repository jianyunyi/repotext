//go:build !linux

package runner

import "os/exec"

func configureProcessCancellation(_ *exec.Cmd) {}
