// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow

import (
	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// flowProvider is the provider descriptor for flow control actions.
// Handwritten — same structure as generated provider descriptors.
type flowProvider struct{}

func (p *flowProvider) Name() string { return "flow" }

func (p *flowProvider) Register(reg *op.ActionRegistry, _ op.Context) {
	reg.Register(&Choose{})
	reg.Register(&Gather{})
	reg.Register(&Elevate{})
	reg.Register(&WaitUntil{})
	reg.Register(&Complete{})
	reg.Register(&Degraded{})
	reg.Register(&Fatal{})
}

// NewPlanned implements op.PlannedProvider.
func (p *flowProvider) NewPlanned(graph *op.Graph, project string, reg *op.ActionRegistry) starlark.Value {
	return NewFlowPlan(graph, project, reg)
}

func init() {
	op.Announce(&flowProvider{})
}
