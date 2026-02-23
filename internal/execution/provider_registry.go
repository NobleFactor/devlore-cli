// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import "github.com/NobleFactor/devlore-cli/pkg/op"

// ProviderRegistrar is the interface for registering providers, re-exported from pkg/op.
type ProviderRegistrar = op.ProviderRegistrar

// RegisterProvider registers a single provider, re-exported from pkg/op.
var RegisterProvider = op.RegisterProvider

// RegisterAllProviders registers all built-in providers, re-exported from pkg/op.
var RegisterAllProviders = op.RegisterAllProviders
