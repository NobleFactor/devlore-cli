// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Compile-time interface satisfaction checks.
var (
	_ op.HostProvider           = (*hostAdapter)(nil)
	_ op.PackageManagerProvider = (*pmAdapter)(nil)
	_ op.ServiceManagerProvider = (*smAdapter)(nil)
)

func TestNewHostProviderNil(t *testing.T) {
	if got := NewHostProvider(nil); got != nil {
		t.Errorf("NewHostProvider(nil) = %v, want nil", got)
	}
}
