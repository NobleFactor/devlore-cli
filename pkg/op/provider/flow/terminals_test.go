// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow

import (
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// terminalActivation builds a bare non-graph activation: Transition no-ops without an executor, which is the
// unit-test contract these terminals tolerate by design.
func terminalActivation(t *testing.T) *op.ActivationRecord {
	t.Helper()
	return op.NewActivationRecord(nil, "", &op.RuntimeEnvironment{})
}

// TestComplete_ReturnsOutput pins the early-return terminal: the output passes through untouched.
func TestComplete_ReturnsOutput(t *testing.T) {

	p := &Provider{ProviderBase: op.NewProviderBase(&op.RuntimeEnvironment{})}

	got := p.Complete(terminalActivation(t), map[string]any{"key": "value"})

	m, ok := got.(map[string]any)
	if !ok || m["key"] != "value" {
		t.Errorf("Complete = %#v, want the output passed through", got)
	}
}

// TestDegraded_RendersAndReturnsMessage pins the degraded terminal: the template-form message (op.RenderError: Go templates over Args/kwargs) renders and returns;
// the condition transition is best-effort (a bare activation has no executor to transition).
func TestDegraded_RendersAndReturnsMessage(t *testing.T) {

	p := &Provider{ProviderBase: op.NewProviderBase(&op.RuntimeEnvironment{})}

	got := p.Degraded(terminalActivation(t),
		"disk {{index .Args 0}} at {{.percent}}%", []any{"nearly full"}, map[string]any{"percent": 91})

	if !strings.Contains(got, "disk nearly full at 91%") {
		t.Errorf("Degraded = %q, want the rendered message", got)
	}
}

// TestFailed_RendersAndReturnsMessage pins the failed terminal: the formatted message renders and returns; the
// condition transition is best-effort under a bare activation.
func TestFailed_RendersAndReturnsMessage(t *testing.T) {

	p := &Provider{ProviderBase: op.NewProviderBase(&op.RuntimeEnvironment{})}

	got := p.Failed(terminalActivation(t), "unit {{index .Args 0}} exploded", []any{"alpha"}, nil)

	if !strings.Contains(got, "unit alpha exploded") {
		t.Errorf("Failed = %q, want the rendered message", got)
	}
}
