// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package git

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// TestDiscoverResource_ErrorPathReturnsANilInterface pins #807: the exported constructor returns a literal nil
// [Resource] beside its error, never a non-nil interface wrapping a nil struct pointer. A caller that checks the
// resource before the error must see nothing there.
func TestDiscoverResource_ErrorPathReturnsANilInterface(t *testing.T) {

	resource, err := DiscoverResource(&op.RuntimeEnvironment{}, struct{}{})
	if err == nil {
		t.Fatalf("DiscoverResource(struct{}{}) = %#v, want an error for an unsupported value type", resource)
	}
	if resource != nil {
		t.Fatalf("DiscoverResource on the error path = %#v (%T), want a nil interface", resource, resource)
	}
}
