// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package service_test

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
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/service"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/service/gen"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

// mockSM implements op.ServiceManager for testing.
type mockSM struct {
	services map[string]struct{ running, enabled bool }
}

func (m *mockSM) Exists(name string) bool {
	_, ok := m.services[name]
	return ok
}

func (m *mockSM) IsRunning(name string) bool {
	if s, ok := m.services[name]; ok {
		return s.running
	}
	return false
}

func (m *mockSM) IsEnabled(name string) bool {
	if s, ok := m.services[name]; ok {
		return s.enabled
	}
	return false
}

func (m *mockSM) Status(_ string) string             { return "active" }
func (m *mockSM) Start(_ string) op.PlatformResult   { return op.PlatformResult{OK: true} }
func (m *mockSM) Stop(_ string) op.PlatformResult    { return op.PlatformResult{OK: true} }
func (m *mockSM) Enable(_ string) op.PlatformResult  { return op.PlatformResult{OK: true} }
func (m *mockSM) Disable(_ string) op.PlatformResult { return op.PlatformResult{OK: true} }
func (m *mockSM) NeedsSudo() bool                    { return false }

func sshRes(t *testing.T) *service.Resource {
	t.Helper()
	r, err := service.NewResource(nil, "sshd")
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}
	return r
}

func testCtx() *op.ExecutionContext {
	sm := &mockSM{services: map[string]struct{ running, enabled bool }{
		"sshd": {running: true, enabled: true},
	}}
	return &op.ExecutionContext{
		Context:  context.Background(),
		Writer:   &bytes.Buffer{},
		Registry: op.NewReceiverRegistry(),
		Platform: &op.Platform{
			OS:             "linux",
			Arch:           "amd64",
			ServiceManager: sm,
		},
	}
}

func receiverType(t *testing.T) op.ProviderReceiverType {
	t.Helper()
	reg := op.NewReceiverRegistry()
	rt, ok := reg.TypeByReflection(reflect.TypeFor[service.Provider]())
	if !ok {
		t.Fatal("service provider type not registered")
	}
	return rt.(op.ProviderReceiverType)
}

// region Starlark integration

func TestStarlark(t *testing.T) {
	ctx := testCtx()
	bind.SetRegistry(ctx.Registry)
	receiver := bind.NewProvider(receiverType(t), service.NewProvider(ctx))

	globals := starlark.StringDict{"service": receiver}

	thread := &starlark.Thread{
		Name:  "service-integration",
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
	assertBool(t, result, "result_exists")
	assertBool(t, result, "result_running")
	assertBool(t, result, "result_enabled")
	assertBool(t, result, "result_not_exists")
}

// endregion

// region Action dispatch

func TestActions_Exists(t *testing.T) {
	ctx := testCtx()

	a, err := ctx.ActionByName("service.exists")
	if err != nil {
		t.Fatalf("action service.exists not registered: %v", err)
	}

	result, _, err := a.Do(ctx, map[string]any{"name": sshRes(t)})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result != true {
		t.Errorf("exists(sshd) = %v, want true", result)
	}
}

func TestActions_Running(t *testing.T) {
	ctx := testCtx()

	a, err := ctx.ActionByName("service.running")
	if err != nil {
		t.Fatalf("action service.running not registered: %v", err)
	}

	result, _, err := a.Do(ctx, map[string]any{"name": sshRes(t)})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result != true {
		t.Errorf("running(sshd) = %v, want true", result)
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
