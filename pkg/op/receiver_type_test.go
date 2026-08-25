// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// region TEST FIXTURES

// testReceipt is a dummy implementation of [Receipt] for testing compensable methods.
type testReceipt struct {
	ReceiptBase
	Data string
}

func (r *testReceipt) URI() string { return "test:" + r.Data }

// testProvider is a minimal provider with methods covering all MethodKind classifications.
type testProvider struct{}

// Action — returns nothing.
func (p *testProvider) Ping() {}

// FallibleAction — returns error.
func (p *testProvider) Validate(value string) error {
	if value == "" {
		return errors.New("empty")
	}
	return nil
}

// Function — returns a result.
func (p *testProvider) Echo(msg string) string { return msg }

// FallibleFunction — returns (T, error).
func (p *testProvider) Parse(input string) (int, error) {
	if input == "bad" {
		return 0, errors.New("parse error")
	}
	return 42, nil
}

// CompensableFunction — returns (T, U, error); activation-first per the step-27 required floor.
func (p *testProvider) Create(activationRecord *ActivationRecord, name string) (string, *testReceipt, error) {
	return "created:" + name, &testReceipt{ReceiptBase: NewReceiptBase(nil), Data: name}, nil
}

// CompensateCreate is the required companion for Create; activation-first per the step-27 required floor.
func (p *testProvider) CompensateCreate(activationRecord *ActivationRecord, compensator *testReceipt) error {
	return nil
}

// MultiParam — multiple parameters.
func (p *testProvider) Multi(a string, b int, c bool) string {
	if c {
		return a
	}
	return ""
}

var testProviderType = reflect.TypeFor[*testProvider]()

// floorViolationProvider carries a compensable method WITHOUT the mandated leading activation — the step-27
// required floor must reject it at registration.
type floorViolationProvider struct{ ProviderBase }

// Create is compensable-shaped but omits the activation: the floor violation under test.
func (p *floorViolationProvider) Create(name string) (string, *testReceipt, error) {
	return name, nil, nil
}

// CompensateCreate conforms (the violation under test is Create's).
func (p *floorViolationProvider) CompensateCreate(activationRecord *ActivationRecord, compensator *testReceipt) error {
	return nil
}

// companionViolationProvider carries a conforming compensable method whose companion omits the activation.
type companionViolationProvider struct{ ProviderBase }

// Create conforms (the violation under test is the companion's).
func (p *companionViolationProvider) Create(activationRecord *ActivationRecord, name string) (string, *testReceipt, error) {
	return name, nil, nil
}

// CompensateCreate omits the mandated activation: the companion floor violation under test.
func (p *companionViolationProvider) CompensateCreate(compensator *testReceipt) error { return nil }

// TestNewReceiverType_RequiredFloor_CompensableWithoutActivation pins the step-27 registration backstop: a
// compensable action lacking the leading activation is rejected.
func TestNewReceiverType_RequiredFloor_CompensableWithoutActivation(t *testing.T) {

	_, err := newReceiverType(
		reflect.TypeFor[*floorViolationProvider](),
		mustParseParameters(t, reflect.TypeFor[*floorViolationProvider](), map[string][]string{"Create": {"name"}}),
		nil,
		true,
	)
	if err == nil || !strings.Contains(err.Error(), "step 27") {
		t.Fatalf("newReceiverType over the floor violation = %v; want the step-27 rejection", err)
	}
}

// TestNewReceiverType_RequiredFloor_CompanionWithoutActivation pins the companion half: a Compensate* companion
// lacking the activation is rejected.
func TestNewReceiverType_RequiredFloor_CompanionWithoutActivation(t *testing.T) {

	_, err := newReceiverType(
		reflect.TypeFor[*companionViolationProvider](),
		mustParseParameters(t, reflect.TypeFor[*companionViolationProvider](), map[string][]string{"Create": {"name"}}),
		nil,
		true,
	)
	if err == nil || !strings.Contains(err.Error(), "step 27") {
		t.Fatalf("newReceiverType over the companion violation = %v; want the step-27 rejection", err)
	}
}

// mustParseParameters cracks a token-form methodParameters map into Parameter values for the test, failing the
// test on parse error. Tests express their inputs in the token form (matching codegen output); this helper drives
// them through parseParameters so the receiver-construction path under test consumes typed Parameter values just
// as production code does.
func mustParseParameters(t *testing.T, providerType reflect.Type, m map[string][]string) map[string][]Parameter {
	t.Helper()
	parsed, err := parseParameters(providerType, m)
	if err != nil {
		t.Fatalf("parseParameters: %v", err)
	}
	return parsed
}

// endregion

func TestNewReceiverType_WithNamedParameters(t *testing.T) {

	parsed := mustParseParameters(t, testProviderType, map[string][]string{
		"Echo":  {"msg"},
		"Parse": {"input"},
	})
	rt, err := newReceiverType(testProviderType, parsed, nil, false)

	if err != nil {
		t.Fatalf("newReceiverType: %v", err)
	}

	if rt.Name() != "testProvider" {
		t.Errorf("Name() = %q, want \"testProvider\"", rt.Name())
	}

	if rt.ProviderType() != testProviderType {
		t.Errorf("ProviderType() = %v, want %v", rt.ProviderType(), testProviderType)
	}

	// Check methods
	m, ok := rt.MethodByName("Echo")
	if !ok {
		t.Fatal("Echo method not found")
	}
	if len(m.Parameters()) != 1 || m.Parameters()[0].Name != "msg" {
		t.Errorf("Echo params = %v, want [{msg ...}]", m.Parameters())
	}
}

func TestNewReceiverType_WithNilParameters_RegistersAllMethods(t *testing.T) {

	rt, err := newReceiverType(testProviderType, nil, nil, false)
	if err != nil {
		t.Fatalf("newReceiverType: %v", err)
	}

	count := 0
	for range rt.Methods() {
		count++
	}
	if count == 0 {
		t.Fatal("nil parameters should register all methods, got 0")
	}

	// Check that Echo got a numeric parameter name.
	m, ok := rt.MethodByName("Echo")
	if !ok {
		t.Fatal("Echo not in methodMap")
	}
	if len(m.Parameters()) != 1 || m.Parameters()[0].Name != "0" {
		t.Errorf("Echo params = %v, want [{0 ...}]", m.Parameters())
	}
}

func TestNewReceiverType_WithNilParameters_MultiParam(t *testing.T) {

	rt, err := newReceiverType(testProviderType, nil, nil, false)
	if err != nil {
		t.Fatalf("newReceiverType: %v", err)
	}

	m, ok := rt.MethodByName("Multi")
	if !ok {
		t.Fatal("Multi not in methodMap")
	}

	if len(m.Parameters()) != 3 {
		t.Fatalf("Multi params = %d, want 3", len(m.Parameters()))
	}

	for i, p := range m.Parameters() {
		want := strconv.Itoa(i)
		if p.Name != want {
			t.Errorf("param[%d] name = %q, want %q", i, p.Name, want)
		}
	}
}

func TestNewReceiverType_WithEmptyParameters_RegistersNoMethods(t *testing.T) {

	rt, err := newReceiverType(testProviderType, make(map[string][]Parameter), nil, false)
	if err != nil {
		t.Fatalf("newReceiverType: %v", err)
	}

	count := 0
	for range rt.Methods() {
		count++
	}
	if count != 0 {
		t.Errorf("empty parameters should register 0 methods, got %d", count)
	}
}

func TestNewReceiverType_MethodsSorted(t *testing.T) {

	parsed := mustParseParameters(t, testProviderType, map[string][]string{
		"Echo":  {"msg"},
		"Parse": {"input"},
		"Ping":  {},
	})
	rt, err := newReceiverType(testProviderType, parsed, nil, false)
	if err != nil {
		t.Fatalf("newReceiverType: %v", err)
	}

	var names []string
	for m := range rt.Methods() {
		names = append(names, m.Name())
	}

	// Check alphabetical order
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Errorf("Methods not sorted: %v", names)
		}
	}
}

func TestNewReceiverType_WrongParamCount_ReturnsError(t *testing.T) {

	// Wrong param count is now caught at the announce boundary by parseParameters' bounds check, before
	// newReceiverType is reached. The test exercises that boundary directly.
	_, err := parseParameters(testProviderType, map[string][]string{
		"Echo": {"a", "b"}, // Echo takes 1 param, not 2
	})

	if err == nil {
		t.Fatal("expected error for wrong parameter count, got nil")
	}
}

func TestNewReceiverType_RejectsReservedParameterNames(t *testing.T) {

	cases := []struct {
		name   string
		params []string
	}{
		{"options", []string{"options"}},
		{"options?", []string{"options?"}},
		{"*options", []string{"*options"}},
		{"args", []string{"args"}},
		{"kwargs", []string{"kwargs"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parsed := mustParseParameters(t, testProviderType, map[string][]string{
				"Echo": tc.params,
			})
			_, err := newReceiverType(testProviderType, parsed, nil, false)

			if err == nil {
				t.Fatalf("expected error for reserved name %q, got nil", tc.name)
			}
		})
	}
}

func TestNewReceiverType_RejectsReservedParameterNames_NamesProviderMethodParam(t *testing.T) {

	parsed := mustParseParameters(t, testProviderType, map[string][]string{
		"Echo": {"options"},
	})

	_, err := newReceiverType(testProviderType, parsed, nil, false)
	if err == nil {
		t.Fatal("expected error for reserved name, got nil")
	}

	message := err.Error()
	for _, want := range []string{receiverName(testProviderType), "Echo", "options"} {
		if !strings.Contains(message, want) {
			t.Errorf("error %q does not name %q (provider / method / offending parameter must all appear)", message, want)
		}
	}
}

func TestNewReceiverType_AllowsVariadicMarkers(t *testing.T) {

	cases := []struct {
		name   string
		params []string
	}{
		{"*args", []string{"*args"}},
		{"**kwargs", []string{"**kwargs"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// testProvider doesn't have these, but validation happens before method lookup
			parsed := mustParseParameters(t, testProviderType, map[string][]string{
				"Echo": tc.params,
			})
			_, err := newReceiverType(testProviderType, parsed, nil, false)

			// Should SUCCEED: markers are allowed even if Go method is not variadic (mappings land in slots)
			if err != nil {
				t.Fatalf("expected success, got %v", err)
			}
		})
	}
}

func TestReceiverType_Methods_Iterator(t *testing.T) {

	parsed := mustParseParameters(t, testProviderType, map[string][]string{
		"Echo":  {"msg"},
		"Parse": {"input"},
	})
	rt, err := newReceiverType(testProviderType, parsed, nil, false)
	if err != nil {
		t.Fatalf("newReceiverType: %v", err)
	}

	var names []string
	for m := range rt.Methods() {
		names = append(names, m.Name())
	}

	if len(names) != 2 {
		t.Errorf("Methods() returned %d items, want 2", len(names))
	}
}

func TestReceiverType_MethodByName(t *testing.T) {

	rt, _ := newReceiverType(testProviderType, mustParseParameters(t, testProviderType, map[string][]string{"Echo": {"msg"}}), nil, false)

	if _, ok := rt.MethodByName("Echo"); !ok {
		t.Error("MethodByName(Echo) failed")
	}

	if _, ok := rt.MethodByName("Missing"); ok {
		t.Error("MethodByName(Missing) succeeded, want false")
	}
}

func TestReceiverType_ProviderType(t *testing.T) {

	rt, _ := newReceiverType(testProviderType, nil, nil, false)
	if rt.ProviderType() != testProviderType {
		t.Errorf("ProviderType() = %v, want %v", rt.ProviderType(), testProviderType)
	}
}

func TestReceiverType_Name_Resource(t *testing.T) {

	type testLocalResource struct{ ResourceBase }
	rt, _ := newReceiverType(reflect.TypeFor[*testLocalResource](), nil, nil, false)

	if rt.Name() != "testLocalResource" {
		t.Errorf("Name() = %q, want \"testLocalResource\"", rt.Name())
	}
}

// region Do

func TestDo_Action(t *testing.T) {

	rt, _ := newReceiverType(testProviderType, mustParseParameters(t, testProviderType, map[string][]string{"Ping": {}}), nil, false)
	p := &testProvider{}

	res, undo, err := rt.Do("Ping", p, nil)

	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if res.IsValid() {
		t.Errorf("result = %v, want invalid", res)
	}
	if undo.IsValid() {
		t.Errorf("undo = %v, want invalid", undo)
	}
}

func TestDo_FallibleAction(t *testing.T) {

	rt, _ := newReceiverType(testProviderType, mustParseParameters(t, testProviderType, map[string][]string{"Validate": {"value"}}), nil, false)
	p := &testProvider{}

	// Success
	_, _, err := rt.Do("Validate", p, []any{""}) // "" fails
	if err == nil {
		t.Error("expected error for empty value")
	}

	// Failure
	_, _, err = rt.Do("Validate", p, []any{"ok"})
	if err != nil {
		t.Errorf("Do: %v", err)
	}
}

func TestDo_Function(t *testing.T) {

	rt, _ := newReceiverType(testProviderType, mustParseParameters(t, testProviderType, map[string][]string{"Echo": {"msg"}}), nil, false)
	p := &testProvider{}

	res, _, err := rt.Do("Echo", p, []any{"hello"})

	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if res.String() != "hello" {
		t.Errorf("result = %q, want \"hello\"", res.String())
	}
}

func TestDo_FallibleFunction(t *testing.T) {

	rt, _ := newReceiverType(testProviderType, mustParseParameters(t, testProviderType, map[string][]string{"Parse": {"input"}}), nil, false)
	p := &testProvider{}

	// Success
	res, _, err := rt.Do("Parse", p, []any{"123"})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if res.Int() != 42 {
		t.Errorf("result = %d, want 42", res.Int())
	}

	// Failure
	_, _, err = rt.Do("Parse", p, []any{"bad"})
	if err == nil {
		t.Error("expected error for 'bad' input")
	}
}

func TestDo_CompensableFunction(t *testing.T) {

	rt, _ := newReceiverType(testProviderType, mustParseParameters(t, testProviderType, map[string][]string{"Create": {"name"}}), nil, true)
	p := &testProvider{}

	result, compensator, err := rt.Do("Create", p, []any{NewActivationRecord(nil, "", &RuntimeEnvironment{}), "foo"})

	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if result.String() != "created:foo" {
		t.Errorf("result = %q, want %q", result.String(), "created:foo")
	}
	if !compensator.IsValid() {
		t.Fatal("expected valid compensator for CompensableFunction")
	}
}

// endregion

// region Do — nil args

func TestDo_NilArg_BecomesZeroValue(t *testing.T) {

	parsed := mustParseParameters(t, testProviderType, map[string][]string{
		"Echo": {"msg"},
	})
	rt, err := newReceiverType(testProviderType, parsed, nil, false)
	if err != nil {
		t.Fatalf("newReceiverType: %v", err)
	}

	p := &testProvider{}
	res, _, err := rt.Do("Echo", p, []any{nil})

	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if res.String() != "" {
		t.Errorf("result = %q, want empty string for nil arg", res.String())
	}
}

// endregion

// region ProviderReceiverType (public)

func TestNewProviderReceiverType(t *testing.T) {

	pType := reflect.TypeFor[*testProvider]()
	construct := func(runtimeEnvironment *RuntimeEnvironment) (any, error) { return &testProvider{}, nil }

	rt, err := NewProviderReceiverType(pType, construct, RoleAction, mustParseParameters(t, pType, map[string][]string{
		"Echo": {"msg"},
	}), nil)

	if err != nil {
		t.Fatalf("NewProviderReceiverType: %v", err)
	}

	if rt.Roles() != RoleAction {
		t.Errorf("Roles() = %v, want RoleAction", rt.Roles())
	}

	p, _ := rt.Construct()(nil)
	if _, ok := p.(*testProvider); !ok {
		t.Errorf("Construct() returned %T, want *testProvider", p)
	}
}

// endregion

// region ResourceReceiverType (public)

func TestNewResourceReceiverType_Public(t *testing.T) {

	type testLocalResource struct{ ResourceBase }
	rType := reflect.TypeFor[*testLocalResource]()

	construct := func(runtimeEnvironment *RuntimeEnvironment, identity any) (Resource, error) {
		return &testLocalResource{}, nil
	}

	// A struct-announced resource is its own implementation — both arguments are the same type, which is
	// what every provider passes until it is sealed.
	rt, err := NewResourceReceiverType(rType, rType, construct, mustParseParameters(t, rType, map[string][]string{
		"URI": {},
	}))

	if err != nil {
		t.Fatalf("NewResourceReceiverType: %v", err)
	}

	r, _ := rt.Construct()(nil, "id")
	if _, ok := r.(*testLocalResource); !ok {
		t.Errorf("Construct() returned %T, want *testLocalResource", r)
	}
}

// endregion

// region NewReceiverType (public)

func TestNewReceiverType_Public(t *testing.T) {

	rt, err := NewReceiverType(testProviderType, nil)

	if err != nil {
		t.Fatalf("NewReceiverType: %v", err)
	}

	if rt.Name() != "testProvider" {
		t.Errorf("Name() = %q, want \"testProvider\"", rt.Name())
	}
}

// endregion

// region ReceiverRegistry concurrency

type concurrentResolveA struct{ A int }

type concurrentResolveB struct{ B string }

// TestReceiverRegistry_TypeByReflectionOrDerive_Concurrent exercises the byType mutex under contention. Many
// goroutines resolve a mix of value and pointer forms of unannounced types against one registry, so the RLock fast
// path, the WLock derive, and the post-acquire re-check all run concurrently. Run with -race to validate the guard.
func TestReceiverRegistry_TypeByReflectionOrDerive_Concurrent(t *testing.T) {

	registry := ReceiverRegistry()

	types := []reflect.Type{
		reflect.TypeFor[concurrentResolveA](),
		reflect.TypeFor[*concurrentResolveA](),
		reflect.TypeFor[concurrentResolveB](),
		reflect.TypeFor[*concurrentResolveB](),
	}

	var wg sync.WaitGroup

	for i := range 200 {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			if rt := registry.TypeByReflectionOrDerive(types[i%len(types)]); rt == nil {
				t.Errorf("TypeByReflectionOrDerive(%v) = nil", types[i%len(types)])
			}
		}(i)
	}

	wg.Wait()
}

// endregion

// --- ProviderRole zones (step 2) ---

func TestProviderRole_ZoneBitLayout(t *testing.T) {

	cases := []struct {
		name string
		got  ProviderRole
		want ProviderRole
	}{
		{"RoleModule", RoleModule, 0x01},
		{"RoleAction", RoleAction, 0x02},
		{"RoleRoot", RoleRoot, 0x100},
		{"dispatch mask", roleDispatchMask, 0x00FF},
		{"placement mask", rolePlacementMask, 0xFF00},
	}

	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s = %#x, want %#x", tc.name, uint(tc.got), uint(tc.want))
		}
	}
}

func TestProviderRole_Dispatch_ReturnsDispatchZoneOnly(t *testing.T) {

	if got := (RoleAction | RoleRoot).Dispatch(); got != RoleAction {
		t.Errorf("(RoleAction|RoleRoot).Dispatch() = %#x, want RoleAction", uint(got))
	}
	if got := RoleRoot.Dispatch(); got != 0 {
		t.Errorf("RoleRoot.Dispatch() = %#x, want 0 (placement bits must not leak into the dispatch zone)", uint(got))
	}
}

func TestProviderRole_Placement_ReturnsPlacementZoneOnly(t *testing.T) {

	if got := (RoleAction | RoleRoot).Placement(); got != RoleRoot {
		t.Errorf("(RoleAction|RoleRoot).Placement() = %#x, want RoleRoot", uint(got))
	}
	if got := RoleAction.Placement(); got != 0 {
		t.Errorf("RoleAction.Placement() = %#x, want 0 (dispatch bits must not leak into the placement zone)", uint(got))
	}
}
