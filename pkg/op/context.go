package op

import (
	"context"
	"io"
	"os"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/op/recovery"
)

// Context provides execution context to actions.
type Context struct {
	context.Context // https://pkg.go.dev/context

	// Root provides OS-enforced chroot-style confinement. All scoped provider I/O goes through this.
	// Opened from the authority boundary directory by the executor; closed after execution completes.
	// Root.Name() returns the authority boundary path.
	Root *os.Root

	// RecoverySite is the shared recovery service for archiving and restoring resources during compensation.
	// Instantiated by the executor from Root.
	RecoverySite *recovery.Site

	// Catalog is the resource catalog for the current execution session. The action layer uses it to shadow Resource
	// results after dispatch. Nil when running without catalog integration (e.g., tests).
	Catalog *ResourceCatalog

	// Data holds tool-provided context: template variables, SOPS config, identities, segment maps, etc. Each tool
	// populates this before calling GraphExecutor.Run().
	Data map[string]any

	// DryRun prevents filesystem modifications when true.
	DryRun bool

	// Graph is the graph being executed. Flow actions use this to look up phases referenced by their slots (e.g.,
	// gather body, choose branch).
	Graph *Graph

	// NodeID is the ID of the currently executing node. Flow actions use this to identify themselves (e.g., gather uses
	// it for proxy context).
	NodeID string

	// Platform provides platform abstractions (package manager, service manager) to action providers. Nil when running
	// in environments where host access is not needed (e.g., pure data transforms).
	Platform *Platform

	// Thread is a Starlark execution thread for callable initialization. Created by the executor at execution time.
	// Actions that need to invoke mem.Callable functions call Init(ctx.Thread) before Fn().
	Thread *starlark.Thread

	// Writer receives user-facing output messages.
	Writer io.Writer
}
