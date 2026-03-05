// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// --- Mock provider for action layer tests ---

// actionTestResource is a custom type with a registered constructor.
type actionTestResource struct {
	Path string
}

func init() {
	RegisterConstructor(func(v any) (actionTestResource, error) {
		s, ok := v.(string)
		if !ok {
			return actionTestResource{}, fmt.Errorf("expected string, got %T", v)
		}
		return actionTestResource{Path: s}, nil
	})
}

// actionConfig is a struct param for testing map → struct coercion.
type actionConfig struct {
	Enabled   bool
	Threshold int
	Label     string
}

// actionResource embeds ResourceBase and satisfies Resource via pointer receivers.
// Used to test that promoteAndShadow correctly promotes value results to pointers.
type actionResource struct {
	ResourceBase
	SourcePath string
}

func (r *actionResource) URI() string    { return r.NewURI(r) }
func (r *actionResource) Scheme() string { return SchemeFile }
func (r *actionResource) Host() string   { return "" }
func (r *actionResource) Path() string   { return r.SourcePath }

// actionProvider exercises all action patterns.
type actionProvider struct{}

// Compensable: (string, map[string]any, error).
func (p *actionProvider) Copy(source, dest string) (string, map[string]any, error) {
	return dest, map[string]any{"source": source, "dest": dest}, nil
}

func (p *actionProvider) CompensateCopy(state map[string]any) error {
	if state["fail"] == true {
		return errors.New("compensate failed")
	}
	return nil
}

// Non-compensable: (string, error).
func (p *actionProvider) Read(path string) (string, error) {
	return "content:" + path, nil
}

// Error-only: (error).
func (p *actionProvider) Validate(path string) error {
	if path == "" {
		return errors.New("empty path")
	}
	return nil
}

// Type coercion: int→os.FileMode.
func (p *actionProvider) Mkdir(path string, mode os.FileMode) (string, error) {
	return fmt.Sprintf("%s:%04o", path, mode), nil
}

// Constructor coercion: string→actionTestResource.
func (p *actionProvider) Deploy(res actionTestResource) (string, error) {
	return "deployed:" + res.Path, nil
}

// Struct param coercion: map[string]any → actionConfig.
func (p *actionProvider) Configure(name string, cfg actionConfig) (string, error) {
	return fmt.Sprintf("%s:enabled=%v,threshold=%d,label=%s", name, cfg.Enabled, cfg.Threshold, cfg.Label), nil
}

// Returns a Resource by value — tests shadowResult.
func (p *actionProvider) Create(path string) (actionResource, string, error) {
	return actionResource{SourcePath: path}, "undo:" + path, nil
}

func (p *actionProvider) CompensateCreate(state string) error {
	return nil
}

// Takes a Resource parameter — tests plan-time catalog resolution.
func (p *actionProvider) Touch(res actionResource) (actionResource, error) {
	return res, nil
}

// Compensable with NoResult: (NoResult, map[string]any, error).
func (p *actionProvider) Delete(path string) (NoResult, map[string]any, error) {
	return NoResult{}, map[string]any{"path": path}, nil
}

func (p *actionProvider) CompensateDelete(state map[string]any) error {
	return nil
}

// Immediate-only (no error return, should be skipped).
func (p *actionProvider) Exists(path string) bool {
	return path != ""
}

// Void with error: (error).
func (p *actionProvider) Noop() error {
	return nil
}

var actionParams = MethodParams{
	"Configure": {"name", "cfg"},
	"Copy":      {"source", "dest"},
	"Create":    {"path"},
	"Delete":    {"path"},
	"Read":      {"path"},
	"Touch":     {"res"},
	"Validate":  {"path"},
	"Mkdir":     {"path", "mode"},
	"Deploy":    {"res"},
	"Exists":    {"path"},
	"Noop":      {},
}

// --- coerceSlotValue tests ---

func TestCoerceSlotValue_Nil(t *testing.T) {
	val, err := coerceSlotValue(nil, reflect.TypeOf(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val.Interface() != "" {
		t.Errorf("got %v, want zero string", val.Interface())
	}

	val, err = coerceSlotValue(nil, reflect.TypeOf(0))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val.Interface() != 0 {
		t.Errorf("got %v, want zero int", val.Interface())
	}

	val, err = coerceSlotValue(nil, reflect.TypeOf(false))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val.Interface() != false {
		t.Errorf("got %v, want false", val.Interface())
	}
}

func TestCoerceSlotValue_DirectAssign(t *testing.T) {
	val, err := coerceSlotValue("hello", reflect.TypeOf(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val.Interface() != "hello" {
		t.Errorf("got %v, want 'hello'", val.Interface())
	}

	val, err = coerceSlotValue(true, reflect.TypeOf(false))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val.Interface() != true {
		t.Errorf("got %v, want true", val.Interface())
	}

	val, err = coerceSlotValue(42, reflect.TypeOf(0))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val.Interface() != 42 {
		t.Errorf("got %v, want 42", val.Interface())
	}
}

func TestCoerceSlotValue_Convert(t *testing.T) {
	// int → int64
	val, err := coerceSlotValue(42, reflect.TypeOf(int64(0)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val.Interface() != int64(42) {
		t.Errorf("got %v (%T), want int64(42)", val.Interface(), val.Interface())
	}

	// int → os.FileMode (uint32)
	val, err = coerceSlotValue(0o755, reflect.TypeOf(os.FileMode(0)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fm := val.Interface().(os.FileMode)
	if fm != 0o755 {
		t.Errorf("got %04o, want 0755", fm)
	}
}

func TestCoerceSlotValue_Constructor(t *testing.T) {
	val, err := coerceSlotValue("/tmp/file", reflect.TypeOf(actionTestResource{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	res := val.Interface().(actionTestResource)
	if res.Path != "/tmp/file" {
		t.Errorf("got %v, want Path=/tmp/file", res)
	}
}

func TestCoerceSlotValue_MapToStruct(t *testing.T) {
	m := map[string]any{"enabled": true, "threshold": 5, "label": "test"}
	val, err := coerceSlotValue(m, reflect.TypeOf(actionConfig{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cfg := val.Interface().(actionConfig)
	if !cfg.Enabled || cfg.Threshold != 5 || cfg.Label != "test" {
		t.Errorf("got %+v, want {Enabled:true Threshold:5 Label:test}", cfg)
	}
}

func TestCoerceSlotValue_MapToStruct_Partial(t *testing.T) {
	// Only some fields set; rest stay zero.
	m := map[string]any{"enabled": true}
	val, err := coerceSlotValue(m, reflect.TypeOf(actionConfig{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cfg := val.Interface().(actionConfig)
	if !cfg.Enabled || cfg.Threshold != 0 || cfg.Label != "" {
		t.Errorf("got %+v, want {Enabled:true Threshold:0 Label:}", cfg)
	}
}

func TestCoerceSlotValue_MapToStruct_UnknownKeys(t *testing.T) {
	// Unknown keys are silently ignored.
	m := map[string]any{"enabled": true, "unknown_key": "ignored"}
	val, err := coerceSlotValue(m, reflect.TypeOf(actionConfig{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cfg := val.Interface().(actionConfig)
	if !cfg.Enabled {
		t.Errorf("got %+v, want Enabled=true", cfg)
	}
}

func TestCoerceSlotValue_Error(t *testing.T) {
	// struct without constructor — cannot coerce from string
	type noConstructor struct{ X int }
	_, err := coerceSlotValue("hello", reflect.TypeOf(noConstructor{}))
	if err == nil {
		t.Fatal("expected error for incompatible types")
	}
	if !strings.Contains(err.Error(), "cannot coerce") {
		t.Errorf("error = %q, want 'cannot coerce' message", err)
	}
}

// --- classifyActionReturn tests ---

func TestClassifyActionReturn_Empty(t *testing.T) {
	result, undo, err := classifyActionReturn(nil)
	if result != nil || undo != nil || err != nil {
		t.Errorf("got (%v, %v, %v), want (nil, nil, nil)", result, undo, err)
	}
}

func TestClassifyActionReturn_ErrorOnly(t *testing.T) {
	// nil error
	results := []reflect.Value{reflect.ValueOf((*error)(nil)).Elem()}
	// Simulating a nil error interface value is tricky with reflect.
	// Use reflect.Zero instead.
	results = []reflect.Value{reflect.Zero(errorType)}
	result, undo, err := classifyActionReturn(results)
	if result != nil || undo != nil || err != nil {
		t.Errorf("got (%v, %v, %v), want (nil, nil, nil)", result, undo, err)
	}

	// non-nil error
	testErr := errors.New("fail")
	results = []reflect.Value{reflect.ValueOf(&testErr).Elem()}
	result, undo, err = classifyActionReturn(results)
	if result != nil || undo != nil || err != testErr {
		t.Errorf("got (%v, %v, %v), want (nil, nil, fail)", result, undo, err)
	}
}

func TestClassifyActionReturn_ValueError(t *testing.T) {
	// (string, error) → success
	results := []reflect.Value{
		reflect.ValueOf("ok"),
		reflect.Zero(errorType),
	}
	result, undo, err := classifyActionReturn(results)
	if result != "ok" || undo != nil || err != nil {
		t.Errorf("got (%v, %v, %v), want ('ok', nil, nil)", result, undo, err)
	}
}

func TestClassifyActionReturn_ValueUndoError(t *testing.T) {
	// (string, map[string]any, error) → compensable
	state := map[string]any{"key": "val"}
	results := []reflect.Value{
		reflect.ValueOf("done"),
		reflect.ValueOf(state),
		reflect.Zero(errorType),
	}
	result, undo, err := classifyActionReturn(results)
	if result != "done" || err != nil {
		t.Errorf("got (%v, _, %v), want ('done', _, nil)", result, err)
	}
	undoMap, ok := undo.(map[string]any)
	if !ok || undoMap["key"] != "val" {
		t.Errorf("undo = %v, want map with key=val", undo)
	}
}

func TestClassifyActionReturn_ErrorPropagation(t *testing.T) {
	testErr := errors.New("bad")
	results := []reflect.Value{
		reflect.ValueOf("ignored"),
		reflect.ValueOf(&testErr).Elem(),
	}
	result, _, err := classifyActionReturn(results)
	// Result is still returned even on error (executor decides what to do).
	if result != "ignored" {
		t.Errorf("result = %v, want 'ignored'", result)
	}
	if err != testErr {
		t.Errorf("err = %v, want %v", err, testErr)
	}
}

// --- RegisterReflectedActions tests ---

func TestRegisterReflectedActions_ActionNames(t *testing.T) {
	reg := NewActionRegistry()
	RegisterReflectedActions(reg, "test", &actionProvider{}, actionParams)

	names := reg.Names()
	sort.Strings(names)

	// Exists is skipped (no error return). Configure, Copy, Create, Delete, Deploy, Mkdir, Noop, Read, Touch, Validate qualify.
	expected := []string{
		"test.configure",
		"test.copy",
		"test.create",
		"test.delete",
		"test.deploy",
		"test.mkdir",
		"test.noop",
		"test.read",
		"test.touch",
		"test.validate",
	}
	if len(names) != len(expected) {
		t.Fatalf("got %v (len %d), want %v (len %d)", names, len(names), expected, len(expected))
	}
	for i, name := range expected {
		if names[i] != name {
			t.Errorf("names[%d] = %q, want %q", i, names[i], name)
		}
	}
}

func TestRegisterReflectedActions_SkipsImmediateOnly(t *testing.T) {
	reg := NewActionRegistry()
	RegisterReflectedActions(reg, "test", &actionProvider{}, actionParams)

	if _, ok := reg.Get("test.exists"); ok {
		t.Error("Exists should not be registered (no error return)")
	}
}

func TestRegisterReflectedActions_CompensableDetection(t *testing.T) {
	reg := NewActionRegistry()
	RegisterReflectedActions(reg, "test", &actionProvider{}, actionParams)

	// Copy has CompensateCopy → CompensableAction.
	action, ok := reg.Get("test.copy")
	if !ok {
		t.Fatal("test.copy not registered")
	}
	if _, ok := action.(CompensableAction); !ok {
		t.Error("test.copy should implement CompensableAction")
	}

	// Read has no CompensateRead → plain Action.
	action, ok = reg.Get("test.read")
	if !ok {
		t.Fatal("test.read not registered")
	}
	if _, ok := action.(CompensableAction); ok {
		t.Error("test.read should NOT implement CompensableAction")
	}
}

func TestRegisterReflectedActions_SkipsMissingMethod(t *testing.T) {
	params := MethodParams{
		"NonExistent": {"a"},
	}
	reg := NewActionRegistry()
	RegisterReflectedActions(reg, "test", &actionProvider{}, params)
	if len(reg.Names()) != 0 {
		t.Errorf("expected no registrations, got %v", reg.Names())
	}
}

// --- reflectedAction.Do tests ---

func TestReflectedAction_Do(t *testing.T) {
	reg := NewActionRegistry()
	RegisterReflectedActions(reg, "test", &actionProvider{}, actionParams)

	action := reg.MustGet("test.read")
	ctx := &Context{
		Context: context.Background(),
		Writer:  io.Discard,
	}
	slots := map[string]any{"path": "/tmp/f"}

	result, undo, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "content:/tmp/f" {
		t.Errorf("result = %v, want 'content:/tmp/f'", result)
	}
	if undo != nil {
		t.Errorf("undo = %v, want nil", undo)
	}
}

func TestReflectedAction_Do_Compensable(t *testing.T) {
	reg := NewActionRegistry()
	RegisterReflectedActions(reg, "test", &actionProvider{}, actionParams)

	action := reg.MustGet("test.copy")
	ctx := &Context{
		Context: context.Background(),
		Writer:  io.Discard,
	}
	slots := map[string]any{"source": "/a", "dest": "/b"}

	result, undo, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "/b" {
		t.Errorf("result = %v, want '/b'", result)
	}
	undoMap, ok := undo.(map[string]any)
	if !ok {
		t.Fatalf("undo = %T, want map[string]any", undo)
	}
	if undoMap["source"] != "/a" || undoMap["dest"] != "/b" {
		t.Errorf("undo = %v, want source=/a, dest=/b", undoMap)
	}
}

func TestReflectedAction_Do_ErrorOnly(t *testing.T) {
	reg := NewActionRegistry()
	RegisterReflectedActions(reg, "test", &actionProvider{}, actionParams)

	action := reg.MustGet("test.validate")
	ctx := &Context{
		Context: context.Background(),
		Writer:  io.Discard,
	}

	// Success.
	_, _, err := action.Do(ctx, map[string]any{"path": "/ok"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Error.
	_, _, err = action.Do(ctx, map[string]any{"path": ""})
	if err == nil {
		t.Fatal("expected error for empty path")
	}
	if err.Error() != "empty path" {
		t.Errorf("err = %q, want 'empty path'", err)
	}
}

func TestReflectedAction_Do_TypeCoercion(t *testing.T) {
	reg := NewActionRegistry()
	RegisterReflectedActions(reg, "test", &actionProvider{}, actionParams)

	action := reg.MustGet("test.mkdir")
	ctx := &Context{
		Context: context.Background(),
		Writer:  io.Discard,
	}
	// Slot value is int (from unmarshalToAny), method expects os.FileMode.
	slots := map[string]any{"path": "/dir", "mode": 0o755}

	result, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "/dir:0755" {
		t.Errorf("result = %v, want '/dir:0755'", result)
	}
}

func TestReflectedAction_Do_Constructor(t *testing.T) {
	reg := NewActionRegistry()
	RegisterReflectedActions(reg, "test", &actionProvider{}, actionParams)

	action := reg.MustGet("test.deploy")
	ctx := &Context{
		Context: context.Background(),
		Writer:  io.Discard,
	}
	// Slot value is string, method expects actionTestResource.
	slots := map[string]any{"res": "/app/bin"}

	result, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "deployed:/app/bin" {
		t.Errorf("result = %v, want 'deployed:/app/bin'", result)
	}
}

func TestReflectedAction_Do_MapToStruct(t *testing.T) {
	reg := NewActionRegistry()
	RegisterReflectedActions(reg, "test", &actionProvider{}, actionParams)

	action := reg.MustGet("test.configure")
	ctx := &Context{
		Context: context.Background(),
		Writer:  io.Discard,
	}
	// Slot value is map[string]any (from unmarshalToAny), method expects actionConfig.
	slots := map[string]any{
		"name": "myapp",
		"cfg":  map[string]any{"enabled": true, "threshold": 10, "label": "prod"},
	}

	result, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "myapp:enabled=true,threshold=10,label=prod" {
		t.Errorf("result = %v, want 'myapp:enabled=true,threshold=10,label=prod'", result)
	}
}

func TestReflectedAction_Do_MissingSlot(t *testing.T) {
	reg := NewActionRegistry()
	RegisterReflectedActions(reg, "test", &actionProvider{}, actionParams)

	action := reg.MustGet("test.read")
	ctx := &Context{
		Context: context.Background(),
		Writer:  io.Discard,
	}
	// Missing "path" slot → coerces nil to zero string.
	result, _, err := action.Do(ctx, map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "content:" {
		t.Errorf("result = %v, want 'content:' (zero string)", result)
	}
}

func TestReflectedAction_Do_CoercionError(t *testing.T) {
	reg := NewActionRegistry()
	RegisterReflectedActions(reg, "test", &actionProvider{}, actionParams)

	action := reg.MustGet("test.read")
	ctx := &Context{
		Context: context.Background(),
		Writer:  io.Discard,
	}
	// Wrong type: map cannot coerce to string.
	slots := map[string]any{"path": map[string]any{"bad": true}}

	_, _, err := action.Do(ctx, slots)
	if err == nil {
		t.Fatal("expected coercion error")
	}
	if !strings.Contains(err.Error(), "param path") {
		t.Errorf("error = %q, want error mentioning 'param path'", err)
	}
}

// --- DryRun tests ---

func TestReflectedAction_DryRun(t *testing.T) {
	reg := NewActionRegistry()
	RegisterReflectedActions(reg, "test", &actionProvider{}, actionParams)

	var buf bytes.Buffer
	action := reg.MustGet("test.read")
	ctx := &Context{
		Context: context.Background(),
		DryRun:  true,
		Writer:  &buf,
	}
	slots := map[string]any{"path": "/tmp/f"}

	result, undo, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil || undo != nil {
		t.Errorf("dry-run should return (nil, nil), got (%v, %v)", result, undo)
	}

	out := buf.String()
	if !strings.Contains(out, "[dry-run] test.read") {
		t.Errorf("output = %q, want '[dry-run] test.read ...'", out)
	}
	if !strings.Contains(out, "/tmp/f") {
		t.Errorf("output = %q, want slot value '/tmp/f'", out)
	}
}

func TestReflectedAction_DryRun_MultipleSlots(t *testing.T) {
	reg := NewActionRegistry()
	RegisterReflectedActions(reg, "test", &actionProvider{}, actionParams)

	var buf bytes.Buffer
	action := reg.MustGet("test.copy")
	ctx := &Context{
		Context: context.Background(),
		DryRun:  true,
		Writer:  &buf,
	}
	slots := map[string]any{"source": "/a", "dest": "/b"}

	_, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "[dry-run] test.copy") {
		t.Errorf("output = %q, want '[dry-run] test.copy ...'", out)
	}
	if !strings.Contains(out, "/a") || !strings.Contains(out, "/b") {
		t.Errorf("output = %q, want slot values '/a' and '/b'", out)
	}
}

// --- Undo tests ---

func TestReflectedCompensableAction_Undo(t *testing.T) {
	reg := NewActionRegistry()
	RegisterReflectedActions(reg, "test", &actionProvider{}, actionParams)

	action := reg.MustGet("test.copy")
	ca, ok := action.(CompensableAction)
	if !ok {
		t.Fatal("test.copy should be CompensableAction")
	}

	ctx := &Context{
		Context: context.Background(),
		Writer:  io.Discard,
	}
	state := map[string]any{"source": "/a", "dest": "/b"}

	err := ca.Undo(ctx, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReflectedCompensableAction_UndoNil(t *testing.T) {
	reg := NewActionRegistry()
	RegisterReflectedActions(reg, "test", &actionProvider{}, actionParams)

	action := reg.MustGet("test.copy")
	ca := action.(CompensableAction)

	ctx := &Context{
		Context: context.Background(),
		Writer:  io.Discard,
	}

	err := ca.Undo(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReflectedCompensableAction_UndoError(t *testing.T) {
	reg := NewActionRegistry()
	RegisterReflectedActions(reg, "test", &actionProvider{}, actionParams)

	action := reg.MustGet("test.copy")
	ca := action.(CompensableAction)

	ctx := &Context{
		Context: context.Background(),
		Writer:  io.Discard,
	}
	state := map[string]any{"fail": true}

	err := ca.Undo(ctx, state)
	if err == nil {
		t.Fatal("expected compensate error")
	}
	if err.Error() != "compensate failed" {
		t.Errorf("err = %q, want 'compensate failed'", err)
	}
}

// --- NoResult tests ---

func TestClassifyActionReturn_NoResult(t *testing.T) {
	results := []reflect.Value{
		reflect.ValueOf(NoResult{}),
		reflect.Zero(errorType),
	}
	result, undo, err := classifyActionReturn(results)
	if result != nil {
		t.Errorf("result = %v, want nil (NoResult sentinel)", result)
	}
	if undo != nil || err != nil {
		t.Errorf("undo = %v, err = %v, want (nil, nil)", undo, err)
	}
}

func TestClassifyActionReturn_NoResult_WithUndo(t *testing.T) {
	state := map[string]any{"path": "/removed"}
	results := []reflect.Value{
		reflect.ValueOf(NoResult{}),
		reflect.ValueOf(state),
		reflect.Zero(errorType),
	}
	result, undo, err := classifyActionReturn(results)
	if result != nil {
		t.Errorf("result = %v, want nil (NoResult sentinel)", result)
	}
	undoMap, ok := undo.(map[string]any)
	if !ok || undoMap["path"] != "/removed" {
		t.Errorf("undo = %v, want map with path=/removed", undo)
	}
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}
}

func TestReflectedAction_Do_NoResult(t *testing.T) {
	reg := NewActionRegistry()
	RegisterReflectedActions(reg, "test", &actionProvider{}, actionParams)

	action := reg.MustGet("test.delete")
	ctx := &Context{
		Context: context.Background(),
		Writer:  io.Discard,
	}

	result, undo, err := action.Do(ctx, map[string]any{"path": "/tmp/gone"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("result = %v, want nil (NoResult)", result)
	}
	undoMap, ok := undo.(map[string]any)
	if !ok || undoMap["path"] != "/tmp/gone" {
		t.Errorf("undo = %v, want map with path=/tmp/gone", undo)
	}
}

func TestReflectedAction_Delete_IsCompensable(t *testing.T) {
	reg := NewActionRegistry()
	RegisterReflectedActions(reg, "test", &actionProvider{}, actionParams)

	action, ok := reg.Get("test.delete")
	if !ok {
		t.Fatal("test.delete not registered")
	}
	if _, ok := action.(CompensableAction); !ok {
		t.Error("test.delete should implement CompensableAction")
	}
}

// --- Catalog shadow tests ---

func TestReflectedAction_Do_ShadowsResource(t *testing.T) {
	reg := NewActionRegistry()
	RegisterReflectedActions(reg, "test", &actionProvider{}, actionParams)

	catalog := NewResourceCatalog()
	action := reg.MustGet("test.create")
	ctx := &Context{
		Context: context.Background(),
		Catalog: catalog,
		NodeID:  "node-1",
		Writer:  io.Discard,
	}

	result, _, err := action.Do(ctx, map[string]any{"path": "/tmp/new"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Result should remain a value type (not promoted to pointer).
	ar, ok := result.(actionResource)
	if !ok {
		t.Fatalf("result type = %T, want actionResource", result)
	}
	if ar.SourcePath != "/tmp/new" {
		t.Errorf("SourcePath = %q, want %q", ar.SourcePath, "/tmp/new")
	}

	// Catalog should have the shadowed entry.
	if catalog.Len() != 1 {
		t.Fatalf("catalog.Len() = %d, want 1", catalog.Len())
	}
	id := catalog.Current("file:///tmp/new")
	if id == "" {
		t.Fatal("catalog has no entry for file:///tmp/new")
	}
	entry, ok := catalog.Lookup(id)
	if !ok {
		t.Fatalf("catalog.Lookup(%q) failed", id)
	}
	base := entry.resourceBase()
	if base.originID != "node-1" {
		t.Errorf("originID = %q, want %q", base.originID, "node-1")
	}

	// extractResource should find the stamped originID on the value result.
	origin, found := extractResource(result)
	if !found {
		t.Fatal("extractResource did not find resource identity on value result")
	}
	if origin != "node-1" {
		t.Errorf("extractResource originID = %q, want %q", origin, "node-1")
	}
}

func TestReflectedAction_Do_NoCatalog_Unchanged(t *testing.T) {
	reg := NewActionRegistry()
	RegisterReflectedActions(reg, "test", &actionProvider{}, actionParams)

	// No catalog — result should be returned unchanged.
	action := reg.MustGet("test.create")
	ctx := &Context{
		Context: context.Background(),
		Writer:  io.Discard,
	}

	result, _, err := action.Do(ctx, map[string]any{"path": "/tmp/file"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := result.(actionResource); !ok {
		t.Errorf("result type = %T, want actionResource", result)
	}
}

func TestReflectedAction_Do_NonResource_Unchanged(t *testing.T) {
	reg := NewActionRegistry()
	RegisterReflectedActions(reg, "test", &actionProvider{}, actionParams)

	catalog := NewResourceCatalog()
	action := reg.MustGet("test.read")
	ctx := &Context{
		Context: context.Background(),
		Catalog: catalog,
		NodeID:  "node-2",
		Writer:  io.Discard,
	}

	result, _, err := action.Do(ctx, map[string]any{"path": "/tmp/f"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "content:/tmp/f" {
		t.Errorf("result = %v, want 'content:/tmp/f'", result)
	}
	// Non-resource result should not shadow.
	if catalog.Len() != 0 {
		t.Errorf("catalog.Len() = %d, want 0 (non-resource)", catalog.Len())
	}
}

func TestReflectedAction_Do_NoResult_NotShadowed(t *testing.T) {
	reg := NewActionRegistry()
	RegisterReflectedActions(reg, "test", &actionProvider{}, actionParams)

	catalog := NewResourceCatalog()
	action := reg.MustGet("test.delete")
	ctx := &Context{
		Context: context.Background(),
		Catalog: catalog,
		NodeID:  "node-3",
		Writer:  io.Discard,
	}

	result, _, err := action.Do(ctx, map[string]any{"path": "/tmp/gone"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("result = %v, want nil (NoResult)", result)
	}
	if catalog.Len() != 0 {
		t.Errorf("catalog.Len() = %d, want 0 (NoResult)", catalog.Len())
	}
}

// --- Pairing validation tests ---

// unpairedProvider has a method with 3 returns but no Compensate companion.
type unpairedProvider struct{}

func (p *unpairedProvider) Destroy(path string) (string, map[string]any, error) {
	return "", nil, nil
}

func TestRegisterReflectedActions_MissingCompensate_Panics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for missing Compensate method")
		}
		msg := fmt.Sprint(r)
		if !strings.Contains(msg, "CompensateDestroy") {
			t.Errorf("panic message = %q, want mention of CompensateDestroy", msg)
		}
	}()

	reg := NewActionRegistry()
	RegisterReflectedActions(reg, "test", &unpairedProvider{}, MethodParams{
		"Destroy": {"path"},
	})
}
