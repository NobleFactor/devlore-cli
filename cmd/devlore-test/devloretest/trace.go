// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package devloretest

import (
	"fmt"
	"sync"

	"go.starlark.net/starlark"
)

// Tracer collects trace entries during Starlark script execution.
// When enabled, it records position and expression info via the
// thread's Print handler and captures plan.* invocations.
type Tracer struct {
	mu      sync.Mutex
	entries []string
	enabled bool
}

// NewTracer creates a Tracer. If enabled is false, all operations are no-ops.
func NewTracer(enabled bool) *Tracer {
	return &Tracer{enabled: enabled}
}

// Enabled returns whether tracing is active.
func (tr *Tracer) Enabled() bool {
	return tr.enabled
}

// Record adds a trace entry.
func (tr *Tracer) Record(format string, args ...any) {
	if !tr.enabled {
		return
	}
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.entries = append(tr.entries, fmt.Sprintf(format, args...))
}

// RecordThread logs the current thread position.
func (tr *Tracer) RecordThread(thread *starlark.Thread, msg string) {
	if !tr.enabled {
		return
	}
	pos := ""
	if stack := thread.CallStack(); len(stack) > 0 {
		frame := stack[len(stack)-1]
		pos = frame.Pos.String()
	}
	tr.Record("%s: %s", pos, msg)
}

// Entries returns a copy of all recorded trace entries.
func (tr *Tracer) Entries() []string {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	out := make([]string, len(tr.entries))
	copy(out, tr.entries)
	return out
}

// PrintHandler returns a starlark.Thread.Print function that captures
// print() output as trace entries and logs them.
func (tr *Tracer) PrintHandler() func(*starlark.Thread, string) {
	return func(thread *starlark.Thread, msg string) {
		if tr.enabled {
			tr.RecordThread(thread, msg)
		}
	}
}
