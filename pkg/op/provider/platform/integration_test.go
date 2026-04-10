// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	"reflect"

	"github.com/NobleFactor/devlore-cli/pkg/op/bind"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/platform"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/platform/gen"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

var testPlatform = &op.Platform{
	OS:       "linux",
	Arch:     "arm64",
	Distro:   "Ubuntu",
	Hostname: "build-host",
	Version:  "24.04",
}

func testCtx() *op.ExecutionContext {
	return &op.ExecutionContext{
		Context:  context.Background(),
		Writer:   &bytes.Buffer{},
		Platform: testPlatform,
		Registry: op.NewReceiverRegistry(),
	}
}

func receiverType(t *testing.T) op.ProviderReceiverType {
	t.Helper()
	reg := op.NewReceiverRegistry()
	rt, ok := reg.TypeByReflection(reflect.TypeFor[platform.Provider]())
	if !ok {
		t.Fatal("platform provider type not registered")
	}
	return rt.(op.ProviderReceiverType)
}

// region Starlark integration

func TestStarlark(t *testing.T) {
	ctx := testCtx()
	receiver := bind.NewProvider(receiverType(t), platform.NewProvider(ctx))

	globals := starlark.StringDict{"platform": receiver}

	thread := &starlark.Thread{
		Name:  "platform-integration",
		Print: func(_ *starlark.Thread, msg string) { t.Logf("[star] %s", msg) },
	}

	data, err := os.ReadFile("testdata/integration.star")
	if err != nil {
		t.Fatalf("reading script: %v", err)
	}

	opts := &syntax.FileOptions{Set: true, GlobalReassign: true, TopLevelControl: true}
	result, err := starlark.ExecFileOptions(opts, thread, "testdata/integration.star", data, globals)
	if err != nil {
		t.Fatalf("exec script: %v", err)
	}

	assertBool(t, result, "result_done")
	assertStringEQ(t, result, "result_arch", "arm64")
	assertStringEQ(t, result, "result_os", "linux")
	assertStringEQ(t, result, "result_distro", "Ubuntu")
	assertStringEQ(t, result, "result_hostname", "build-host")
	assertStringEQ(t, result, "result_version", "24.04")
}

// endregion

// region Action dispatch

func TestActions(t *testing.T) {
	ctx := testCtx()

	tests := []struct {
		action string
		want   string
	}{
		{"platform.arch", "arm64"},
		{"platform.os", "linux"},
		{"platform.distro", "Ubuntu"},
		{"platform.hostname", "build-host"},
		{"platform.version", "24.04"},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			a, err := ctx.ActionByName(tt.action)
			if err != nil {
				t.Fatalf("action %q not registered: %v", tt.action, err)
			}
			result, _, err := a.Do(ctx, map[string]any{})
			if err != nil {
				t.Fatalf("Do() error = %v", err)
			}
			got, ok := result.(string)
			if !ok {
				t.Fatalf("result type = %T, want string", result)
			}
			if got != tt.want {
				t.Errorf("result = %q, want %q", got, tt.want)
			}
		})
	}
}

// endregion

// region Assertions

func assertBool(t *testing.T, globals starlark.StringDict, key string) {
	t.Helper()
	v, ok := globals[key]
	if !ok {
		t.Errorf("missing global %q", key)
		return
	}
	if v != starlark.True {
		t.Errorf("%s = %v, want true", key, v)
	}
}

func assertStringEQ(t *testing.T, globals starlark.StringDict, key, want string) {
	t.Helper()
	v, ok := globals[key]
	if !ok {
		t.Errorf("missing global %q", key)
		return
	}
	s, ok := v.(starlark.String)
	if !ok {
		t.Errorf("%s: expected String, got %s", key, v.Type())
		return
	}
	if string(s) != want {
		t.Errorf("%s = %q, want %q", key, string(s), want)
	}
}

// endregion
