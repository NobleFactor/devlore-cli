// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package provider_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider"
)

// newTestEnvironment builds a minimal runtime environment for resolving providers in these tests.
//
// Parameters:
//   - `t`: the running test, marked as the helper's caller.
//
// Returns:
//   - `*op.RuntimeEnvironment`: a runtime environment with no providers yet constructed.
func newTestEnvironment(t *testing.T) *op.RuntimeEnvironment {

	t.Helper()

	testApplication := &application.Application{Name: "instance-test"}
	spec := op.NewRuntimeEnvironmentSpec(testApplication.Name).WithApplication(testApplication)
	return op.NewRuntimeEnvironment(context.Background(), spec)
}

// fakeProvider is a no-op provider announced only to exercise provider.Instance against the process registry.
type fakeProvider struct {
	op.ProviderBase
}

// unregisteredProvider is never announced, so provider.Instance must fail to resolve it.
type unregisteredProvider struct {
	op.ProviderBase
}

func init() {
	op.AnnounceProvider(
		reflect.TypeFor[fakeProvider](),
		op.RoleAction,
		func(runtimeEnvironment *op.RuntimeEnvironment) (any, error) {
			return &fakeProvider{ProviderBase: op.NewProviderBase(runtimeEnvironment)}, nil
		},
		map[string]op.MethodMetadata{},
	)
}

// --- Instance ---

// TestInstance_RegisteredProvider_ReturnsCachedSingleton verifies that Instance resolves a registered provider and
// returns the single per-environment instance on every call.
func TestInstance_RegisteredProvider_ReturnsCachedSingleton(t *testing.T) {

	runtimeEnvironment := newTestEnvironment(t)

	first, err := provider.Instance[fakeProvider](runtimeEnvironment)
	if err != nil {
		t.Fatalf("Instance returned an error for a registered provider: %v", err)
	}
	if first == nil {
		t.Fatal("Instance returned a nil provider for a registered type")
	}

	second, err := provider.Instance[fakeProvider](runtimeEnvironment)
	if err != nil {
		t.Fatalf("second Instance call returned an error: %v", err)
	}
	if first != second {
		t.Errorf("Instance returned distinct pointers %p and %p; expected the one per-environment instance", first, second)
	}
}

// TestInstance_UnregisteredType_ReturnsError verifies that Instance errors when no provider is registered for T.
func TestInstance_UnregisteredType_ReturnsError(t *testing.T) {

	runtimeEnvironment := newTestEnvironment(t)

	if _, err := provider.Instance[unregisteredProvider](runtimeEnvironment); err == nil {
		t.Fatal("Instance returned a nil error for an unregistered provider type; expected a resolution error")
	}
}
