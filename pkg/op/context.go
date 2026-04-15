// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/NobleFactor/devlore-cli/pkg/op/sops"
	"go.starlark.net/starlark"
)

// ExecutionContext provides the execution environment for providers, resources, and graphs.
type ExecutionContext struct {
	context.Context // https://pkg.go.dev/context

	// ProgramName identifies the running tool (e.g., "lore", "writ").
	ProgramName string

	// Catalog is the resource catalog for the current execution session. The do layer uses it to shadow Resource
	// results after dispatch. Nil when running without catalog integration (e.g., tests).
	Catalog *ResourceCatalog

	// Data holds tool-provided context: template variables, identities, segment maps, etc. Each tool populates this
	// before calling GraphExecutor.Run().
	Data map[string]any

	// DryRun prevents filesystem modifications when true.
	DryRun bool

	// Platform provides platform abstractions (package manager, service manager) to do providers. Nil when running
	// in environments where host access is not needed (e.g., pure data transforms).
	Platform *Platform

	// RecoverySite is the shared recovery service for archiving and restoring resources during compensation.
	// Instantiated by the executor from Root.
	RecoverySite *RecoverySite

	// Registry is the receiver type registry for the current session. Providers, graphs, and the starlark runtime
	// use it to look up receiver types by name.
	Registry *ReceiverRegistry

	// Results holds the accumulated node results from the current execution. Flow actions (choose, gather) use this
	// to resolve cross-phase promise references in branch nodes.
	Results map[string]any

	// Root provides scoped filesystem operations. All provider I/O goes through this interface.
	// Three implementations: confinedRoot (execution), RootReader (planning), RootReaderWriter (testing).
	// Created by the executor or test runner; closed after execution completes.
	Root Root

	// Sops provides SOPS operations (decryption, signing, verification). Nil when SOPS is not configured (no
	// .sops.yaml found). Receivers access this via p.ExecutionContext().SopsClient.
	Sops *sops.Client

	// Thread is a Starlark execution thread for callable initialization. Created by the executor at execution time.
	// Actions that need to invoke mem.Function resources call Init(ctx.Thread) before Fn().
	Thread starlark.Thread

	// Writer receives user-facing output messages.
	Writer io.Writer

	// mu guards the providers map for concurrent access.
	mu sync.Mutex

	// providers caches lazily-constructed provider instances by name.
	providers map[string]any
}

// NewExecutionContext returns an ExecutionContext with the given root and auto-detected platform.
func NewExecutionContext(root Root) ExecutionContext {
	return ExecutionContext{Root: root, Platform: NewPlatform()}
}

// region EXPORTED METHODS

// region Behaviors

// ActionByName returns a resolved Action for the given dotted action name (e.g., "file.write_text").
//
// The name is split into provider name and method name. The provider must play the action role. The provider instance is
// cached — subsequent calls for the same provider reuse the instance.
//
// Parameters:
//   - name: the dotted action name (e.g., "file.write_text").
//
// Returns:
//   - Action: the resolved action wrapping the provider instance and method.
//   - error: non-nil if the provider is not a registered action, the method doesn't exist, or construction fails.
func (ctx *ExecutionContext) ActionByName(name string) (Action, error) {

	dot := strings.LastIndex(name, ".")
	if dot < 0 {
		return nil, fmt.Errorf("invalid action name %q: no dot", name)
	}

	receiverName := name[:dot]
	methodSnake := name[dot+1:]

	prt, ok := ctx.Registry.ActionByName(receiverName)
	if !ok {
		return nil, fmt.Errorf("unknown action provider: %s", receiverName)
	}

	var method *Method
	for m := range prt.Methods() {
		if CamelToSnake(m.Name()) == methodSnake {
			method = m
			break
		}
	}

	if method == nil {
		return nil, fmt.Errorf("action %q: method %q not found on %q", name, methodSnake, receiverName)
	}

	if _, err := ctx.cachedProvider(prt); err != nil {
		return nil, err
	}

	return newAction(prt, method, name), nil
}

// ModuleByName returns a cached provider instance for the named module, constructing it on first access.
//
// Parameters:
//   - name: the module name (e.g., "file", "ui").
//
// Returns:
//   - any: the provider instance.
//   - error: non-nil if the name is not a registered module or construction fails.
func (ctx *ExecutionContext) ModuleByName(name string) (any, error) {

	prt, ok := ctx.Registry.ModuleByName(name)
	if !ok {
		return nil, fmt.Errorf("unknown module: %s", name)
	}

	return ctx.cachedProvider(prt)
}

// ExecuteSubgraph runs a subgraph using the context's shared results and recovery stack.
//
// This is the entry point for flow providers (Choose, Gather) that need to execute a subgraph
// from within a provider method. It delegates to the executor's subgraph runner.
//
// Parameters:
//   - graph: the root graph (for context access).
//   - sg: the subgraph to execute.
//
// Returns:
//   - any: the terminal node's output value, or nil.
//   - error: non-nil if the subgraph fails.
func (ctx *ExecutionContext) ExecuteSubgraph(graph *Graph, sg *Subgraph) (any, error) {

	e := &GraphExecutor{hooks: NewHookRegistry()}
	stack := NewRecoveryStack()
	if ctx.Results == nil {
		ctx.Results = make(map[string]any)
	}
	return e.executeSubgraph(graph, sg, ctx.Results, stack)
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// cachedProvider returns a cached provider instance for the given type descriptor, constructing it on first access.
//
// The lock is released before calling Construct to avoid deadlock when a provider's constructor calls cachedProvider for
// a sibling. Double-check after construction handles concurrent callers.
//
// Parameters:
//   - prt: the provider receiver type descriptor.
//
// Returns:
//   - any: the provider instance.
//   - error: non-nil if construction fails.
func (ctx *ExecutionContext) cachedProvider(prt ProviderReceiverType) (any, error) {

	name := prt.Name()

	ctx.mu.Lock()
	if p, ok := ctx.providers[name]; ok {
		ctx.mu.Unlock()
		return p, nil
	}
	ctx.mu.Unlock()

	p, err := prt.Construct()(ctx)
	if err != nil {
		return nil, fmt.Errorf("construct provider %s: %w", name, err)
	}

	ctx.mu.Lock()
	if existing, ok := ctx.providers[name]; ok {
		ctx.mu.Unlock()
		return existing, nil
	}
	if ctx.providers == nil {
		ctx.providers = make(map[string]any)
	}
	ctx.providers[name] = p
	ctx.mu.Unlock()

	return p, nil
}

// endregion

// endregion
