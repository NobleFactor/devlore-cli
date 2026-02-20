// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package content

// Provider provides content actions.
//
//devlore:plannable
type Provider struct{}

// Literal returns the provided content as-is.
func (p *Provider) Literal(content []byte) []byte { return content }
