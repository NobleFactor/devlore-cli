// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

var (
	// activationRecordType is cached for detecting provider methods whose first parameter is an [*ActivationRecord].
	//
	// [Method.Invoke] autofills the [*ActivationRecord] with the per-dispatch record carrying the runtime environment,
	// producing-node identity, and per-call cancellation context. `context.Context` as a first parameter is NOT
	// supported. Methods that need cancellation access it via `activationRecord.Context`.
	activationRecordType = reflect.TypeFor[*ActivationRecord]()

	// errorType is cached for return-type classification.
	errorType = reflect.TypeFor[error]()

	// receiptType is cached for the [MethodCompensableFunction] compensator-shape check.
	receiptType = reflect.TypeFor[Receipt]()

	// recoveryStackType is cached for the [MethodCompensableFunction] compensator-shape check.
	//
	// [Compensator] values typed as [*RecoveryStack] are recognized by [Method.Invoke] as engine-built sagas (e.g., the
	// value WalkTree returns) and spliced into the parent stack via [RecoveryStack.PushNested] rather than being
	// treated as a single [Receipt].
	recoveryStackType = reflect.TypeFor[*RecoveryStack]()
)

// Method describes a callable method on a provider or resource.
//
// It is shared metadata used by both action receiverTypes and starlark receivers. Actions wrap a Method for graph
// dispatch. Starlark receivers wrap a Method for immediate dispatch. Method itself is neither — it is the callable they
// both delegate to.
//
// Any method of a provider may have a plan companion; no method need have one. Companions are discovered by reflection
// on the receiver type, using a name-prefix convention:
//   - `plan (Plan<Name>)`: plan-time output spec, computes the identity of the resource the method will produce from
//     the same inputs. Pure — no I/O.
//   - `undo (Compensate<Name>)`: compensation companion for compensable methods, takes the compensator returned by the
//     forward method and reverses its effect.
type Method struct {
	actionName string          // canonical <pkg-path>.<receiverName>.<methodName>; computed at NewMethod
	do         *reflect.Method // forward method
	kind       MethodKind      // classified from return signature
	modifiers  MethodModifiers // surface modifiers (e.g. ModifierProperty), stamped at announcement
	parameters []Parameter     // named parameters (excluding receiver and any leading activation)
	plan       *reflect.Method // plan-time output spec companion; nil if the method has no plan companion
	planner    Planner         // plan-mode dispatch strategy; nil for resource methods; default ActionPlanner for provider methods
	undo       *reflect.Method // compensation companion; nil unless compensable

	// BY DESIGN (step 27, the required-floor rule): this bit is the mechanism serving the permissive read side,
	// not a wart. Activation-first is REQUIRED for compensable actions and Compensate* companions (validated
	// below and by codegen) and PERMITTED for everything else, so fallible and pure actions legitimately vary
	// (json/yaml.Parse claim production through theirs; most reads take none). The bridge injects per this bit
	// at dispatch. The companion's counterpart bit is gone: the floor mandates the activation-first companion
	// shape, so the undo adapter (step 43) is single-shape by construction.
	firstParamIsActivation bool // true when `do`'s first parameter (after receiver) is *ActivationRecord

	// The registration-baked dispatch adapters (step 43: reflect once at registration). Each closes over its
	// resolved reflect method and every signature-derived decision — the variadic branch, the mandated
	// companion shape — so the invoke paths make plain calls with no per-call reflection decisions.
	doInvoke   func(reflectArgs []reflect.Value) []reflect.Value                       // the forward call; built at NewMethod
	undoInvoke func(receiver any, activation *ActivationRecord, compensator any) error // nil when no companion
}

// NewMethod creates a [Method] from a reflected method, its parameter names, and its optional plan and undo companions.
//
// Classification rules:
//   - [MethodAction] returns nothing ()
//   - [MethodFallibleAction] returns an error or nil (error)
//   - [MethodFunction] returns a single result (T)
//   - [MethodFallibleFunction] returns a single result and an error (T, error)
//   - [MethodCompensableFunction] returns a single result, its compensator, and an error (T, U, error)
//
// Returns an error if:
//   - paramNames length doesn't match method parameter count (excluding receiver)
//   - return signature does not match any known method kind
//   - plan companion provided for a method that produces no result
//   - plan companion parameter list differs from do
//   - plan companion return signature is not (T, error) where T matches `do`'s first result
//   - compensable method has no Compensate companion (if enforceCompanions is true)
//   - Compensate companion signature is invalid
//
// Parameters:
//   - `do`: the reflected Go method to wrap.
//   - `parameters`: parsed Parameter values matching the method's non-receiver parameters. Token-form parsing happens
//     upstream in parseParameters at the announcement boundary; NewMethod consumes typed Parameters only.
//   - `plan`: the Plan<Name> companion method, or nil if the method has no plan companion.
//   - `undo`: the Compensate companion method, or nil for non-compensable methods.
//   - `enforceCompanions`: true if this method belongs to a provider; enables companion requirements.
//
// Returns:
//   - `*Method`: the classified method.
//   - `error`: non-nil if validation fails.
func NewMethod(
	do *reflect.Method,
	parameters []Parameter,
	plan *reflect.Method,
	undo *reflect.Method,
	enforceCompanions bool,
) (*Method, error) {

	methodType := do.Type

	// Detect whether the first Go parameter (after the receiver at index 0) is an [*ActivationRecord]. If so,
	// [Method.Invoke] autofills it with the per-dispatch record and the remaining Go parameters align with the caller
	// supplied parameter names. The `announce` map lists user-declared parameters only — the activation is implicit.

	firstParamIsActivation := methodType.NumIn() >= 2 && methodType.In(1) == activationRecordType

	expectedParams := methodType.NumIn() - 1
	if firstParamIsActivation {
		expectedParams--
	}

	if len(parameters) != expectedParams {

		names := make([]string, len(parameters))

		for i, p := range parameters {
			names[i] = p.Name
		}

		return nil, fmt.Errorf("expected %d parameter names for method %s, not %d: %s",
			expectedParams,
			do.Name,
			len(parameters),
			strings.Join(names, ", "))
	}

	if err := validateParameterPositions(do, parameters); err != nil {
		return nil, err
	}

	kind, err := classifyMethodKind(do, enforceCompanions)
	if err != nil {
		return nil, err
	}

	if err := validatePlanCompanion(do, plan, kind); err != nil {
		return nil, err
	}

	// A compensable forward (three returns) MAY declare a Compensate<Name> companion, attached below as undo, but no
	// longer must: a receipt can name its own undo via compensatingAction, resolved through the registry's
	// compensating-action index. When no companion is found, undo stays nil and compensation routes through the
	// receipt instead.

	// The required floor (step 27): a compensable action cannot claim production or stamp receipts without
	// dispatch identity, so its first parameter MUST be the activation. Codegen enforces the same rule at
	// generation time; this is the registration-side backstop for hand-announced types.
	if enforceCompanions && kind == MethodCompensableFunction && !firstParamIsActivation {
		return nil, fmt.Errorf(
			"method %s is compensable and must declare *ActivationRecord as its first parameter (step 27)",
			do.Name)
	}

	undoInvoke, err := buildUndoInvoke(do, undo, kind)
	if err != nil {
		return nil, err
	}

	// The input parameters carry Name, Type, Optional, Variadic, Kwargs, and Default already; Type was set upstream by
	// parseParameters or by newReceiverType's auto-positional path. Defensive copy so the Method's internal slice is
	// independent of the caller's input.

	params := make([]Parameter, len(parameters))
	copy(params, parameters)

	receiverType := do.Type.In(0)

	if receiverType.Kind() == reflect.Pointer {
		receiverType = receiverType.Elem()
	}

	actionName := receiverType.PkgPath() + "." + receiverType.Name() + "." + do.Name

	// The forward adapter (step 43): the variadic branch is a signature fact, decided once here — the invoke
	// path calls the adapter with its assembled arguments and never re-derives dispatch shape per call.
	doFn := do.Func
	var doInvoke func([]reflect.Value) []reflect.Value
	if do.Type.IsVariadic() {
		doInvoke = doFn.CallSlice
	} else {
		doInvoke = doFn.Call
	}

	return &Method{
		actionName:             actionName,
		do:                     do,
		doInvoke:               doInvoke,
		firstParamIsActivation: firstParamIsActivation,
		kind:                   kind,
		parameters:             params,
		plan:                   plan,
		undo:                   undo,
		undoInvoke:             undoInvoke,
	}, nil
}

// region EXPORTED METHODS

// region State management

// ActionName returns the canonical action name for this method.
//
// Returns:
//   - `string`: the canonical `<pkg-path>.<receiver>.<method>` action name computed at construction.
func (m *Method) ActionName() string { return m.actionName }

// Kind returns the classification of this method's signature.
//
// Returns:
//   - `MethodKind`: the signature classification computed at construction.
func (m *Method) Kind() MethodKind { return m.kind }

// Modifiers returns the surface modifiers stamped on this method.
//
// Returns:
//   - `MethodModifiers`: the modifier set, or [ModifierNone] when none were declared.
func (m *Method) Modifiers() MethodModifiers { return m.modifiers }

// setModifiers stamps the surface modifiers on this method.
//
// Called by the announcement path; the modifier set originates in the codegen-emitted [MethodMetadata.Modifiers] for
// the method.
//
// Parameters:
//   - `modifiers`: the modifier set to stamp.
func (m *Method) setModifiers(modifiers MethodModifiers) { m.modifiers = modifiers }

// Name returns the short name of the method.
//
// Returns:
//   - `string`: the method's short Go name.
func (m *Method) Name() string { return m.do.Name }

// ParameterByName returns the Parameter with the given name, if any.
//
// Parameters:
//   - `name`: the parameter name to look up.
//
// Returns:
//   - `Parameter`: the matching parameter, or the zero `Parameter` when none matches.
//   - `bool`: true when a parameter with `name` exists.
func (m *Method) ParameterByName(name string) (Parameter, bool) {

	for _, p := range m.parameters {
		if p.Name == name {
			return p, true
		}
	}

	return Parameter{}, false
}

// Parameters returns the named parameters of the method, excluding the receiver and any leading context.Context.
//
// Returns:
//   - `[]Parameter`: the named parameters, excluding the receiver and any leading [*ActivationRecord].
func (m *Method) Parameters() []Parameter { return m.parameters }

// Planner returns the plan-mode dispatch strategy for this method.
//
// Nil for resource methods (resources are not plan-dispatchable). Provider methods carry the planner declared at
// announcement; absent declaration means [ActionPlanner].
//
// Returns:
//   - `Planner`: the dispatch strategy, or nil for resource methods.
func (m *Method) Planner() Planner { return m.planner }

// setPlanner stamps the plan-mode dispatch strategy on this method.
//
// Called by the receiver-type construction path at announcement time. Resource methods skip this call; provider methods
// receive either the announcement-declared planner or [ActionPlanner] by default.
//
// Parameters:
//   - `planner`: the dispatch strategy resolved at announcement.
func (m *Method) setPlanner(planner Planner) { m.planner = planner }

// ReceiverType returns the reflect.Type of the method's receiver.
//
// Returns:
//   - `reflect.Type`: the receiver's type, pointer or value as declared.
func (m *Method) ReceiverType() reflect.Type { return m.do.Type.In(0) }

// compensatorType returns the receipt/compensator type the compensation companion's last parameter declares.
//
// Resume reads this to instantiate the concrete receipt before [Receipt.RestoreEncoded] — the companion is the same one
// compensation resolves at unwind, so no receipt registry is needed.
//
// Returns:
//   - `reflect.Type`: the companion's last parameter type.
//   - `bool`: false when the method has no compensation companion.
func (m *Method) compensatorType() (reflect.Type, bool) {

	if m.undo == nil {
		return nil, false
	}

	funcType := m.undo.Func.Type()
	if funcType.NumIn() == 0 {
		return nil, false
	}

	return funcType.In(funcType.NumIn() - 1), true
}

// ResultType returns the reflect.Type of the method's first non-error result, or nil.
//
// Returns:
//   - `reflect.Type`: the first non-error result's type, or nil when the method returns nothing or only an error.
func (m *Method) ResultType() reflect.Type {

	t := m.do.Type

	if t.NumOut() == 0 {
		return nil
	}

	first := t.Out(0)

	if t.NumOut() == 1 && first.Implements(errorType) {
		return nil
	}

	return first
}

// Undo calls the compensation companion on the receiver with the given activation and compensator.
//
// The call goes through the registration-baked undo adapter (step 43): the companion's mandated activation-first
// shape (step 27's floor) was validated and closed over at [NewMethod], so this is a plain call — no per-call
// reflection decisions.
//
// Parameters:
//   - `activation`: the per-dispatch record forwarded to the companion.
//   - `receiver`: the provider value the companion is called on.
//   - `compensator`: the compensator the forward method returned, reversed by the companion.
//
// Returns:
//   - `error`: the companion's error, or non-nil when the method has no compensation companion.
func (m *Method) Undo(activation *ActivationRecord, receiver any, compensator Compensator) error {

	if m.undoInvoke == nil {
		return fmt.Errorf("method %s has no compensation companion", m.do.Name)
	}

	return m.undoInvoke(receiver, activation, compensator)
}

// endregion

// region Behaviors

// Invoke coerces slot values into Go arguments via [Convert] and dispatches to the wrapped method.
//
// Reads the resolved slot values from `activation.Slots` (stamped by the executor before the dispatch). Each
// parameter's value is looked up by name and converted to the parameter's declared Go type.
//
// Parameters:
//   - `activation`: the per-dispatch record carrying resolved slot values, runtime environment, and unit identity.
//   - `receiver`: the provider or resource value the wrapped method is called on.
//
// Returns:
//   - `Result`: the method's unwrapped return value, or nil for actions.
//   - `Compensator`: the committed [Receipt] or spliced [*RecoveryStack], or nil when there is no compensator.
//   - `error`: non-nil if slot conversion, dispatch, or receipt commit failed.
func (m *Method) Invoke(activation *ActivationRecord, receiver any) (Result, Compensator, error) {

	params := m.Parameters()
	goArgs := make([]any, 0, len(params)+1)

	if m.firstParamIsActivation {
		goArgs = append(goArgs, activation)
	}

	for _, p := range params {

		value := activation.Slots[p.Name]

		// A planned invocation that omitted a defaulted optional parameter carries the parsed-but-unresolved
		// [DeferredDefault] in its slot (the planner stuffs Parameter.Default verbatim). Dispatch is where the
		// live runtime environment and the filled sibling slots finally exist, so the deferred form resolves
		// here — the same contract the starlark bridge's direct-invocation path implements.
		if deferred, ok := value.(DeferredDefault); ok {
			resolved, err := deferred.Resolve(activation.RuntimeEnvironment, activation.Slots, p.Type)
			if err != nil {
				return nil, nil, fmt.Errorf("param %s: resolve deferred default: %w", p.Name, err)
			}
			value = resolved
		}

		val, err := convertSlotValue(activation, value, p.Type)
		if err != nil {
			return nil, nil, fmt.Errorf("param %s: %w", p.Name, err)
		}

		goArgs = append(goArgs, val)
	}

	// The consumed-Gone guard (§3's consumption table, ruled 2026-08-20/22), at the ruled seam: after
	// conversion — the earliest point the complete consumed set exists, promises resolved and strings
	// interned — and before the forward call. A consumer of an entry the catalog knows is gone SEES the
	// state; it does not rediscover the loss through its own I/O.
	if guardErr := guardConsumedGone(activation, goArgs); guardErr != nil {
		return nil, nil, guardErr
	}

	result, compensator, dispatchErr := m.Do(receiver, goArgs)

	// Unwrap the reflected return once. m.Do hands back a reflect.Value; the receipt's stored result — and every
	// promise consumer that later reads it via [RecoveryStack.ResultByUnitID] — must see the underlying Go value, not
	// the reflect.Value wrapper. (A consumer slot binding `source=<upstream>` converts that stored value to its
	// parameter type, which fails on a raw reflect.Value.)

	unwrappedResult := resultOrNil(result)
	compensatorValue := compensatorOrNil(compensator)

	// A dispatch error does NOT discard the compensator. A compensable forward call returns its accumulated recovery
	// state (a *Receipt or a *RecoveryStack) as the compensator precisely so a failure can be rolled
	// back; the compensator is committed as usual and returned ALONGSIDE dispatchErr, so the caller's audit receipt
	// carries it ([GraphExecutor.pushAuditReceipt]) and [RecoveryStack.Unwind] runs its Compensate companion. Only an
	// inflate/build error — a defect committing the compensator itself — short-circuits to a nil compensator.

	switch v := compensatorValue.(type) {
	case nil:

		return unwrappedResult, nil, dispatchErr

	case Receipt:

		if commitErr := v.Commit(activation, unwrappedResult, compensatorValue, dispatchErr); commitErr != nil {
			return nil, nil, fmt.Errorf("inflate %s receipt: %w", m.actionName, commitErr)
		}

		return unwrappedResult, v, dispatchErr

	case *RecoveryStack:

		return unwrappedResult, v, dispatchErr

	default:

		// The compensator shape is restricted to a concrete *Receipt or a *RecoveryStack — isLegalCompensator
		// enforces it at NewMethod — so anything else reaching here is a defect, not a runtime input.
		return nil, nil, fmt.Errorf("compensable method %s: compensator %T is not a *Receipt or *RecoveryStack",
			m.actionName, compensatorValue)
	}
}

// Do dispatches a method call directly with Go arguments, returning reflected values.
//
// Parameters:
//   - `receiver`: the provider or resource value the method is called on; auto-addressed when passed by value.
//   - `args`: the Go arguments in declaration order, excluding the receiver.
//
// Returns:
//   - `reflect.Value`: the method's first result, or the zero Value for actions.
//   - `reflect.Value`: the method's compensator (compensable third return), or the zero Value.
//   - `error`: non-nil if the argument count is wrong or the method returned a non-nil error.
func (m *Method) Do(receiver any, args []any) (result, compensator reflect.Value, err error) {

	v := reflect.ValueOf(receiver)

	if v.Kind() != reflect.Pointer {
		ptr := reflect.New(v.Type())
		ptr.Elem().Set(v)
		v = ptr
	}

	numIn := m.do.Type.NumIn()

	if len(args)+1 != numIn {
		return reflect.Value{}, reflect.Value{}, fmt.Errorf("method %s: expected %d arguments (including receiver), got %d",
			m.do.Name,
			numIn,
			len(args)+1)
	}

	reflectArgs := make([]reflect.Value, len(args)+1)
	reflectArgs[0] = v

	for i, arg := range args {
		if arg == nil {
			reflectArgs[i+1] = reflect.Zero(m.do.Type.In(i + 1))
		} else {
			reflectArgs[i+1] = reflect.ValueOf(arg)
		}
	}

	// The registration-baked forward adapter (step 43): the variadic decision was made once at NewMethod.
	results := m.doInvoke(reflectArgs)

	switch m.kind {
	case MethodAction:
		return reflect.Value{}, reflect.Value{}, nil
	case MethodFallibleAction:
		return reflect.Value{}, reflect.Value{}, errorFromValue(results[0])
	case MethodFunction:
		return results[0], reflect.Value{}, nil
	case MethodFallibleFunction:
		return results[0], reflect.Value{}, errorFromValue(results[1])
	case MethodCompensableFunction:
		return results[0], results[1], errorFromValue(results[2])
	}

	assert.Unreachable("Method.Invoke: exhaustive switch on m.kind")
	return reflect.Value{}, reflect.Value{}, nil
}

// String returns the full Go method signature in human-readable form.
//
// Returns:
//   - `string`: the full Go method signature in human-readable form.
func (m *Method) String() string {

	receiverType := m.ReceiverType()

	if receiverType.Kind() == reflect.Pointer {
		receiverType = receiverType.Elem()
	}

	var b strings.Builder

	b.WriteString("func (")
	b.WriteString(receiverType.Name())
	b.WriteString(") ")
	b.WriteString(m.do.Name)
	b.WriteString("(")

	params := make([]string, len(m.parameters))

	for i, p := range m.parameters {
		params[i] = p.Name + " " + p.Type.String()
	}

	b.WriteString(strings.Join(params, ", "))
	b.WriteString(")")

	numOut := m.do.Type.NumOut()

	if numOut > 0 {

		b.WriteString(" ")

		if numOut > 1 {
			b.WriteString("(")
		}

		results := make([]string, numOut)

		for i := range results {
			results[i] = m.do.Type.Out(i).String()
		}

		b.WriteString(strings.Join(results, ", "))

		if numOut > 1 {
			b.WriteString(")")
		}
	}

	return b.String()
}

// endregion

// endregion

// region SUPPORTING TYPES

// MethodKind identifies the signature and capabilities of a method.
type MethodKind int

const (
	// MethodAction produces no result and cannot fail. Return: ().
	MethodAction MethodKind = iota

	// MethodFallibleAction produces no result but may fail. Return: (error).
	MethodFallibleAction

	// MethodFunction produces a result and cannot fail. Return: (T).
	MethodFunction

	// MethodFallibleFunction produces a result but may fail. Return: (T, error).
	MethodFallibleFunction

	// MethodCompensableFunction produces a result and compensator or an error. Return: (T, U, error).
	MethodCompensableFunction
)

// MethodMetadata is the codegen-emitted record describing one method on a registered provider.
//
// Carries source-level information that Go reflection can't see: the starlark parameter spelling, any surface modifiers
// (e.g. eager property projection via [ModifierProperty]), and, optionally, the planner type that materializes the
// method's calls into an [ExecutableUnit]. Absent Planner means the method uses [ActionPlanner] — the default vanilla
// leaf-node dispatcher.
type MethodMetadata struct {
	ParameterNames []string        // starlark parameter name tokens, ordered to match the Go method's parameter slots
	Modifiers      MethodModifiers // surface modifiers (e.g. ModifierProperty); ModifierNone is the default
	Planner        reflect.Type    // optional; nil means default ActionPlanner
}

// MethodModifiers is a bit set of per-method surface modifiers.
//
// It is orthogonal to [MethodKind]: where MethodKind classifies a method's return signature (action vs. function), a
// modifier records how the method is projected onto a starlark surface. The set is codegen-emitted onto
// [MethodMetadata] and threaded onto the constructed [Method]; the zero value [ModifierNone] is the default callable
// projection.
type MethodModifiers uint

const (

	// ModifierProperty marks a zero-arg getter ([MethodFunction] or [MethodFallibleFunction]) for property projection.
	//
	// A starlark attribute access calls the method and yields its result instead of returning the builtin. The codegen
	// sets it from a `+devlore:property` directive; it is valid only on zero-arg, value-returning methods (an action
	// has no value to project). Subsequent flags double from here (2, 4, 8, …).
	ModifierProperty MethodModifiers = 1 << 0
)

// endregion

// region HELPER FUNCTIONS

// convertSlotValue converts one resolved slot value toward its parameter type, routing resource-typed
// parameters through graph dispatch's identity resolution first.
//
// At graph dispatch a string is a key, never a constructor (4-resource-management.md §5.6): when the
// activation is a graph dispatch (`activation.Graph` non-nil — the per-dispatch frame carries dispatch
// kind) and the parameter is resource-typed, the slot value resolves through the run catalog via
// [resolveDispatchResource] and a miss is the catalog's refusal, never fresh construction. Every other
// dispatch — immediate mode's nil-Graph activations — and every non-resource parameter follows
// [Convert]'s cascade unchanged.
//
// Parameters:
//   - `activation`: the per-dispatch record carrying dispatch kind and the runtime environment.
//   - `value`: the resolved slot value.
//   - `target`: the parameter's declared Go type.
//
// Returns:
//   - `any`: the converted (or catalog-resolved) value.
//   - `error`: non-nil when resolution refuses or no conversion path succeeds.
func convertSlotValue(activation *ActivationRecord, value any, target reflect.Type) (any, error) {

	if v, applied, err := resolveDispatchResource(activation, value, target); applied {
		return v, err
	}

	return Convert(activation.RuntimeEnvironment, value, target)
}

// guardConsumedGone applies the consumed-Gone guard to the converted arguments (§3's consumption table).
//
// Each catalog-resident resource among the arguments is checked against the run catalog's state; a [Gone]
// entry routes through the caller's [MissingResourcePolicy], found among the same arguments by type (the
// parameter's type is the declaration) with the fail-safe [MissingResourcePolicyStop] default: Stop fails
// the dispatch on the narrated verdict — "destroyed by <unit> before <consumer> could run" when the
// destroyer stamp is present — and Ignore proceeds, the provider seeing and handling the absence. A
// warning is produced on every detection, under both policies.
//
// Parameters:
//   - `activation`: the dispatch activation; carries the run catalog, the caller's id, and the narrator.
//   - `goArgs`: the converted Go arguments.
//
// Returns:
//   - `error`: the narrated verdict under [MissingResourcePolicyStop]; nil otherwise.
func guardConsumedGone(activation *ActivationRecord, goArgs []any) error {

	environment := activation.RuntimeEnvironment
	if environment == nil || environment.ResourceCatalog == nil {
		return nil
	}

	policy := MissingResourcePolicyStop
	for _, arg := range goArgs {
		if declared, ok := arg.(MissingResourcePolicy); ok {
			policy = declared
			break
		}
	}

	for _, arg := range goArgs {

		resource, ok := arg.(Resource)
		if !ok || resource.resourceBase().id == "" {
			continue
		}

		if environment.ResourceCatalog.State(resource.ID()) != Gone {
			continue
		}

		verdict := verdictForGone(environment.ResourceCatalog, activation.CallerID, resource)

		if environment.Status != nil {
			environment.Status.Warn(fmt.Sprintf("%s (policy %s)", verdict, policy))
		}

		if policy == MissingResourcePolicyStop {
			return errors.New(verdict)
		}
		// Ignore: the call proceeds; the provider handles the absence.
	}

	return nil
}

// resolveDispatchResource is graph dispatch's identity resolution — the §5.6 seam.
//
// Applies only when `activation` is a graph dispatch with a run catalog and `target` implements
// [Resource]. A [Resource] value resolves by its URI — the dispatched object must BE the run clone's
// entry (re-based, state-carrying, the row pre-flight verified), never the captured planning object. A
// string is the rehydrated identity a reload leaves in the slot and resolves as the key it is. Any other
// value cannot name a resource at dispatch. A miss is the catalog's verdict: the catalog is complete by
// construction (§5.1), so nothing constructs here — construction from strings survives only in load-time
// rehydration and immediate mode.
//
// Parameters:
//   - `activation`: the per-dispatch record; its Graph and the environment's catalog gate the step.
//   - `value`: the resolved slot value.
//   - `target`: the parameter's declared Go type.
//
// Returns:
//   - `any`: the run clone's canonical entry on a hit.
//   - `bool`: true when this step applied (regardless of error).
//   - `error`: the refusal on a miss, a non-identity value, or an entry that cannot fill `target`.
func resolveDispatchResource(activation *ActivationRecord, value any, target reflect.Type) (resolved any, applied bool, err error) {

	if activation == nil || activation.Graph == nil {
		return nil, false, nil
	}
	environment := activation.RuntimeEnvironment
	if environment == nil || environment.ResourceCatalog == nil {
		return nil, false, nil
	}
	if !target.Implements(resourceInterfaceType) {
		return nil, false, nil
	}

	var key string
	switch v := value.(type) {
	case Resource:
		key = v.URI()
	case string:
		key = v
	case nil:
		return nil, false, nil
	default:
		return nil, true, fmt.Errorf(
			"graph dispatch: a %T cannot name a resource — a string is a key, never a constructor (4-resource-management.md §5.6)",
			value)
	}

	catalog := environment.ResourceCatalog

	id := catalog.Current(key)
	if id == "" {
		return nil, true, fmt.Errorf(
			"graph dispatch: %q is not in the run catalog — a string is a key, never a constructor; nothing constructs at dispatch (4-resource-management.md §5.6)",
			key)
	}

	canonical, ok := catalog.Lookup(id)
	if !ok {
		return nil, true, fmt.Errorf("graph dispatch: run catalog names %q as %s but holds no entry", key, id)
	}
	if !reflect.TypeOf(canonical).AssignableTo(target) {
		return nil, true, fmt.Errorf(
			"graph dispatch: run catalog entry %q is %T, which cannot fill a %s slot", key, canonical, target)
	}

	return canonical, true, nil
}

// verdictForGone renders the catalog's verdict for a consumed-Gone entry, naming both units when the
// destroyer stamp is present.
//
// Parameters:
//   - `catalog`: the run catalog holding the destroyer stamp.
//   - `consumerID`: the consuming unit's id.
//   - `resource`: the gone resource.
//
// Returns:
//   - `string`: the verdict text.
func verdictForGone(catalog *ResourceCatalog, consumerID string, resource Resource) string {

	if destroyer := catalog.DestroyerOf(resource.ID()); destroyer != "" {
		return fmt.Sprintf("%s consumes %s, destroyed by %s before it could run",
			consumerID, resource.URI(), destroyer)
	}

	return fmt.Sprintf("%s consumes %s, which is gone", consumerID, resource.URI())
}

// errorFromValue extracts an error from a reflect.Value, returning nil when the value holds a nil interface.
//
// Parameters:
//   - `v`: the reflected return value holding an error interface, possibly nil.
//
// Returns:
//   - `error`: the unwrapped error, or nil when `v` holds a nil interface.
func errorFromValue(v reflect.Value) error {
	if v.IsNil() {
		return nil
	}
	return assert.Type[error]("method error result", v.Interface())
}

// errorInvalidResultParameters returns a standard error for an unsupported return signature.
//
// Parameters:
//   - `do`: the reflected method whose return signature was rejected.
//
// Returns:
//   - `error`: a formatted error naming the method and its unsupported signature.
func errorInvalidResultParameters(do *reflect.Method) error {
	return fmt.Errorf("expected void, pure, fallible, or compensable result parameters for method %s, not: %s",
		do.Name,
		methodSignature(do))
}

// isLegalCompensator returns true if t is a valid return type for a compensator.
//
// The compensator shape is restricted to a concrete pointer that implements [Receipt] (a `*Receipt`) or a
// [*RecoveryStack]. Slices and bare interfaces are rejected: a batch producer returns one [*RecoveryStack] of per-item
// receipts, so [Method.Invoke] never has to splice a slice into a stack.
//
// Parameters:
//   - `t`: the candidate compensator type to validate.
//
// Returns:
//   - `bool`: true when `t` is a concrete `*Receipt` or a [*RecoveryStack].
func isLegalCompensator(t reflect.Type) bool {

	if t.Kind() == reflect.Pointer && t.Implements(receiptType) {
		return true
	}

	if t == recoveryStackType {
		return true
	}

	return false
}

// methodSignature renders a reflect.Method as a human-readable Go function signature.
//
// Parameters:
//   - `m`: the reflected method to render.
//
// Returns:
//   - `string`: the method's Go signature in human-readable form.
func methodSignature(m *reflect.Method) string {

	mt := m.Type
	var b strings.Builder

	receiver := mt.In(0)

	if receiver.Kind() == reflect.Pointer {
		receiver = receiver.Elem()
	}

	b.WriteString("func (")
	b.WriteString(receiver.Name())
	b.WriteString(") ")
	b.WriteString(m.Name)
	b.WriteString("(")

	params := make([]string, mt.NumIn()-1)

	for i := range params {
		params[i] = mt.In(i + 1).String()
	}

	b.WriteString(strings.Join(params, ", "))
	b.WriteString(")")

	if mt.NumOut() > 0 {

		b.WriteString(" ")

		if mt.NumOut() > 1 {
			b.WriteString("(")
		}

		results := make([]string, mt.NumOut())

		for i := range results {
			results[i] = mt.Out(i).String()
		}

		b.WriteString(strings.Join(results, ", "))

		if mt.NumOut() > 1 {
			b.WriteString(")")
		}
	}

	return b.String()
}

// endregion

// validateParameterPositions checks the variadic / kwargs position rules: each flag implies the
// parameter sits in the last (or last-before-kwargs) slot. The token grammar already enforces that
// variadic / kwargs cannot also carry ?/=; only cross-parameter position is validated here.
//
// Parameters:
//   - `do`: the forward method, for error context.
//   - `parameters`: the declared parameters.
//
// Returns:
//   - `error`: the first position violation, or nil.
func validateParameterPositions(do *reflect.Method, parameters []Parameter) error {

	for i, p := range parameters {

		if p.Kwargs && i != len(parameters)-1 {
			return fmt.Errorf("keyword catch-all %q must be the last parameter of method %s",
				p.Name,
				do.Name)
		}

		if p.Variadic && i != len(parameters)-1 && (i != len(parameters)-2 || !parameters[i+1].Kwargs) {
			return fmt.Errorf("variadic parameter %q must be the last or second-to-last (before **kwargs) parameter of method %s",
				p.Name,
				do.Name)
		}
	}

	return nil
}

// classifyMethodKind classifies the forward method by its return signature.
//
// The compensator of a three-return (compensable) method must be a concrete *Receipt or a
// *RecoveryStack if it's to join a saga — no slices, no bare interface. A batch producer returns one
// *RecoveryStack of per-item receipts. That shape is enforced only for providers where compensation
// is expected (`enforceCompanions`).
//
// Parameters:
//   - `do`: the forward method.
//   - `enforceCompanions`: whether provider companion rules apply.
//
// Returns:
//   - `MethodKind`: the classification.
//   - `error`: non-nil for an unsupported return signature.
func classifyMethodKind(do *reflect.Method, enforceCompanions bool) (MethodKind, error) {

	methodType := do.Type

	switch methodType.NumOut() {
	default:
		return 0, errorInvalidResultParameters(do)

	case 0:
		return MethodAction, nil

	case 1:
		if methodType.Out(0).Implements(errorType) {
			return MethodFallibleAction, nil
		}
		return MethodFunction, nil

	case 2:
		if !methodType.Out(1).Implements(errorType) {
			return 0, errorInvalidResultParameters(do)
		}
		return MethodFallibleFunction, nil

	case 3:
		if !methodType.Out(2).Implements(errorType) {
			return 0, errorInvalidResultParameters(do)
		}
		if !isLegalCompensator(methodType.Out(1)) && enforceCompanions {
			return 0, fmt.Errorf("compensable method %s: compensator type %s must be a *Receipt or a *RecoveryStack",
				do.Name,
				methodType.Out(1))
		}
		return MethodCompensableFunction, nil
	}
}

// validatePlanCompanion cross-validates a Plan<Name> companion against the forward method: same
// parameters, exactly (result, error) returns with the forward's result type.
//
// Parameters:
//   - `do`: the forward method.
//   - `plan`: the plan companion; nil validates trivially.
//   - `kind`: the forward's classification.
//
// Returns:
//   - `error`: the first mismatch, or nil.
func validatePlanCompanion(do, plan *reflect.Method, kind MethodKind) error {

	if plan == nil {
		return nil
	}

	methodType := do.Type

	if kind == MethodAction || kind == MethodFallibleAction {
		return fmt.Errorf("plan companion %s provided for method %s which produces no result",
			plan.Name,
			do.Name)
	}

	planType := plan.Type

	if planType.NumIn() != methodType.NumIn() {
		return fmt.Errorf("plan companion %s for method %s must accept %d parameters, got %d",
			plan.Name,
			do.Name,
			methodType.NumIn()-1,
			planType.NumIn()-1)
	}

	for i := 1; i < methodType.NumIn(); i++ {
		if planType.In(i) != methodType.In(i) {
			return fmt.Errorf("plan companion %s for method %s: parameter %d type mismatch: got %s, want %s",
				plan.Name,
				do.Name,
				i-1,
				planType.In(i),
				methodType.In(i))
		}
	}

	if planType.NumOut() != 2 {
		return fmt.Errorf("plan companion %s for method %s must return exactly 2 values (result, error), got %d",
			plan.Name,
			do.Name,
			planType.NumOut())
	}

	if planType.Out(0) != methodType.Out(0) {
		return fmt.Errorf("plan companion %s for method %s: result type mismatch: got %s, want %s",
			plan.Name,
			do.Name,
			planType.Out(0),
			methodType.Out(0))
	}

	if !planType.Out(1).Implements(errorType) {
		return fmt.Errorf("plan companion %s for method %s: second return value must implement error",
			plan.Name,
			do.Name)
	}

	return nil
}

// buildUndoInvoke validates a Compensate<Name> companion and builds its invoker.
//
// The required floor (step 27): a compensation companion is dispatched by the recovery machinery with
// an activation in hand, and its shape is mandated as (receiver, *ActivationRecord, compensator)
// returning exactly one error.
//
// Parameters:
//   - `do`: the forward method, for error context.
//   - `undo`: the compensation companion; nil builds a nil invoker.
//   - `kind`: the forward's classification; a companion demands a compensable forward.
//
// Returns:
//   - `func(any, *ActivationRecord, any) error`: the invoker, or nil without a companion.
//   - `error`: the first signature violation, or nil.
func buildUndoInvoke(
	do, undo *reflect.Method, kind MethodKind,
) (func(receiver any, activation *ActivationRecord, compensator any) error, error) {

	if undo == nil {
		return nil, nil
	}

	if kind != MethodCompensableFunction {
		return nil, fmt.Errorf("compensation companion %s provided, but method %s is %v, not compensable",
			undo.Name,
			do.Name,
			kind)
	}

	undoType := undo.Type

	if undoType.NumIn() != 3 {
		return nil, fmt.Errorf("compensation companion %s for method %s has an invalid signature: expected (*ActivationRecord, compensator), got %d parameter(s) (step 27: the required floor)",
			undo.Name,
			do.Name,
			undoType.NumIn()-1)
	}

	if undoType.In(1) != activationRecordType {
		return nil, fmt.Errorf("compensation companion %s for method %s has an invalid signature: first parameter must be *ActivationRecord, got %s",
			undo.Name,
			do.Name,
			undoType.In(1))
	}

	if undoType.NumOut() != 1 || !undoType.Out(0).Implements(errorType) {
		return nil, fmt.Errorf("compensation companion %s for method %s has an invalid signature: must return exactly one parameter (error), got %d",
			undo.Name,
			do.Name,
			undoType.NumOut())
	}

	undoFn := undo.Func

	return func(receiver any, activation *ActivationRecord, compensator any) error {
		results := undoFn.Call([]reflect.Value{
			reflect.ValueOf(receiver),
			reflect.ValueOf(activation),
			reflect.ValueOf(compensator),
		})
		return errorFromValue(results[0])
	}, nil
}
