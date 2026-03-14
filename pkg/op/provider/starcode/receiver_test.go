// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starcode_test

import (
	"os"
	"sort"
	"testing"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/starcode"
	starcodegen "github.com/NobleFactor/devlore-cli/pkg/op/provider/starcode/gen"
)

func TestMain(m *testing.M) {
	op.InitAll(op.NewActionRegistry(), op.Context{})
	os.Exit(m.Run())
}

// TestReceiverAttrNames verifies StarcodeReceiver.AttrNames returns the expected attribute list.
func TestReceiverAttrNames(t *testing.T) {
	r := op.WrapProviderInExecutingReceiver(starcodegen.Receiver, &starcode.Provider{Root: "."})
	names := r.AttrNames()

	if len(names) != 1 {
		t.Fatalf("AttrNames length = %d, want 1", len(names))
	}
	if names[0] != "capture" {
		t.Errorf("AttrNames[0] = %q, want %q", names[0], "capture")
	}
}

// TestReceiverAttrUnknown verifies StarcodeReceiver.Attr returns an error for unknown attributes.
func TestReceiverAttrUnknown(t *testing.T) {
	r := op.WrapProviderInExecutingReceiver(starcodegen.Receiver, &starcode.Provider{Root: "."})

	val, err := r.Attr("bogus")
	if err == nil {
		t.Fatal("expected error for unknown attribute, got nil")
	}
	if val != nil {
		t.Errorf("expected nil value for unknown attribute, got %v", val)
	}
}

// TestReceiverAttrCapture verifies StarcodeReceiver.Attr returns a value for the "capture" attribute.
func TestReceiverAttrCapture(t *testing.T) {
	r := op.WrapProviderInExecutingReceiver(starcodegen.Receiver, &starcode.Provider{Root: "."})

	val, err := r.Attr("capture")
	if err != nil {
		t.Fatalf("Attr(capture): %v", err)
	}
	if val == nil {
		t.Fatal("expected non-nil value for capture attribute")
	}
}

// TestSourcesValueAttrNames verifies SourcesValue.AttrNames returns the expected sorted list.
// TODO: Re-enable when dependent type wrappers are implemented (see generate.star TODO).
func TestSourcesValueAttrNames(t *testing.T) {
	t.Skip("dependent type wrapper not yet implemented — Sources methods not exposed via op.Marshal")
	val, _ := op.Marshal(&starcode.Sources{Root: ".", Files: nil})
	sv := val.(starlark.HasAttrs)
	names := sv.AttrNames()

	want := []string{"analyze", "count", "index", "paths", "stats"}
	if len(names) != len(want) {
		t.Fatalf("AttrNames length = %d, want %d", len(names), len(want))
	}

	sorted := make([]string, len(names))
	copy(sorted, names)
	sort.Strings(sorted)

	for i, name := range sorted {
		if name != want[i] {
			t.Errorf("AttrNames[%d] = %q, want %q", i, name, want[i])
		}
	}
}

// TestSourcesValueAttrUnknown verifies SourcesValue.Attr returns an error for unknown attributes.
func TestSourcesValueAttrUnknown(t *testing.T) {
	v, _ := op.Marshal(&starcode.Sources{Root: ".", Files: nil})
	sv := v.(starlark.HasAttrs)

	val, err := sv.Attr("bogus")
	if err == nil {
		t.Fatal("expected error for unknown attribute, got nil")
	}
	if val != nil {
		t.Errorf("expected nil value for unknown attribute, got %v", val)
	}
}

// TestSourcesValueAttrPaths verifies SourcesValue.Attr returns a value for "paths".
func TestSourcesValueAttrPaths(t *testing.T) {
	t.Skip("dependent type wrapper not yet implemented")
	v, _ := op.Marshal(&starcode.Sources{Root: ".", Files: nil})
	sv := v.(starlark.HasAttrs)

	val, err := sv.Attr("paths")
	if err != nil {
		t.Fatalf("Attr(paths): %v", err)
	}
	if val == nil {
		t.Fatal("expected non-nil value for paths attribute")
	}
}

// TestSourcesValueAttrCount verifies SourcesValue.Attr returns a value for "count".
func TestSourcesValueAttrCount(t *testing.T) {
	t.Skip("dependent type wrapper not yet implemented")
	v, _ := op.Marshal(&starcode.Sources{Root: ".", Files: nil})
	sv := v.(starlark.HasAttrs)

	val, err := sv.Attr("count")
	if err != nil {
		t.Fatalf("Attr(count): %v", err)
	}
	if val == nil {
		t.Fatal("expected non-nil value for count attribute")
	}
}
