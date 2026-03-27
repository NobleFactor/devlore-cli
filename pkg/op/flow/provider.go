// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow

import (
	"reflect"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// flowProvider is the provider descriptor for flow control actions.
// Handwritten — same structure as generated provider descriptors.
// Flow actions are special-cased: they have no backing provider struct.
type flowProvider struct{}

func (p *flowProvider) MethodParams() map[string][]string         { return nil }
func (p *flowProvider) MethodParamsFor(_ string) []string         { return nil }
func (p *flowProvider) ReceiverName() string                      { return "flow" }

func (p *flowProvider) GetOrCreateProvider(_ op.Context) op.ContextProvider { return nil }

func (p *flowProvider) ProviderType() reflect.Type {
	return reflect.TypeOf((*flowProvider)(nil)).Elem()
}

func (p *flowProvider) Register(_ op.Context, reg *op.ReceiverRegistry) {
	reg.Register(&Choose{})
	reg.Register(&Gather{})
	reg.Register(&Elevate{})
	reg.Register(&WaitUntil{})
	reg.Register(&Complete{})
	reg.Register(&Degraded{})
	reg.Register(&Fatal{})
}

func init() {
	op.AnnounceReceiver(&flowProvider{})
}
