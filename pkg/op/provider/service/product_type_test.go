// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package service

import (
	"testing"
)

// TestReceipt_RecordsTheAnnouncedProductType pins the identity a receipt records for its result.
//
// A receipt records its product type at commit and resolves it back on restore through an index built from
// every action method's DECLARED result type. The two are derived by different code from different types, so
// they agree only if both name the announced identity.
//
// Before sealing they agreed by construction: the declared return was `*service.Resource` and the value's
// dynamic type was `*service.Resource`. Sealing separates them — the declared return is the interface while the
// value is `*service.resource` — so a recorded id taken from the Go type would name a struct no method
// declares, the index would miss, and `retypeStampedResult` would swallow that with a bare `return`, leaving a
// resumed result as its raw URI string rather than a resource.
//
// Asserting that a resume merely succeeds would prove nothing against that failure path. This asserts the
// recorded id itself.
func TestReceipt_RecordsTheAnnouncedProductType(t *testing.T) {

	runtimeEnvironment := newTestRuntimeEnvironment(t)

	r, err := NewResource(runtimeEnvironment, "", "nginx")
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	// The announced identity: the interface's canonical type id, which is what the URI fragment carries and
	// what the product-type index is keyed by.
	const want = "github.com/NobleFactor/devlore-cli/pkg/op/provider/service.Resource"

	if got := r.ResourceType(); got != want {
		t.Errorf("ResourceType() = %q, want %q — the fragment must name the interface, not the struct", got, want)
	}

	if uri := r.URI(); uri != "tag:devlore.noblefactor.com,2026-01-01:svc:nginx#"+want {
		t.Errorf("URI() = %q, want the fragment to name %q", uri, want)
	}
}
