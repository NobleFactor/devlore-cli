// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package process is the single bridge between os/exec and the runtime environment's status (narration) and result
// (typed payload) channels.
//
// Every provider that shells out goes through Runner so line-splitting, dry-run, cancellation, and error wrapping
// live in exactly one place.
package process

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/NobleFactor/devlore-cli/pkg/result"
	"github.com/NobleFactor/devlore-cli/pkg/status"
)

// Runner executes os/exec commands with stdout and stderr routed through the configured status and result channels.
type Runner struct {
	context context.Context // when canceled, the subprocess receives a kill
	dryRun  bool            // when true, narrate the command but skip execution
	result  result.Sink     // typed-payload sink consumed by Emit
	status  status.Sink     // narration sink for stdout and stderr line streams and pre-execution narration
}

// NewRunner constructs a Runner bound to the given runtime values.
//
// Parameters:
//   - context: cancellation signal for the subprocess; when Done, the subprocess is killed.
//   - dryRun:  when true, narrate the command but skip execution.
//   - result:  typed-payload sink consumed by Emit.
//   - status:  narration sink for stdout and stderr line streams and pre-execution narration.
//
// Returns:
//   - *Runner: the initialized runner.
func NewRunner(context context.Context, dryRun bool, result result.Sink, status status.Sink) *Runner {

	return &Runner{
		context: context,
		dryRun:  dryRun,
		result:  result,
		status:  status,
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

	cmd.Stdout = &stdout
	cmd.Stderr = newLineWriter(r.status.Warn)

	r.bindCancel(cmd)

	if err := cmd.Run(); err != nil {
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

	r.narrate(cmd)
	if r.dryRun {
		return nil
	}

	stdoutLines := newLineWriter(r.status.Note)
	cmd.Stdout = stdoutLines
	cmd.Stderr = newLineWriter(r.status.Warn)
	r.bindCancel(cmd)

	err := cmd.Run()
	stdoutLines.Flush()
	if err != nil {
		return wrapExitError(cmd, err)
	}
	return nil
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

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
		_ = cmd.Cancel()
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
