// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow

import (
	"reflect"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// flowProvider is the provider descriptor for flow control actions.
// Handwritten — same structure as generated provider descriptors.
// Flow actions are special-cased: they have no backing provider struct.
type flowProvider struct{}

func (p *flowProvider) ReceiverName() string { return "flow" }

func (p *flowProvider) GetOrCreateProvider(_ op.Context) op.ContextProvider { return nil }

func (p *flowProvider) ProviderType() reflect.Type {
	return reflect.TypeOf((*flowProvider)(nil)).Elem()
}

func (p *flowProvider) Register(reg *op.ActionRegistry, _ op.Context) {
	reg.Register(&Choose{})
	reg.Register(&Gather{})
	reg.Register(&Elevate{})
	reg.Register(&WaitUntil{})
	reg.Register(&Complete{})
	reg.Register(&Degraded{})
	reg.Register(&Fatal{})
}

// NewPlanning implements op.PlanningReceiverFactory.
func (p *flowProvider) NewPlanning(graph *op.Graph, project string, reg *op.ActionRegistry) starlark.Value {
	return NewFlowPlan(graph, project, reg)
}

func init() {
	op.Announce(&flowProvider{})
}
