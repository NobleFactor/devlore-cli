// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package platform is a data provider — the Starlark surface for
// op.Context.Platform. It holds no independent state; it wraps the Platform
// struct that the executor places on the execution context.
//
// Like all providers, platform is a singleton that follows the lifetime of
// its graph (execution) or script (planning). It receives context at
// construction time via a constructor that accepts a context object by
// reference. The provider reads Platform data from that context — it never
// creates or owns a Platform.
//
// Access type is "both":
//
//   - Immediate: platform.distro returns a string from the local machine's
//     Platform. Useful for single-machine local plans.
//   - Planned: plan.platform.distro returns a promise (Output) that the
//     executor resolves against the target machine's Platform at execution
//     time. This is critical for graphs targeting remote machines — the
//     graph is planned once and can be executed on many targets, each with
//     a different Platform.
//
// Because the provider is read-only (no side effects), it has no action
// methods, no compensation pairs, and no codegen. Concrete manager types
// are private; consumers see only op.PackageManager and op.ServiceManager.
package platform

import (
	"runtime"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// New returns a fully populated Platform for the current OS.
func New() *op.Platform {
	switch runtime.GOOS {
	case "darwin":
		return newDarwin()
	case "linux":
		return newLinux()
	case "windows":
		return newWindows()
	default:
		return newLinux()
	}
}
