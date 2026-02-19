// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"fmt"
	"strings"
	"sync/atomic"
)

// nodeCounter provides unique node IDs across all plan bindings.
var nodeCounter uint64

// generateNodeID creates a unique node ID with the given prefix and components.
func generateNodeID(prefix string, components ...string) string {
	id := atomic.AddUint64(&nodeCounter, 1)
	if len(components) > 0 {
		return fmt.Sprintf("%s-%s-%d", prefix, strings.Join(components, "-"), id)
	}
	return fmt.Sprintf("%s-%d", prefix, id)
}
