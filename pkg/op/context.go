// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"context"
	"io"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/op/sops"
)

// ContextBase provides the execution environment shared by all contexts: immediate-mode scripting
// environments and graph execution alike.
type ContextBase struct {
	context.Context // https://pkg.go.dev/context

	// Root provides scoped filesystem operations. All provider I/O goes through this interface.
	// Three implementations: confinedRoot (execution), RootReader (planning), RootReaderWriter (testing).
	// Created by the executor or test runner; closed after execution completes.
	Root Root

	// ProgramName identifies the running tool (e.g., "lore", "writ").
	ProgramName string

	// DryRun prevents filesystem modifications when true.
	DryRun bool

	// Platform provides platform abstractions (package manager, service manager) to action providers. Nil when running
	// in environments where host access is not needed (e.g., pure data transforms).
	Platform *Platform

	// Writer receives user-facing output messages.
	Writer io.Writer

	// SopsClient provides SOPS operations (decryption, signing, verification). Nil when SOPS is not configured (no
	// .sops.yaml found). Receivers access this via p.Context().SopsClient.
	SopsClient *sops.Client

	// Data holds tool-provided context: template variables, identities, segment maps, etc. Each tool populates this
	// before calling GraphExecutor.Run().
	Data map[string]any
}

// NewContextBase returns a [ContextBase] with the [Platform] auto-detected from the host OS.
//
// Parameters:
//   - root: the filesystem root for scoped I/O operations.
//
// Returns:
//   - ContextBase: a context base with platform initialized.
func NewContextBase(root Root) ContextBase {
	return ContextBase{
		Platform: NewPlatform(),
		Root:     root,
	}
}

// Context provides execution context to actions.
type Context struct {
	ContextBase

	// RecoverySite is the shared recovery service for archiving and restoring resources during compensation.
	// Instantiated by the executor from Root.
	RecoverySite *RecoverySite

	// Catalog is the resource catalog for the current execution session. The action layer uses it to shadow Resource
	// results after dispatch. Nil when running without catalog integration (e.g., tests).
	Catalog *ResourceCatalog

	// Graph is the graph being executed. Flow actions use this to look up phases referenced by their slots (e.g.,
	// gather body, choose branch).
	Graph *Graph

	// NodeID is the ID of the currently executing node. Flow actions use this to identify themselves (e.g., gather uses
	// it for proxy context).
	NodeID string

	// Thread is a Starlark execution thread for callable initialization. Created by the executor at execution time.
	// Actions that need to invoke mem.Callable functions call Init(ctx.Thread) before Fn().
	Thread *starlark.Thread

	// Results holds the accumulated node results from the current execution. Flow actions (choose, gather) use this
	// to resolve cross-phase promise references in branch nodes.
	Results map[string]any
}
