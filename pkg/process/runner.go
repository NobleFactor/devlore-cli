// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package process is the single bridge between os/exec and the runtime environment's status (narration) and result
// (typed payload) channels.
//
// Every provider that shells out goes through Runner so line-splitting, dry-run, cancellation, and error wrapping
// live in exactly one place.
package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"

	"github.com/NobleFactor/devlore-cli/pkg/result"
	"github.com/NobleFactor/devlore-cli/pkg/status"
)

// Runner executes os/exec commands with stdout and stderr routed through the configured status and result channels.
type Runner struct {
	context context.Context  // when canceled, the subprocess receives a kill
	dryRun  bool             // when true, narrate the command but skip execution
	result  *result.Pipeline // typed-payload pipeline consumed by Emit
	status  *status.Narrator // narrator for stdout and stderr line streams and pre-execution narration
}

// NewRunner constructs a Runner bound to the given runtime values.
//
// Parameters:
//   - ctx:        cancellation signal for the subprocess; when Done, the subprocess is killed.
//   - dryRun:     when true, narrate the command but skip execution.
//   - pipeline:   typed-payload pipeline consumed by Emit.
//   - narrator:   narrator for stdout and stderr line streams and pre-execution narration.
//
// Returns:
//   - *Runner: the initialized runner.
func NewRunner(ctx context.Context, dryRun bool, pipeline *result.Pipeline, narrator *status.Narrator) *Runner {

	return &Runner{
		context: ctx,
		dryRun:  dryRun,
		result:  pipeline,
		status:  narrator,
	}
}

// region EXPORTED METHODS

// region Behaviors

// Fallible actions

// Capture executes cmd, returning stdout bytes verbatim and streaming stderr through ctx.Status.Warn.
//
// In dry-run, narrates the command and returns nil bytes with a nil error.
//
// Parameters:
//   - cmd: the prepared exec.Cmd; its Stdout, Stderr, and Cancel fields are overwritten by the runner.
//
// Returns:
//   - []byte: the captured stdout (nil in dry-run).
//   - error: a wrapped exit-error carrying the command path and exit code on non-zero exit; full stderr remains
//     available to the caller via the streamed status messages.
func (r *Runner) Capture(cmd *exec.Cmd) ([]byte, error) {

	r.narrate(cmd)

	if r.dryRun {
		return nil, nil
	}

	var stdout bytes.Buffer
	stderrLines := newLineWriter(r.status.Warn)

	cmd.Stdout = &stdout
	cmd.Stderr = stderrLines

	r.bindCancel(cmd)

	err := cmd.Run()
	stderrLines.Flush()
	if err != nil {
		return nil, wrapExitError(cmd, err)
	}

	return stdout.Bytes(), nil
}

// Emit captures stdout, applies parse to produce a typed value, then forwards the value to ctx.Result.Emit.
//
// Stderr streams through ctx.Status.Warn. In dry-run, narrates the command and returns nil without invoking parse.
//
// Parameters:
//   - cmd: the prepared exec.Cmd
//   - parse: converts captured stdout bytes into the typed value forwarded to the result sink
//
// Returns:
//   - error: a wrapped exit-error, parse error, or sink error; nil on success
func (r *Runner) Emit(cmd *exec.Cmd, parse func([]byte) (any, error)) error {

	out, err := r.Capture(cmd)
	if err != nil {
		return err
	}
	if r.dryRun {
		return nil
	}
	value, err := parse(out)
	if err != nil {
		return fmt.Errorf("process: parse %s output: %w", cmd.Path, err)
	}
	return r.result.Emit(value)
}

// Run executes cmd, streaming stdout through ctx.Status.Note and stderr through ctx.Status.Warn line-by-line.
//
// In dry-run, narrates the command and returns nil without launching it.
//
// Parameters:
//   - cmd: the prepared exec.Cmd; its Stdout, Stderr, and Cancel fields are overwritten by the runner.
//
// Returns:
//   - error: a wrapped exit-error carrying the command path and exit code on non-zero exit; full stderr remains
//     available to the caller via the streamed status messages.
func (r *Runner) Run(cmd *exec.Cmd) error {
	return r.relay(cmd, r.status.Warn)
}

// RunNarrating executes cmd, streaming both its stdout and its stderr through ctx.Status.Note line-by-line.
//
// For a child that narrates on stderr, as every program on the shared root does (10-command-line-interface.md §5):
// relayed by [Runner.Run], its story reads as a run of warnings, and relayed here it reads as what it is. Whether the
// child failed is its exit status's to say, not its stderr's. In dry-run, narrates the command and returns nil
// without launching it.
//
// Parameters:
//   - cmd: the prepared exec.Cmd; its Stdout, Stderr, and Cancel fields are overwritten by the runner.
//
// Returns:
//   - error: a wrapped exit-error carrying the command path and exit code on non-zero exit.
func (r *Runner) RunNarrating(cmd *exec.Cmd) error {
	return r.relay(cmd, r.status.Note)
}

// Stream executes cmd, writing its stdout to `stdout` as it arrives and handing each line of its stderr to `stderr`.
//
// The seam the in-process broker frames a command from ([op.InProcessBroker]): bytes on stdout become output frames
// as they are written, lines on stderr become narration frames, and nothing is rendered here. Like [Runner.Run], it
// narrates the command first, skips it in dry-run, and kills it when the runner's context ends; unlike it, the
// child's exit status is the caller's to read from cmd.ProcessState, so a non-zero exit is not an error here.
//
// Parameters:
//   - cmd: the prepared exec.Cmd; its Stdout, Stderr, and Cancel fields are overwritten by the runner.
//   - stdout: receives the child's stdout as it is written.
//   - stderr: receives each line of the child's stderr, without its newline, flushed at exit.
//
// Returns:
//   - bool: true when the child was launched; false in dry-run, where nothing ran.
//   - error: non-nil only when the child could not be launched or was killed; a non-zero exit is not an error.
func (r *Runner) Stream(cmd *exec.Cmd, stdout io.Writer, stderr func(string)) (bool, error) {

	r.narrate(cmd)
	if r.dryRun {
		return false, nil
	}

	stderrLines := newLineWriter(stderr)
	cmd.Stdout = stdout
	cmd.Stderr = stderrLines
	r.bindCancel(cmd)

	err := cmd.Run()
	stderrLines.Flush()

	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return true, fmt.Errorf("process: %s: %w", cmd.Path, err)
	}
	return true, nil
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// Fallible actions

// relay executes cmd, streaming stdout through ctx.Status.Note and stderr through `stderr`, line by line.
//
// [os/exec] copies the two streams in two goroutines, so the lines are handed to the narrator one at a time, whichever
// stream each came from: a narrator is not required to be safe for concurrent use, and one over a buffer is not. Both
// streams are flushed when the child exits, so a last line without its newline still reaches the narrator.
//
// Parameters:
//   - cmd: the prepared exec.Cmd; its Stdout, Stderr, and Cancel fields are overwritten by the runner.
//   - stderr: the narration each line of the child's stderr is relayed through.
//
// Returns:
//   - error: a wrapped exit-error carrying the command path and exit code on non-zero exit.
func (r *Runner) relay(cmd *exec.Cmd, stderr func(string)) error {

	r.narrate(cmd)
	if r.dryRun {
		return nil
	}

	var narrating sync.Mutex
	stdoutLines := newLineWriter(oneAtATime(&narrating, r.status.Note))
	stderrLines := newLineWriter(oneAtATime(&narrating, stderr))
	cmd.Stdout = stdoutLines
	cmd.Stderr = stderrLines
	r.bindCancel(cmd)

	err := cmd.Run()
	stdoutLines.Flush()
	stderrLines.Flush()
	if err != nil {
		return wrapExitError(cmd, err)
	}
	return nil
}

// Actions

// bindCancel wires the runtime environment's context cancellation to the subprocess so a ctx cancel kills it.
//
// Parameters:
//   - cmd: the exec.Cmd whose Cancel hook is installed
func (r *Runner) bindCancel(cmd *exec.Cmd) {

	if r.context == nil {
		return
	}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Kill()
	}
	go func() {
		<-r.context.Done()
		_ = cmd.Cancel() //nolint:errcheck // best-effort cancellation; the subprocess may already have exited
	}()
}

// narrate emits a single "$ <command> <args...>" line to status before the command runs so audit trails always show
// what was launched.
//
// Parameters:
//   - cmd: the exec.Cmd whose argv is rendered into the narration line
func (r *Runner) narrate(cmd *exec.Cmd) {

	if r.status == nil {
		return
	}
	prefix := "$ "
	if r.dryRun {
		prefix = "[dry-run] $ "
	}
	r.status.Note(prefix + cmd.String())
}

// endregion

// endregion

// lineWriter is an io.Writer that buffers partial writes and forwards complete newline-terminated lines to the
// supplied callback. The only line-splitter in the package.
type lineWriter struct {
	emit func(string) // forwarded each complete line, without its trailing newline
	mu   sync.Mutex
	buf  []byte // buffered partial line awaiting a newline
}

// newLineWriter constructs a lineWriter that forwards each complete line to emit.
//
// Parameters:
//   - emit: callback invoked once per complete line
//
// Returns:
//   - *lineWriter: the initialized writer
func newLineWriter(emit func(string)) *lineWriter {

	return &lineWriter{emit: emit}
}

// region EXPORTED METHODS

// region Behaviors

// Fallible actions

// Write satisfies io.Writer: it appends p to the internal buffer, emitting each newline-terminated line to the
// configured callback.
//
// Parameters:
//   - p: the bytes to absorb
//
// Returns:
//   - int: always len(p)
//   - error: always nil
func (l *lineWriter) Write(p []byte) (int, error) {

	l.mu.Lock()
	defer l.mu.Unlock()

	l.buf = append(l.buf, p...)
	for {
		i := bytes.IndexByte(l.buf, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimRight(string(l.buf[:i]), "\r")
		l.buf = l.buf[i+1:]
		if l.emit != nil {
			l.emit(line)
		}
	}
	return len(p), nil
}

// Actions

// Flush emits any buffered partial line. Call after the subprocess exits.
func (l *lineWriter) Flush() {

	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.buf) == 0 {
		return
	}
	if l.emit != nil {
		l.emit(strings.TrimRight(string(l.buf), "\r"))
	}
	l.buf = nil
}

// endregion

// endregion
