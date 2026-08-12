// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package service

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// TestReceipt_RestoreEncoded_JSONandYAML proves the recovery-stack decode path (op.reconstructReceipt ->
// Receipt.RestoreEncoded) reconstructs a service receipt's Resource and its was_running / was_enabled flags from a
// trace stored in either document format — the coverage step 44 adds (service receipts were previously behind,
// decoding only via the stack-unused UnmarshalJSON).
func TestReceipt_RestoreEncoded_JSONandYAML(t *testing.T) {
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			runtimeEnvironment := &op.RuntimeEnvironment{ResourceCatalog: op.NewResourceCatalog()}
			resource, err := DiscoverResource(runtimeEnvironment, "nginx")
			if err != nil {
				t.Fatalf("DiscoverResource: %v", err)
			}

			original := &Receipt{ReceiptBase: op.NewReceiptBase(resource), WasRunning: true, WasEnabled: true}
			base, fields := marshalThenDecodeReceipt(t, format, original)

			reloaded := &Receipt{}
			if err := reloaded.RestoreEncoded(runtimeEnvironment, base, fields); err != nil {
				t.Fatalf("RestoreEncoded(%s): %v", format, err)
			}
			if got := reloaded.Resource(); got == nil || got.URI() != resource.URI() {
				t.Errorf("restored resource URI = %v, want %q", got, resource.URI())
			}
			if !reloaded.WasRunning || !reloaded.WasEnabled {
				t.Errorf("restored flags = (running=%v, enabled=%v), want (true, true)", reloaded.WasRunning, reloaded.WasEnabled)
			}
		})
	}
}

// marshalThenDecodeReceipt marshals `receipt` in `format`, then decodes it into the (base, fields) pair the recovery
// stack hands RestoreEncoded — proving RestoreEncoded consumes values from either codec identically.
func marshalThenDecodeReceipt(t *testing.T, format string, receipt any) (base op.ReceiptData, fields map[string]any) {
	t.Helper()

	var data []byte
	var err error
	fields = map[string]any{}

	switch format {
	case "json":
		if data, err = json.Marshal(receipt); err != nil {
			t.Fatalf("json.Marshal: %v", err)
		}
		if err = json.Unmarshal(data, &base); err != nil {
			t.Fatalf("json.Unmarshal base: %v", err)
		}
		if err = json.Unmarshal(data, &fields); err != nil {
			t.Fatalf("json.Unmarshal fields: %v", err)
		}
	case "yaml":
		if data, err = yaml.Marshal(receipt); err != nil {
			t.Fatalf("yaml.Marshal: %v", err)
		}
		if err = yaml.Unmarshal(data, &base); err != nil {
			t.Fatalf("yaml.Unmarshal base: %v", err)
		}
		if err = yaml.Unmarshal(data, &fields); err != nil {
			t.Fatalf("yaml.Unmarshal fields: %v", err)
		}
	}

	return base, fields
}
