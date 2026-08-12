// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"strings"
	"sync/atomic"
)

// nodeCounter provides unique node IDs across all plan bindings.
var nodeCounter uint64

// GenerateNodeID creates a unique node ID with the given prefix and components.
func GenerateNodeID(prefix string, components ...string) string {
	id := atomic.AddUint64(&nodeCounter, 1)
	if len(components) > 0 {
		return fmt.Sprintf("%s-%s-%d", prefix, strings.Join(components, "-"), id)
	}
	return fmt.Sprintf("%s-%d", prefix, id)
}
