// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlarkbridge

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"go.starlark.net/starlark"
)

// region Test fixtures

// pipelineResource is a registered Resource type used to verify starlarkToGoTyped's full string→Resource
// pipeline (NoneType filter → toGo → op.Convert → registered constructor).
type pipelineResource struct {
	op.ResourceBase
	Path string
}

// Resolve implements [op.Resource]; tests don't observe state.
func (r *pipelineResource) Resolve() error { return nil }

func newPipelineResource(ctx *op.RuntimeEnvironment, identity any) (op.Resource, error) {
	s, ok := identity.(string)
	if !ok {
		return nil, fmt.Errorf("pipelineResource: expected string, got %T", identity)
	}
	base, err := op.NewResourceBase(ctx, s, reflect.TypeFor[*pipelineResource]())
	if err != nil {
		return nil, err
	}
	return &pipelineResource{ResourceBase: base, Path: s}, nil
}

// init registers pipelineResource with the package-global announce table at test-binary load time. The
// init runs only when the starlarkbridge test binary builds; production code never sees it.
func init() {
	op.AnnounceResource(
		reflect.TypeFor[pipelineResource](),
		newPipelineResource,
		nil,
	)
}

// makePipelineContext returns an RuntimeEnvironment whose Registry has pipelineResource announced.
func makePipelineContext(t *testing.T) *op.RuntimeEnvironment {
	t.Helper()
	return &op.RuntimeEnvironment{Registry: op.NewReceiverRegistry()}
}

// endregion

// region NoneType short-circuit

func TestStarlarkToGoTyped_NoneShortCircuit(t *testing.T) {

	ctx := makePipelineContext(t)

	tests := []struct {
		name   string
		target reflect.Type
	}{
		{"target string", reflect.TypeFor[string]()},
		{"target int", reflect.TypeFor[int]()},
		{"target Resource", reflect.TypeFor[*pipelineResource]()},
		{"target slice", reflect.TypeFor[[]string]()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := starlarkToGoTyped(ctx, starlark.None, tt.target)
			if err != nil {
				t.Fatalf("starlarkToGoTyped(None → %s): %v", tt.target, err)
			}
			if got != nil {
				t.Errorf("got %#v, want nil for None short-circuit", got)
			}
		})
	}
}

// endregion

// region full pipeline composition

func TestStarlarkToGoTyped_PrimitiveTargets(t *testing.T) {

	ctx := makePipelineContext(t)

	tests := []struct {
		name   string
		sv     starlark.Value
		target reflect.Type
		want   any
	}{
		{"String to string", starlark.String("hi"), reflect.TypeFor[string](), "hi"},
		{"Int to int", starlark.MakeInt64(42), reflect.TypeFor[int](), 42},
		{"Int to int64", starlark.MakeInt64(42), reflect.TypeFor[int64](), int64(42)},
		{"Bool to bool", starlark.Bool(true), reflect.TypeFor[bool](), true},
		{"Float to float64", starlark.Float(3.14), reflect.TypeFor[float64](), 3.14},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := starlarkToGoTyped(ctx, tt.sv, tt.target)
			if err != nil {
				t.Fatalf("starlarkToGoTyped: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v (%T), want %#v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestStarlarkToGoTyped_StringToResource(t *testing.T) {

	ctx := makePipelineContext(t)

	got, err := starlarkToGoTyped(ctx, starlark.String("/etc/foo"), reflect.TypeFor[*pipelineResource]())
	if err != nil {
		t.Fatalf("starlarkToGoTyped: %v", err)
	}
	r, ok := got.(*pipelineResource)
	if !ok {
		t.Fatalf("got %T, want *pipelineResource", got)
	}
	if r.Path != "/etc/foo" {
		t.Errorf("got Path=%q, want \"/etc/foo\"", r.Path)
	}
}

func TestStarlarkToGoTyped_ListToTypedSlice(t *testing.T) {

	ctx := makePipelineContext(t)

	list := starlark.NewList([]starlark.Value{
		starlark.String("a"),
		starlark.String("b"),
	})
	got, err := starlarkToGoTyped(ctx, list, reflect.TypeFor[[]string]())
	if err != nil {
		t.Fatalf("starlarkToGoTyped: %v", err)
	}
	want := []string{"a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestStarlarkToGoTyped_DictToTypedMap(t *testing.T) {

	ctx := makePipelineContext(t)

	dict := starlark.NewDict(2)
	_ = dict.SetKey(starlark.String("k"), starlark.String("v"))
	_ = dict.SetKey(starlark.String("n"), starlark.String("w"))

	got, err := starlarkToGoTyped(ctx, dict, reflect.TypeFor[map[string]string]())
	if err != nil {
		t.Fatalf("starlarkToGoTyped: %v", err)
	}
	want := map[string]string{"k": "v", "n": "w"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

// endregion

// region error propagation

func TestStarlarkToGoTyped_Errors(t *testing.T) {

	ctx := makePipelineContext(t)

	t.Run("unconvertible", func(t *testing.T) {
		_, err := starlarkToGoTyped(ctx, starlark.MakeInt64(1), reflect.TypeFor[bool]())
		if err == nil {
			t.Fatal("want error for int → bool")
		}
	})

	t.Run("Resource construction error", func(t *testing.T) {
		// The constructor accepts string only. Pass an Int.
		_, err := starlarkToGoTyped(ctx, starlark.MakeInt64(1), reflect.TypeFor[*pipelineResource]())
		if err == nil {
			t.Fatal("want error from constructor")
		}
	})

	t.Run("unregistered Resource", func(t *testing.T) {
		type otherResource struct {
			op.ResourceBase
		}
		_, err := starlarkToGoTyped(ctx, starlark.String("x"), reflect.TypeFor[*otherResource]())
		if err == nil {
			t.Fatal("want error for unregistered Resource")
		}
		if !strings.Contains(err.Error(), "not registered") {
			t.Errorf("want 'not registered', got %q", err)
		}
	})
}

// endregion
