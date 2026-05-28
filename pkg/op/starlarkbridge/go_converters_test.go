// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlarkbridge

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// region Test fixtures

// pipelineResource is a registered Resource type used to verify StarlarkToGoTyped's full string→Resource
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

// makePipelineContext returns a RuntimeEnvironment whose Registry has pipelineResource announced.
func makePipelineContext(t *testing.T) *op.RuntimeEnvironment {
	t.Helper()
	return &op.RuntimeEnvironment{ReceiverRegistry: op.NewReceiverRegistry()}
}

// endregion

// region toGo with target=any (interface-target branch in toGoInto)

func TestToGo_AnyTarget_Primitives(t *testing.T) {

	anyType := reflect.TypeFor[any]()

	tests := []struct {
		name string
		sv   starlark.Value
		want any
	}{
		{"None", starlark.None, nil},
		{"Bool true", starlark.Bool(true), true},
		{"Bool false", starlark.Bool(false), false},
		{"Int small", starlark.MakeInt64(42), int64(42)},
		{"Int zero", starlark.MakeInt64(0), int64(0)},
		{"Int negative", starlark.MakeInt64(-7), int64(-7)},
		{"Float", starlark.Float(3.14), 3.14},
		{"String", starlark.String("hi"), "hi"},
		{"String empty", starlark.String(""), ""},
		{"String unicode", starlark.String("héllo"), "héllo"},
		{"Bytes", starlark.Bytes("data"), []byte("data")},
		{"Bytes empty", starlark.Bytes(""), []byte("")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toGo(tt.sv, anyType)
			if err != nil {
				t.Fatalf("toGo: %v", err)
			}
			if tt.sv == starlark.None {
				// Interface zero value is nil; reflect.DeepEqual considers nil == nil.
				if got != nil {
					t.Errorf("got %#v, want nil for None", got)
				}
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v (%T), want %#v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestToGo_AnyTarget_Containers(t *testing.T) {

	anyType := reflect.TypeFor[any]()

	t.Run("List of mixed primitives", func(t *testing.T) {
		list := starlark.NewList([]starlark.Value{
			starlark.String("a"),
			starlark.MakeInt64(1),
			starlark.Bool(true),
		})
		got, err := toGo(list, anyType)
		if err != nil {
			t.Fatalf("toGo: %v", err)
		}
		want := []any{"a", int64(1), true}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("List empty", func(t *testing.T) {
		list := starlark.NewList(nil)
		got, err := toGo(list, anyType)
		if err != nil {
			t.Fatalf("toGo: %v", err)
		}
		want := []any{}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("Tuple", func(t *testing.T) {
		tup := starlark.Tuple{starlark.String("x"), starlark.MakeInt64(2)}
		got, err := toGo(tup, anyType)
		if err != nil {
			t.Fatalf("toGo: %v", err)
		}
		want := []any{"x", int64(2)}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("Set", func(t *testing.T) {
		set := starlark.NewSet(2)
		_ = set.Insert(starlark.String("a"))
		_ = set.Insert(starlark.String("b"))
		got, err := toGo(set, anyType)
		if err != nil {
			t.Fatalf("toGo: %v", err)
		}
		gotSlice, ok := got.([]any)
		if !ok {
			t.Fatalf("got %T, want []any", got)
		}
		if len(gotSlice) != 2 {
			t.Errorf("got len %d, want 2", len(gotSlice))
		}
	})

	t.Run("Dict with string keys", func(t *testing.T) {
		dict := starlark.NewDict(2)
		_ = dict.SetKey(starlark.String("k"), starlark.String("v"))
		_ = dict.SetKey(starlark.String("n"), starlark.MakeInt64(1))
		got, err := toGo(dict, anyType)
		if err != nil {
			t.Fatalf("toGo: %v", err)
		}
		gotMap, ok := got.(map[any]any)
		if !ok {
			t.Fatalf("got %T, want map[any]any", got)
		}
		if gotMap["k"] != "v" || gotMap["n"] != int64(1) {
			t.Errorf("got %#v, want {k: v, n: 1}", gotMap)
		}
	})
}

func TestToGo_AnyTarget_IntOverflow(t *testing.T) {

	// starlark.Int holds arbitrary precision; values out of int64 range error in toGoInto.
	bigInt := starlark.MakeUint64(^uint64(0)) // max uint64; Int64() will fail
	_, err := toGo(bigInt, reflect.TypeFor[any]())
	if err == nil {
		t.Fatal("want error for int out of range")
	}
}

// endregion

// region toGo with concrete (non-interface) targets

func TestToGo_ConcreteTarget_Primitives(t *testing.T) {

	tests := []struct {
		name   string
		sv     starlark.Value
		target reflect.Type
		want   any
	}{
		{"String to string", starlark.String("hi"), reflect.TypeFor[string](), "hi"},
		{"Bool to bool", starlark.Bool(true), reflect.TypeFor[bool](), true},
		{"Int to int", starlark.MakeInt64(42), reflect.TypeFor[int](), int(42)},
		{"Int to int64", starlark.MakeInt64(42), reflect.TypeFor[int64](), int64(42)},
		{"Int to int32", starlark.MakeInt64(42), reflect.TypeFor[int32](), int32(42)},
		{"Int to uint", starlark.MakeInt64(42), reflect.TypeFor[uint](), uint(42)},
		{"Int to uint64", starlark.MakeInt64(42), reflect.TypeFor[uint64](), uint64(42)},
		{"Float to float64", starlark.Float(3.14), reflect.TypeFor[float64](), 3.14},
		{"Float to float32", starlark.Float(3.14), reflect.TypeFor[float32](), float32(3.14)},
		{"Int to float64", starlark.MakeInt64(7), reflect.TypeFor[float64](), 7.0},
		{"Bytes to []byte", starlark.Bytes("data"), reflect.TypeFor[[]byte](), []byte("data")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toGo(tt.sv, tt.target)
			if err != nil {
				t.Fatalf("toGo: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v (%T), want %#v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestToGo_ConcreteTarget_Errors(t *testing.T) {

	tests := []struct {
		name        string
		sv          starlark.Value
		target      reflect.Type
		errContains string
	}{
		{"String target gets Int", starlark.MakeInt64(1), reflect.TypeFor[string](), "expected string"},
		{"Bool target gets String", starlark.String("x"), reflect.TypeFor[bool](), "expected bool"},
		{"Int target gets String", starlark.String("x"), reflect.TypeFor[int](), "expected int"},
		{"Float target gets String", starlark.String("x"), reflect.TypeFor[float64](), "expected float or int"},
		{"Bytes target gets Int", starlark.MakeInt64(1), reflect.TypeFor[[]byte](), "expected bytes"},
		{"List target gets Int", starlark.MakeInt64(1), reflect.TypeFor[[]string](), "expected list"},
		{"Map target gets Int", starlark.MakeInt64(1), reflect.TypeFor[map[string]string](), "expected dict"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := toGo(tt.sv, tt.target)
			if err == nil {
				t.Fatal("want error")
			}
			if !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("want error mentioning %q, got %q", tt.errContains, err)
			}
		})
	}
}

func TestToGo_ConcreteTarget_Slice(t *testing.T) {

	t.Run("List to []string", func(t *testing.T) {
		list := starlark.NewList([]starlark.Value{
			starlark.String("a"),
			starlark.String("b"),
		})
		got, err := toGo(list, reflect.TypeFor[[]string]())
		if err != nil {
			t.Fatalf("toGo: %v", err)
		}
		want := []string{"a", "b"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("List to []int", func(t *testing.T) {
		list := starlark.NewList([]starlark.Value{
			starlark.MakeInt64(1),
			starlark.MakeInt64(2),
		})
		got, err := toGo(list, reflect.TypeFor[[]int]())
		if err != nil {
			t.Fatalf("toGo: %v", err)
		}
		want := []int{1, 2}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("List index error reports position", func(t *testing.T) {
		list := starlark.NewList([]starlark.Value{
			starlark.String("a"),
			starlark.MakeInt64(1), // wrong type
		})
		_, err := toGo(list, reflect.TypeFor[[]string]())
		if err == nil {
			t.Fatal("want error")
		}
		if !strings.Contains(err.Error(), "list index 1") {
			t.Errorf("want error mentioning 'list index 1', got %q", err)
		}
	})
}

func TestToGo_ConcreteTarget_Map(t *testing.T) {

	dict := starlark.NewDict(2)
	_ = dict.SetKey(starlark.String("k"), starlark.String("v"))
	_ = dict.SetKey(starlark.String("n"), starlark.String("w"))

	got, err := toGo(dict, reflect.TypeFor[map[string]string]())
	if err != nil {
		t.Fatalf("toGo: %v", err)
	}
	want := map[string]string{"k": "v", "n": "w"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestToGo_ConcreteTarget_Struct(t *testing.T) {

	type point struct {
		X int
		Y int
	}

	// camelToSnake lowercases field names; dict keys must match the snake form.
	dict := starlark.NewDict(2)
	_ = dict.SetKey(starlark.String("x"), starlark.MakeInt64(3))
	_ = dict.SetKey(starlark.String("y"), starlark.MakeInt64(4))

	got, err := toGo(dict, reflect.TypeFor[point]())
	if err != nil {
		t.Fatalf("toGo: %v", err)
	}
	want := point{X: 3, Y: 4}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

// endregion

// region toGoInto direct (in-place mutation)

func TestToGoInto_NilStarlark(t *testing.T) {

	// nil starlark.Value sets target to zero.
	var s string = "preset"
	rv := reflect.ValueOf(&s).Elem()
	if err := toGoInto(nil, rv); err != nil {
		t.Fatalf("toGoInto: %v", err)
	}
	if s != "" {
		t.Errorf("got %q, want empty (zero of string)", s)
	}
}

func TestToGoInto_NoneSetsZero(t *testing.T) {

	var i int = 99
	rv := reflect.ValueOf(&i).Elem()
	if err := toGoInto(starlark.None, rv); err != nil {
		t.Fatalf("toGoInto: %v", err)
	}
	if i != 0 {
		t.Errorf("got %d, want 0 (zero of int)", i)
	}
}

func TestToGoInto_PointerAllocate(t *testing.T) {

	// Target is *string; toGoInto must allocate a fresh pointer when nil.
	var sp *string
	rv := reflect.ValueOf(&sp).Elem()
	if err := toGoInto(starlark.String("hi"), rv); err != nil {
		t.Fatalf("toGoInto: %v", err)
	}
	if sp == nil {
		t.Fatal("got nil pointer, want allocated")
	}
	if *sp != "hi" {
		t.Errorf("got %q, want \"hi\"", *sp)
	}
}

func TestToGoInto_StarlarkValuePassThrough(t *testing.T) {

	// When the starlark.Value is directly assignable to the target type, it passes through.
	var fn *starlark.Function
	rv := reflect.ValueOf(&fn).Elem()
	// Need a real function-typed starlark.Value. starlark.Builtin satisfies starlark.Value but isn't *Function.
	// Use Bool which is assignable to starlark.Value but not to *Function — should fail.
	err := toGoInto(starlark.Bool(true), rv)
	if err == nil {
		t.Fatal("want error: starlark.Bool not assignable to *starlark.Function")
	}
}

// endregion

// region StarlarkToGoTyped NoneType short-circuit

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
			got, err := StarlarkToGoTyped(ctx, starlark.None, tt.target)
			if err != nil {
				t.Fatalf("StarlarkToGoTyped(None → %s): %v", tt.target, err)
			}
			if got != nil {
				t.Errorf("got %#v, want nil for None short-circuit", got)
			}
		})
	}
}

// endregion

// region StarlarkToGoTyped full pipeline composition

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
			got, err := StarlarkToGoTyped(ctx, tt.sv, tt.target)
			if err != nil {
				t.Fatalf("StarlarkToGoTyped: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v (%T), want %#v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestStarlarkToGoTyped_StringToResource(t *testing.T) {

	ctx := makePipelineContext(t)

	got, err := StarlarkToGoTyped(ctx, starlark.String("/etc/foo"), reflect.TypeFor[*pipelineResource]())
	if err != nil {
		t.Fatalf("StarlarkToGoTyped: %v", err)
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
	got, err := StarlarkToGoTyped(ctx, list, reflect.TypeFor[[]string]())
	if err != nil {
		t.Fatalf("StarlarkToGoTyped: %v", err)
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

	got, err := StarlarkToGoTyped(ctx, dict, reflect.TypeFor[map[string]string]())
	if err != nil {
		t.Fatalf("StarlarkToGoTyped: %v", err)
	}
	want := map[string]string{"k": "v", "n": "w"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

// endregion

// region StarlarkToGoTyped error propagation

func TestStarlarkToGoTyped_Errors(t *testing.T) {

	ctx := makePipelineContext(t)

	t.Run("unconvertible", func(t *testing.T) {
		_, err := StarlarkToGoTyped(ctx, starlark.MakeInt64(1), reflect.TypeFor[bool]())
		if err == nil {
			t.Fatal("want error for int → bool")
		}
	})

	t.Run("Resource construction error", func(t *testing.T) {
		// The constructor accepts string only. Pass an Int.
		_, err := StarlarkToGoTyped(ctx, starlark.MakeInt64(1), reflect.TypeFor[*pipelineResource]())
		if err == nil {
			t.Fatal("want error from constructor")
		}
	})

	t.Run("unregistered Resource", func(t *testing.T) {
		type otherResource struct {
			op.ResourceBase
		}
		_, err := StarlarkToGoTyped(ctx, starlark.String("x"), reflect.TypeFor[*otherResource]())
		if err == nil {
			t.Fatal("want error for unregistered Resource")
		}
		if !strings.Contains(err.Error(), "not registered") {
			t.Errorf("want 'not registered', got %q", err)
		}
	})
}

// endregion
