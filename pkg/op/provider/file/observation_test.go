// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// TestObservation_RidesReceipt_RoundTrips proves the execution-record stance for observations (phase-8 step 22
// slice D): an observation produced by Provider.Observe is a plain record that rides its receipt through a trace
// save/load in both document formats — it is never cataloged, and resume leaves the reloaded (untyped) value as-is
// per the re-observe-and-verify stance.
func TestObservation_RidesReceipt_RoundTrips(t *testing.T) {

	dir := t.TempDir()
	p := testProvider(t, dir)

	path := filepath.Join(dir, "observed.txt")
	if err := os.WriteFile(path, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}

	resource, err := DiscoverResource(p.RuntimeEnvironment(), path)
	if err != nil {
		t.Fatalf("DiscoverResource: %v", err)
	}

	observation, err := p.Observe(resource)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if !observation.Exists {
		t.Fatal("Observe: Exists = false for a real file")
	}

	receipt := &op.ReceiptBase{}
	if err := receipt.Commit(nil, observation, nil, nil); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	stack := op.NewRecoveryStack()
	stack.Push(receipt)

	codecs := []struct {
		name   string
		encode func(any) ([]byte, error)
		decode func([]byte, any) error
	}{
		{"json", json.Marshal, json.Unmarshal},
		{"yaml", yaml.Marshal, yaml.Unmarshal},
	}

	for _, codec := range codecs {
		t.Run(codec.name, func(t *testing.T) {

			data, err := codec.encode(stack)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}

			var reloaded op.RecoveryStack
			if err := codec.decode(data, &reloaded); err != nil {
				t.Fatalf("decode: %v", err)
			}

			receipts := reloaded.Receipts()
			if len(receipts) != 1 {
				t.Fatalf("reloaded Receipts() = %d, want 1", len(receipts))
			}
			if receipts[0].Result() == nil {
				t.Fatal("reloaded Result() = nil; the observation record must survive the round trip")
			}
		})
	}
}
