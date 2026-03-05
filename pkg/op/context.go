package op

import (
	"context"
	"io"
)

// Context provides execution context to actions.
type Context struct {
	context.Context // https://pkg.go.dev/context

	// Catalog is the resource catalog for the current execution session.
	// The action layer uses it to shadow Resource results after dispatch.
	// Nil when running without catalog integration (e.g., tests).
	Catalog *ResourceCatalog

	// Data holds tool-provided context: template variables, SOPS config,
	// identities, segment maps, etc. Each tool populates this before
	// calling GraphExecutor.Run().
	Data map[string]any

	// DryRun prevents filesystem modifications when true.
	DryRun bool

	// Graph is the graph being executed. Flow actions use this to look up
	// phases referenced by their slots (e.g., gather body, choose branch).
	Graph *Graph

	// NodeID is the ID of the currently executing node. Flow actions use
	// this to identify themselves (e.g., gather uses it for proxy context).
	NodeID string

	// Platform provides platform abstractions (package manager, service
	// manager) to action providers. Nil when running in environments
	// where host access is not needed (e.g., pure data transforms).
	Platform *Platform

	// Writer receives user-facing output messages.
	Writer io.Writer
}
