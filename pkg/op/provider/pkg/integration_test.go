// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package pkg_test

import (
	"bytes"
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op/bind"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	pkgprov "github.com/NobleFactor/devlore-cli/pkg/op/provider/pkg"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/pkg/gen"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

// mockPM implements op.PackageManager for testing.
type mockPM struct {
	installed map[string]string // name → version
}

func (m *mockPM) Name() string                             { return "mock" }
func (m *mockPM) ParsePURL(id string) op.PURL              { return op.PURL{Type: "mock", Name: id} }
func (m *mockPM) Installed(name string) bool               { _, ok := m.installed[name]; return ok }
func (m *mockPM) Version(name string) string               { return m.installed[name] }
func (m *mockPM) Available(_ string) bool                  { return true }
func (m *mockPM) Search(_ string, _ int) []op.SearchResult { return nil }
func (m *mockPM) Install(_ ...string) op.PlatformResult    { return op.PlatformResult{OK: true} }
func (m *mockPM) Remove(_ string) op.PlatformResult        { return op.PlatformResult{OK: true} }
func (m *mockPM) Update() op.PlatformResult                { return op.PlatformResult{OK: true, Stdout: "updated"} }
func (m *mockPM) AddRepo(_, _, _ string) op.PlatformResult { return op.PlatformResult{OK: true} }
func (m *mockPM) NeedsSudo() bool                          { return false }

func testCtx() *op.ExecutionContext {
	pm := &mockPM{installed: map[string]string{"curl": "7.88.0"}}
	return &op.ExecutionContext{
		Context:  context.Background(),
		Writer:   &bytes.Buffer{},
		Registry: op.NewReceiverRegistry(),
		Platform: &op.Platform{
			OS:              "linux",
			Arch:            "amd64",
			PackageManager:  pm,
			PackageManagers: map[string]op.PackageManager{"mock": pm},
		},
	}
}

func receiverType(t *testing.T) op.ProviderReceiverType {
	t.Helper()
	reg := op.NewReceiverRegistry()
	rt, ok := reg.TypeByReflection(reflect.TypeFor[pkgprov.Provider]())
	if !ok {
		t.Fatal("pkg provider type not registered")
	}
	return rt.(op.ProviderReceiverType)
}

func pkgRes(t *testing.T, ctx *op.ExecutionContext, name string) *pkgprov.Resource {
	t.Helper()
	r, err := pkgprov.NewResource(ctx, name)
	if err != nil {
		t.Fatalf("NewResource(%q): %v", name, err)
	}
	return r
}

// region Starlark integration

func TestStarlark(t *testing.T) {
	ctx := testCtx()
	receiver := bind.NewProvider(receiverType(t), pkgprov.NewProvider(ctx))

	globals := starlark.StringDict{"pkg": receiver}

	thread := &starlark.Thread{
		Name:  "pkg-integration",
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
	assertBool(t, result, "result_installed")
	assertBool(t, result, "result_not_installed")
}

// endregion

// region Action dispatch

func TestActions_Installed(t *testing.T) {
	ctx := testCtx()

	a, err := ctx.ActionByName("pkg.installed")
	if err != nil {
		t.Fatalf("action pkg.installed not registered: %v", err)
	}

	result, _, err := a.Do(ctx, map[string]any{"name": pkgRes(t, ctx, "curl")})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result != true {
		t.Errorf("installed(curl) = %v, want true", result)
	}
}

func TestActions_NotInstalled(t *testing.T) {
	ctx := testCtx()

	a, err := ctx.ActionByName("pkg.not_installed")
	if err != nil {
		t.Fatalf("action pkg.not_installed not registered: %v", err)
	}

	result, _, err := a.Do(ctx, map[string]any{"name": pkgRes(t, ctx, "missing-pkg")})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result != true {
		t.Errorf("not_installed(missing-pkg) = %v, want true", result)
	}
}

func TestActions_Update(t *testing.T) {
	ctx := testCtx()

	a, err := ctx.ActionByName("pkg.update")
	if err != nil {
		t.Fatalf("action pkg.update not registered: %v", err)
	}

	result, _, err := a.Do(ctx, map[string]any{"manager": ""})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result == nil {
		t.Error("update result = nil, want non-nil")
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

// endregion
