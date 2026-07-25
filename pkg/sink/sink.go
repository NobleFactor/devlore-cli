// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package sink defines the byte-out endpoint contract used by [pkg/status] (categorized narration)
// and [pkg/result] (structured result emission).
//
// A [Sink] is an [io.WriteCloser] that knows whether it's writing to a TTY. Higher-level wrappers
// (status.Narrator, result.Pipeline) consume a Sink and decide what to write through it; the Sink
// just delegates writes to its underlying [io.Writer] and reports TTY-ness derived at construction.
//
// Six convenience constructors ship in this package, each pre-wiring the underlying writer:
//
//   - [Stderr] / [Stdout] — the two standard process streams; TTY-awareness derived from the fd.
//   - [Capture] — returns a Sink + the [bytes.Buffer] it wraps so tests can assert on captured bytes.
//   - [Discard] — wraps [io.Discard]; every write succeeds and goes nowhere.
//   - [File] — opens the named path; the returned Sink owns the file and closes it on [Sink.Close].
//   - [Tee] — fans out writes to multiple sinks; closes all on [Sink.Close].
//
// The generic [New] constructor wraps any [io.Writer]; the caller owns the writer's lifecycle and
// the returned Sink's [Sink.Close] is a no-op. TTY-ness is derived from the writer when possible
// (writer is an [*os.File] pointing at a terminal); false otherwise.
package sink

import (
	"bytes"
	"errors"
	"io"
	"os"

	"golang.org/x/term"
)

// Sink is the byte-out contract.
//
// Wraps an [io.Writer] with [io.Closer] (default no-op for sinks that don't own their writer) and a [Sink.IsTTY] query
// so higher-level wrappers can decide whether to emit ANSI color codes.
//
// Sinks shipped by this package are constructed via the convenience functions ([Stderr], [Stdout], [Capture],
// [Discard], [File], [Tee]) or via the generic [New]. Implementations are unexported; callers always interact with the
// interface.
type Sink interface {
	io.WriteCloser

	// IsTTY reports whether the underlying writer is connected to a terminal.
	//
	// Computed at construction from the writer (true iff the writer is an [*os.File] whose fd passes
	// [term.IsTerminal]); always false for [Capture], [Discard], [File], and [Tee], and false for [New] when the
	// wrapped writer isn't a terminal-backed [*os.File].
	IsTTY() bool
}

// region EXPORTED FUNCTIONS

// region Behaviors

// Capture returns a Sink whose writes accumulate into an in-memory buffer.
//
// Test fixture. The returned Sink does not own anything that needs cleanup; [Sink.Close] is a no-op. IsTTY always
// returns false.
//
// Returns:
//   - Sink: the buffer-backed sink, ready to install on a status.Narrator or result.Pipeline.
//   - *bytes.Buffer: the underlying buffer; pass to test assertions.
func Capture() (Sink, *bytes.Buffer) {

	buf := &bytes.Buffer{}
	return &impl{w: buf, isTTY: false}, buf
}

// Discard returns a Sink whose writes go nowhere.
//
// Wraps [io.Discard]; every write succeeds, every byte is dropped. [Sink.Close] is a no-op. IsTTY
// always returns false. The cli boundary picks this when `--silent` is set.
//
// Returns:
//   - Sink: the no-op sink.
func Discard() Sink {

	return &impl{w: io.Discard, isTTY: false}
}

// File opens the named path for writing and returns a Sink that owns the file's lifecycle.
//
// The Sink's [Sink.Close] closes the underlying [os.File], so the caller pairs construction with `defer iox.Close(&err,
// sink)` to release the fd. IsTTY returns false (regular files are not terminals).
//
// Parameters:
//   - path: the filesystem path to open. The file is created or truncated via [os.Create].
//
// Returns:
//   - Sink: the file-backed sink.
//   - error: non-nil if the file could not be opened.
func File(path string) (Sink, error) {

	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}

	return &impl{w: f, close: f.Close, isTTY: false}, nil
}

// New wraps an arbitrary [io.Writer] in a Sink.
//
// The returned Sink does not own the writer's lifecycle — [Sink.Close] is a no-op. The caller is responsible for
// closing the writer if it requires cleanup. IsTTY is derived: true iff the writer is an [*os.File] whose fd passes
// [term.IsTerminal]; false otherwise (including all non-file writers — buffers, pipes, network connections).
//
// Parameters:
//   - w: the writer to wrap. Must not be nil.
//
// Returns:
//   - Sink: the writer-backed sink.
func New(w io.Writer) Sink {

	return &impl{w: w, isTTY: detectTTY(w)}
}

// Stderr returns a Sink wrapping [os.Stderr]. TTY-awareness is derived from the fd.
//
// The cli boundary's default narration sink. [Sink.Close] is a no-op — stderr is owned by the OS,
// never closed by user code.
//
// Returns:
//   - Sink: the stderr-backed sink.
func Stderr() Sink {

	return &impl{w: os.Stderr, isTTY: detectTTY(os.Stderr)}
}

// Stdout returns a Sink wrapping [os.Stdout]. TTY-awareness is derived from the fd.
//
// The cli boundary's default result-emission sink. [Sink.Close] is a no-op — stdout is owned by the
// OS, never closed by user code.
//
// Returns:
//   - Sink: the stdout-backed sink.
func Stdout() Sink {

	return &impl{w: os.Stdout, isTTY: detectTTY(os.Stdout)}
}

// Tee returns a Sink that fans writes out to every supplied sink in order.
//
// [Sink.Close] cascades to every supplied sink and joins their errors via [errors.Join].
//
// IsTTY always returns false — even if every supplied sink is TTY-aware, fanning out implies "this
// is being captured/logged in addition to displayed," which usually means decoration should be
// suppressed. Callers that need TTY-aware fan-out can build a custom impl.
//
// Parameters:
//   - sinks: the destinations to fan out to. Empty slice produces a Sink whose writes are no-ops.
//
// Returns:
//   - Sink: the fan-out sink.
func Tee(sinks ...Sink) Sink {

	writers := make([]io.Writer, len(sinks))
	for i, s := range sinks {
		writers[i] = s
	}

	closeFn := func() error {
		var errs []error
		for _, s := range sinks {
			if err := s.Close(); err != nil {
				errs = append(errs, err)
			}
		}
		return errors.Join(errs...)
	}

	return &impl{w: io.MultiWriter(writers...), close: closeFn, isTTY: false}
}

// endregion

// endregion

// region UNEXPORTED TYPES

// impl is the canonical [Sink] implementation: a writer plus an optional close function plus a
// TTY-awareness flag computed at construction.
//
// All exported constructors return *impl boxed as the [Sink] interface; callers never reference impl
// directly. The pattern lets the package ship one implementation while exposing many constructor
// variants without proliferating types.
type impl struct {
	w     io.Writer
	close func() error // nil iff the sink does not own its writer
	isTTY bool
}

// region Interface guards

var _ Sink = (*impl)(nil)

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// Close releases the underlying writer if the sink owns it; otherwise no-op.
//
// Returns:
//   - error: the result of the owned writer's Close (or nil if the sink doesn't own its writer).
func (s *impl) Close() error {

	if s.close == nil {
		return nil
	}
	return s.close()
}

// IsTTY returns the TTY-awareness flag computed at construction.
//
// Returns:
//   - bool: true iff the wrapped writer is a terminal-backed [*os.File].
func (s *impl) IsTTY() bool {

	return s.isTTY
}

// Write delegates to the wrapped writer.
//
// Parameters:
//   - p: the bytes to write.
//
// Returns:
//   - int: the count of bytes successfully written.
//   - error: any write error from the underlying writer.
func (s *impl) Write(p []byte) (int, error) {

	return s.w.Write(p)
}

// endregion

// endregion

// endregion

// region HELPER FUNCTIONS

// region Behaviors

// detectTTY returns true iff w is an [*os.File] whose file descriptor passes [term.IsTerminal].
//
// Falls back to false for all non-file writers (buffers, pipes, network connections, multi-writers)
// — those cannot be terminals, so the answer is unconditionally false.
//
// Parameters:
//   - w: the writer to probe.
//
// Returns:
//   - bool: true iff w is a terminal-backed file.
func detectTTY(w io.Writer) bool {

	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// endregion

// endregion
