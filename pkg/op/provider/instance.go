// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package provider

import (
	"fmt"
	"reflect"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Instance returns the runtime environment's single instance of provider type T, constructing it on first access.
//
// It is the generic form of [op.RuntimeEnvironment.ProviderByType]: it keys the lookup by reflect.TypeFor[T]() — the
// struct form used at registration — and asserts the cached value to *T. A collaborating provider writes
// provider.Instance[file.Provider](runtimeEnvironment) and receives the same *file.Provider that every other
// consumer in the environment holds, built on first access and cached for the environment's lifetime.
//
// Parameters:
//   - `runtimeEnvironment`: the runtime environment whose provider cache supplies the instance.
//
// Returns:
//   - `*T`: the one environment-scoped instance of T, constructed on first access.
//   - `error`: non-nil if T is not a registered provider, the cached value is not *T, or construction fails.
func Instance[T any](runtimeEnvironment *op.RuntimeEnvironment) (*T, error) {

	value, err := runtimeEnvironment.ProviderByType(reflect.TypeFor[T]())
	if err != nil {
		return nil, err
	}

	instance, ok := value.(*T)
	if !ok {
		return nil, fmt.Errorf("provider for %s has unexpected type %T", reflect.TypeFor[T](), value)
	}

	return instance, nil
}
