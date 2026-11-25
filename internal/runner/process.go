package runner

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"
)

type Result struct {
	ExitCode  int           `json:"exit_code"`
	Duration  time.Duration `json:"duration_ns"`
	Output    []byte        `json:"output,omitempty"`
	Truncated bool          `json:"truncated"`
}
type ProcessRunner struct {
	MaxOutput int
	Timeout   time.Duration
}

// Run executes a fixed executable and argument list without a shell. This is a local
// development adapter, not an isolation boundary for untrusted repositories. Output
// is bounded while being collected, avoiding unbounded memory use from noisy programs.
func (r ProcessRunner) Run(ctx context.Context, dir string, env []string, name string, args ...string) (Result, error) {
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	max := r.MaxOutput
	if max <= 0 {
		max = 2 << 20
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	configureProcessCancellation(cmd)
	cmd.Dir, cmd.Env = dir, env
	var buf limitedBuffer
	buf.limit = max
	started := time.Now()
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	res := Result{ExitCode: 0, Duration: time.Since(started), Output: buf.buf.Bytes(), Truncated: buf.truncated}
	if cmd.ProcessState != nil {
		res.ExitCode = cmd.ProcessState.ExitCode()
	}
	if ctx.Err() != nil {
		return res, ctx.Err()
	}
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return res, nil
		}
		return res, err
	}
	return res, nil
}

type limitedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := b.limit - b.buf.Len()
	if n > remaining {
		b.truncated = true
	}
	if remaining > 0 {
		take := n
		if take > remaining {
			take = remaining
		}
		_, _ = b.buf.Write(p[:take])
	}
	return n, nil
}
