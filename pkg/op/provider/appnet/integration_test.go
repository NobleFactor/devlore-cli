// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package appnet_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op/bind"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	appnetprov "github.com/NobleFactor/devlore-cli/pkg/op/provider/appnet"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/appnet/gen"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func testCtx() *op.ExecutionContext {
	return &op.ExecutionContext{
		Context:  context.Background(),
		Writer:   &bytes.Buffer{},
		Registry: op.NewReceiverRegistry(),
	}
}

func receiverType(t *testing.T) op.ProviderReceiverType {
	t.Helper()
	reg := op.NewReceiverRegistry()
	rt, ok := reg.TypeByReflection(reflect.TypeFor[appnetprov.Provider]())
	if !ok {
		t.Fatal("appnet provider type not registered")
	}
	return rt.(op.ProviderReceiverType)
}

// region Starlark integration

func TestStarlark(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("test content"))
	}))
	defer srv.Close()

	ctx := testCtx()
	receiver := bind.NewProvider(receiverType(t), appnetprov.NewProvider(ctx))

	globals := starlark.StringDict{
		"appnet":   receiver,
		"test_url": starlark.String(srv.URL),
	}

	thread := &starlark.Thread{
		Name:  "appnet-integration",
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
	assertStringEQ(t, result, "result_download_type", "bytes")
}

// endregion

// region Action dispatch

func TestActions_Download(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("action content"))
	}))
	defer srv.Close()

	ctx := testCtx()

	a, err := ctx.ActionByName("appnet.download")
	if err != nil {
		t.Fatalf("action appnet.download not registered: %v", err)
	}

	res, resErr := appnetprov.NewResource(nil, srv.URL)
	if resErr != nil {
		t.Fatalf("NewResource: %v", resErr)
	}
	result, _, err := a.Do(ctx, map[string]any{"url": res})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}

	b, ok := result.([]byte)
	if !ok {
		t.Fatalf("result type = %T, want []byte", result)
	}
	if string(b) != "action content" {
		t.Errorf("result = %q, want 'action content'", string(b))
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
