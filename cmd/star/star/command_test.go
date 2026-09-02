// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package star

import (
	"fmt"
	"reflect"
	"testing"

	"go.starlark.net/starlark"
)

func TestCommand_Run(t *testing.T) {
	tests := []struct {
		name    string
		runFunc func(cmd, ctx starlark.Value) (starlark.Value, error)
		args    map[string]string
		wantErr bool
	}{
		{
			name: "successful run with no args",
			runFunc: func(_, _ starlark.Value) (starlark.Value, error) {
				return starlark.None, nil
			},
			args:    map[string]string{},
			wantErr: false,
		},
		{
			name: "successful run with args",
			runFunc: func(_, ctx starlark.Value) (starlark.Value, error) {
				argsAttr, err := ctx.(starlark.HasAttrs).Attr("args")
				if err != nil {
					return nil, err
				}
				argsDict := argsAttr.(*starlark.Dict)
				val, _, _ := argsDict.Get(starlark.String("name"))
				if val.(starlark.String) != "test" {
					t.Errorf("expected arg 'name' to be 'test', got %v", val)
				}
				return starlark.None, nil
			},
			args:    map[string]string{"name": "test"},
			wantErr: false,
		},
		{
			name: "run with dry_run context",
			runFunc: func(_, ctx starlark.Value) (starlark.Value, error) {
				dryRunAttr, err := ctx.(starlark.HasAttrs).Attr("dry_run")
				if err != nil {
					return nil, err
				}
				if dryRunAttr != starlark.False {
					t.Errorf("expected dry_run to be False, got %v", dryRunAttr)
				}
				return starlark.None, nil
			},
			args:    map[string]string{},
			wantErr: false,
		},
		{
			name: "run function returns error",
			runFunc: func(_, _ starlark.Value) (starlark.Value, error) {
				return nil, fmt.Errorf("intentional error")
			},
			args:    map[string]string{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runnable := &testCallable{fn: tt.runFunc}

			cmd := &Command{
				Name:      "test-cmd",
				Help:      "Test command",
				Extension: &Extension{},
				RunFunc:   runnable,
			}

			_, err := cmd.Run(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("Command.Run() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCommand_RunWithDryRun(t *testing.T) {
	originalDryRun := DryRun
	defer func() { DryRun = originalDryRun }()

	DryRun = true

	runnable := &testCallable{fn: func(_, ctx starlark.Value) (starlark.Value, error) {
		dryRunAttr, err := ctx.(starlark.HasAttrs).Attr("dry_run")
		if err != nil {
			return nil, err
		}
		if dryRunAttr != starlark.True {
			t.Errorf("expected dry_run to be True when DryRun=true, got %v", dryRunAttr)
		}
		return starlark.None, nil
	}}

	cmd := &Command{
		Name:      "test-cmd",
		Help:      "Test command",
		Extension: &Extension{},
		RunFunc:   runnable,
	}

	if _, err := cmd.Run(map[string]string{}); err != nil {
		t.Errorf("Command.Run() unexpected error: %v", err)
	}
}

func TestCommand_RunWithExtension(t *testing.T) {
	ext := &Extension{
		Name:   "com.example.test",
		Dir:    "/tmp/test-ext",
		Source: SourceProjectLocal,
	}

	runnable := &testCallable{fn: func(cmdVal, _ starlark.Value) (starlark.Value, error) {
		// Access extension through the command argument.
		extAttr, err := cmdVal.(starlark.HasAttrs).Attr("extension")
		if err != nil {
			return nil, err
		}
		extObj := extAttr.(starlark.HasAttrs)

		dir, err := extObj.Attr("dir")
		if err != nil {
			return nil, err
		}
		if dir.(starlark.String) != "/tmp/test-ext" {
			t.Errorf("expected dir '/tmp/test-ext', got %v", dir)
		}

		name, err := extObj.Attr("name")
		if err != nil {
			return nil, err
		}
		if name.(starlark.String) != "com.example.test" {
			t.Errorf("expected name 'com.example.test', got %v", name)
		}

		return starlark.None, nil
	}}

	cmd := &Command{
		Name:      "test-cmd",
		Help:      "Test command",
		Extension: ext,
		RunFunc:   runnable,
	}

	if _, err := cmd.Run(map[string]string{}); err != nil {
		t.Errorf("Command.Run() unexpected error: %v", err)
	}
}

func TestCommand_StarlarkValue(t *testing.T) {
	ext := &Extension{Name: "com.example.test"}
	cmd := &Command{
		Name:      "test.cmd",
		Extension: ext,
	}

	if cmd.Type() != "command" {
		t.Errorf("Type() = %q, want %q", cmd.Type(), "command")
	}

	nameVal, err := cmd.Attr("name")
	if err != nil {
		t.Fatalf("Attr(name) error = %v", err)
	}
	if nameVal.String() != `"test.cmd"` {
		t.Errorf("Attr(name) = %v, want %q", nameVal, "test.cmd")
	}

	extVal, err := cmd.Attr("extension")
	if err != nil {
		t.Fatalf("Attr(extension) error = %v", err)
	}
	if extVal != ext {
		t.Error("Attr(extension) should return the parent extension")
	}
}

// testCallable is a minimal starlark.Callable for testing.
// fn receives (command, ctx) — the two args passed by Command.Run.
type testCallable struct {
	fn func(cmd, ctx starlark.Value) (starlark.Value, error)
}

func (c *testCallable) Name() string          { return "test_func" }
func (c *testCallable) String() string        { return "test_func" }
func (c *testCallable) Type() string          { return "function" }
func (c *testCallable) Freeze()               {}
func (c *testCallable) Truth() starlark.Bool  { return starlark.True }
func (c *testCallable) Hash() (uint32, error) { return 0, nil }

func (c *testCallable) CallInternal(_ *starlark.Thread, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if c.fn == nil {
		return starlark.None, nil
	}
	var cmd, ctx starlark.Value
	if len(args) > 0 {
		cmd = args[0]
	}
	if len(args) > 1 {
		ctx = args[1]
	}
	return c.fn(cmd, ctx)
}

// TestCommand_RunReturnsTheScriptsResult pins the result channel: what run() returns is the command's
// result, converted to the JSON shape the output pipeline renders, and None is no result.
func TestCommand_RunReturnsTheScriptsResult(t *testing.T) {

	tests := []struct {
		name  string
		value starlark.Value
		want  any
	}{
		{name: "None is no result", value: starlark.None, want: nil},
		{name: "a string", value: starlark.String("ok"), want: "ok"},
		{name: "an int", value: starlark.MakeInt(7), want: int64(7)},
		{name: "a list", value: starlark.NewList([]starlark.Value{starlark.MakeInt(1), starlark.String("b")}), want: []any{int64(1), "b"}},
		{name: "a dict", value: dictOf(t, "name", starlark.String("x"), "ok", starlark.True), want: map[string]any{"name": "x", "ok": true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &Command{
				Name:      "test-cmd",
				Extension: &Extension{},
				RunFunc:   &testCallable{fn: func(_, _ starlark.Value) (starlark.Value, error) { return tt.value, nil }},
			}
			got, err := cmd.Run(map[string]string{})
			if err != nil {
				t.Fatalf("Command.Run() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Command.Run() = %#v, want %#v", got, tt.want)
			}
		})
	}

	t.Run("a function has no JSON shape", func(t *testing.T) {
		cmd := &Command{
			Name:      "test-cmd",
			Extension: &Extension{},
			RunFunc:   &testCallable{fn: func(_, _ starlark.Value) (starlark.Value, error) { return &testCallable{}, nil }},
		}
		if _, err := cmd.Run(map[string]string{}); err == nil {
			t.Error("Command.Run() accepted a callable as a result")
		}
	})
}

// dictOf builds a dict from alternating string keys and values.
func dictOf(t *testing.T, pairs ...any) *starlark.Dict {
	t.Helper()
	d := starlark.NewDict(len(pairs) / 2)
	for i := 0; i < len(pairs); i += 2 {
		if err := d.SetKey(starlark.String(pairs[i].(string)), pairs[i+1].(starlark.Value)); err != nil {
			t.Fatal(err)
		}
	}
	return d
}
