// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package git

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// TestReceipt_RestoreEncoded_JSONandYAML proves the recovery-stack decode path (op.reconstructReceipt ->
// Receipt.RestoreEncoded) reconstructs a git receipt's Resource from a trace stored in either document format — the
// coverage step 44 adds (git receipts were previously behind, decoding only via the stack-unused UnmarshalJSON).
func TestReceipt_RestoreEncoded_JSONandYAML(t *testing.T) {
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			runtimeEnvironment := &op.RuntimeEnvironment{
				Root:            fsroot.OpenWritableUnconfined("/"),
				ResourceCatalog: op.NewResourceCatalog(),
			}
			resource, err := DiscoverResource(runtimeEnvironment, t.TempDir()+"/repo")
			if err != nil {
				t.Fatalf("DiscoverResource: %v", err)
			}

			base, fields := marshalThenDecodeReceipt(t, format, NewReceipt(resource))

			reloaded := &Receipt{}
			if err := reloaded.RestoreEncoded(runtimeEnvironment, base, fields); err != nil {
				t.Fatalf("RestoreEncoded(%s): %v", format, err)
			}
			if got := reloaded.Resource(); got == nil || got.URI() != resource.URI() {
				t.Errorf("restored resource URI = %v, want %q", got, resource.URI())
			}
		})
	}
}

// marshalThenDecodeReceipt marshals `receipt` in `format`, then decodes it into the (base, fields) pair the recovery
// stack hands RestoreEncoded — proving RestoreEncoded consumes values from either codec identically.
func marshalThenDecodeReceipt(t *testing.T, format string, receipt any) (op.ReceiptData, map[string]any) {
	t.Helper()

	var data []byte
	var err error
	var base op.ReceiptData
	fields := map[string]any{}

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
