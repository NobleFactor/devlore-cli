// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import "io"

// BindingConfig holds configuration for constructing Starlark bindings.
// Passed to ImmediateFactory functions so immediate receivers can write
// output and identify the running program.
type BindingConfig struct {
	// Writer is the output destination for immediate receivers (e.g., ui.note).
	Writer io.Writer

	// ProgramName identifies the running tool (e.g., "lore", "writ").
	ProgramName string

	// Color enables ANSI color codes in output.
	Color bool

	// WorkDir is the working directory for providers that operate on files.
	// If empty, providers should default to the current working directory.
	WorkDir string

	// Platform provides platform abstractions (package manager, service manager)
	// for providers that need them in immediate mode.
	Platform *Platform
}
