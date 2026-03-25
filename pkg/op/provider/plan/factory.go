// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package plan

import (
	"reflect"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Params maps Go method names to Starlark parameter name lists.
var Params = op.MethodParams{
	"Complete":  {"output?"},
	"Degraded":  {"format", "*args", "**kwargs"},
	"Fatal":     {"format", "*args", "**kwargs"},
	"WaitUntil": {"target", "predicate", "timeout", "interval?"},
	"Gather":    {"*promises"},
	"Choose":    {"when", "then"},
}

// Factory is the ReceiverFactory for the plan provider.
var Factory op.ReceiverFactory = &receiverFactory{}

// receiverFactory implements op.ReceiverFactory for the plan provider.
type receiverFactory struct{}

// ReceiverName returns the Starlark receiver name.
func (f *receiverFactory) ReceiverName() string { return "plan" }

// GetOrCreateProvider returns nil — the plan provider is constructed per-graph,
// not per-context.
func (f *receiverFactory) GetOrCreateProvider(_ op.Context) op.ContextProvider { return nil }

// ProviderType returns the reflect.Type of the plan Provider.
func (f *receiverFactory) ProviderType() reflect.Type {
	return reflect.TypeOf((*Provider)(nil)).Elem()
}

// Register registers receiver params for the plan provider.
func (f *receiverFactory) Register(_ *op.ActionRegistry, _ op.Context) {
	op.RegisterReceiverParams(f, Params)
}

// NewExecuting creates the plan receiver bound to a graph.
// The plan provider is an executing receiver — its methods run immediately
// during script evaluation to create graph nodes.
func (f *receiverFactory) NewExecuting(graph *op.Graph, project string, reg *op.ActionRegistry) *op.ExecutingReceiver {
	p := NewProvider(graph, project, reg)
	return op.WrapProviderInExecutingReceiver(Factory, p)
}

func init() {
	op.AnnounceReceiver(Factory)
}
