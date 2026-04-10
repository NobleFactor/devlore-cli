// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package service

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// NewResource creates a service.Resource from a value.
//
// Parameters:
//   - ctx: the execution context.
//   - value: expected to be a string service name.
//
// Returns:
//   - *Resource: the initialized resource.
//   - error: if value is not a string.
func NewResource(ctx *op.ExecutionContext, value any) (*Resource, error) {

	name, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("service.Resource: expected string name, got %T", value)
	}

	return &Resource{
		ResourceBase: op.NewResourceBase(ctx, "svc:"+name),
		Name:         name,
	}, nil
}

// Resource represents a system service.
type Resource struct {
	op.ResourceBase
	Name string
}

// String returns a compact JSON representation of the resource.
func (r *Resource) String() string { return r.Format(r) }

// Tombstone holds service-specific compensation state.
type Tombstone struct {
	op.TombstoneBase
	Name       string
	WasRunning bool
	WasEnabled bool
}
