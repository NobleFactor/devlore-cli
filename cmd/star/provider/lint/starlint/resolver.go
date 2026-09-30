// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package starlint resolves `plan.*` calls in Starlark against the action surface devlore actually has.
//
// It exists because nothing read the contents of a `.star` file: `devlore-registry/packages/docker` shipped
// 40 scripts calling seven APIs that do not exist, and every gate passed them (devlore-cli#721).
//
// # Where the truth comes from
//
// The resolver asks [op.ReceiverRegistry], not the generated source. The registry is populated by the `init`
// functions of the generated `gen` packages, so it holds exactly what a running `lore` binary offers.
//
// Reading the generated text instead would have lost two things that decide real calls. [op.Parameter] has
// a `Kwargs` flag -- a method declaring `**kwargs` accepts any keyword -- and [op.Method] carries claims,
// which is how a hermetic runtime decides what it admits. Neither survives as a list of names.
//
// # What a phase script may call
//
// `plan` is the plan provider, a qualified provider on [op.SurfaceScript]. Three things reach a script
// through it, and they are filtered differently:
//
//  1. `plan.<method>(...)` -- the plan provider's own methods. These are script-surface globals, so the
//     hermetic filter applies: a planning runtime admits only a method claiming [op.ClaimDeterministic],
//     because a plan must produce the same graph on any machine.
//  2. `plan.<namespace>.<method>(...)` -- the graph namespace. Workflow-surface providers reached by name,
//     and NOT hermetic-filtered: a graph accepts anything with an action signature, so `file` contributes
//     its mutators here even though a script global gets only its path algebra.
//  3. `plan.<method>(...)` -- a promoted provider's methods, surfacing at the namespace root rather than
//     qualified by their own name ([op.PlacementPromoted]). `ui` is the case: `note()` in a script,
//     `plan.note()` in a workflow.
//
// On top of that, lore withholds [lorepackage.LifecycleVerbs] -- scripts contribute invocations, and lore
// alone assembles, runs and persists. Both readers take that list from one slice, so a verb cannot be
// denied at run time and accepted by the linter.
//
// The two filters are not redundant and the resolver reports them apart, because the reason is the useful
// part of the finding. `plan.clear` claims determinism, so hermetic admits it and the denial is what stops
// it. `plan.run` claims nothing, so it is gone before the denial is consulted.
package starlint

import (
	"sort"
	"strings"

	"github.com/NobleFactor/devlore-cli/cmd/internal/lorepackage"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// planReceiver is the name the plan provider is registered under, and the global a phase script writes.
const planReceiver = "plan"

// region Types

// Resolver answers whether a `plan.*` call names something a phase script may invoke.
//
// Build one with [NewPhaseScriptResolver]. A Resolver is read-only once built and safe to share.
type Resolver struct {

	// namespaces maps a graph-namespace provider name to its methods, keyed by Starlark attribute.
	namespaces map[string]map[string]*op.Method

	// bare maps an attribute reached directly on `plan` to its method, before either filter is applied.
	bare map[string]*op.Method

	// hermeticFiltered is the subset of `bare` a planning runtime withholds for making no deterministic
	// claim. Membership here is not an error in itself -- it is the reason one is reported.
	hermeticFiltered map[string]bool

	// denied is the set of `plan` attributes lore withholds from phase scripts.
	denied map[string]bool

	// phaseScript is true when this resolver models lore's hermetic planning runtime. False models an
	// ambient one -- a star extension command or a devlore-test script -- where neither filter applies.
	phaseScript bool
}

// Resolution is the outcome of resolving one call. Its [Resolution.String] is the rule a finding carries.
type Resolution int

const (
	// Resolved means the call names a method a phase script may invoke.
	Resolved Resolution = iota

	// UnknownNamespace means `plan.<namespace>` is not a provider in the graph namespace.
	UnknownNamespace

	// UnknownMethod means the namespace has no such method, or nothing offers it directly on `plan`.
	UnknownMethod

	// Denied means the method exists and is withheld from phase scripts by [lorepackage.LifecycleVerbs].
	Denied

	// NotDeterministic means the method exists but a planning runtime withholds it, because it makes no
	// [op.ClaimDeterministic] and a plan must produce the same graph on any machine.
	NotDeterministic
)

// String returns the resolution's name, which is the rule name a finding carries.
//
// Returns:
//   - `string`: a stable kebab-case identifier.
func (r Resolution) String() string {

	switch r {
	case Resolved:
		return "resolved"
	case UnknownNamespace:
		return "unknown-namespace"
	case UnknownMethod:
		return "unknown-method"
	case Denied:
		return "denied"
	case NotDeterministic:
		return "not-deterministic"
	default:
		return "unknown"
	}
}

// endregion

// region Constructors

// NewPhaseScriptResolver builds a Resolver for a package phase script, modeling the runtime lore builds in
// `prepareScriptEnv`: hermetic, with [lorepackage.LifecycleVerbs] denied on `plan`.
//
// Returns:
//   - `*Resolver`: ready to resolve.
func NewPhaseScriptResolver() *Resolver { return newResolver(true) }

// NewAmbientResolver builds a Resolver for a script running outside lore's planning runtime -- a `star`
// extension command, or a devlore-test data script.
//
// Neither restriction applies there. `star` is a scripting tool where effects are the point, so nothing is
// hermetic-filtered, and lore's lifecycle denial is lore's policy for phase scripts, not a property of the
// action surface.
//
// This constructor exists because assuming otherwise was wrong in a way only running the checker showed:
// applied to the repository's own corpus, a phase-script resolver reported `plan.save_definition` twelve
// times across `cmd/devlore-test/devloretest/data`, where it is entirely legal. Those were false positives
// against the requirement that there be none.
//
// Returns:
//   - `*Resolver`: ready to resolve.
func NewAmbientResolver() *Resolver { return newResolver(false) }

// newResolver builds a Resolver for one of the two runtimes.
//
// Parameters:
//   - `phaseScript`: true to apply the hermetic filter and lore's lifecycle denial.
//
// Returns:
//   - `*Resolver`: ready to resolve.
func newResolver(phaseScript bool) *Resolver {

	resolver := &Resolver{
		namespaces:       map[string]map[string]*op.Method{},
		bare:             map[string]*op.Method{},
		hermeticFiltered: map[string]bool{},
		denied:           map[string]bool{},
		phaseScript:      phaseScript,
	}

	if phaseScript {
		for _, verb := range lorepackage.LifecycleVerbs {
			resolver.denied[verb] = true
		}
	}

	registry := op.ReceiverRegistry()

	// 1. The graph namespace: workflow-surface providers by name, unfiltered.
	for _, provider := range registry.Workflows() {

		methods := methodsOf(provider)
		if len(methods) == 0 {
			continue
		}

		resolver.namespaces[provider.Name()] = methods
	}

	// 2. The plan provider's own methods, which are script-surface globals and so hermetic-filtered.
	for _, provider := range registry.Scripts() {

		if provider.Name() != planReceiver {
			continue
		}

		for attribute, method := range methodsOf(provider) {

			resolver.bare[attribute] = method

			if phaseScript && method.Claims()&op.ClaimDeterministic == 0 {
				resolver.hermeticFiltered[attribute] = true
			}
		}
	}

	// 3. Promoted providers, which surface at the namespace root. PromotedProviders is deliberately not
	//    filtered by surface -- placement applies to every surface a provider reaches -- so the workflow
	//    filter is applied here. These sit in the graph namespace's root and are not hermetic-filtered.
	for _, provider := range registry.PromotedProviders() {

		if provider.Flags().Surfaces()&op.SurfaceWorkflow == 0 {
			continue
		}

		for attribute, method := range methodsOf(provider) {

			if _, own := resolver.bare[attribute]; own {
				continue
			}

			resolver.bare[attribute] = method
		}
	}

	return resolver
}

// endregion

// region Behaviors

// Resolve resolves `plan.<namespace>.<method>` when `namespace` is non-empty, and `plan.<method>` otherwise.
//
// Parameters:
//   - `namespace`: the qualifying provider, or "" for a call directly on `plan`.
//   - `method`: the Starlark attribute being called, in snake case.
//
// Returns:
//   - `Resolution`: what the call is.
//   - `*op.Method`: the method whenever one was found, including when it is withheld; nil otherwise.
func (r *Resolver) Resolve(namespace, method string) (Resolution, *op.Method) {

	if namespace == "" {

		found, ok := r.bare[method]
		if !ok {
			return UnknownMethod, nil
		}

		// Denial before hermeticity: lore's policy is the narrower statement, and it is the one a package
		// author can act on. A verb withheld by both is reported as denied.
		if r.denied[method] {
			return Denied, found
		}

		if r.hermeticFiltered[method] {
			return NotDeterministic, found
		}

		return Resolved, found
	}

	methods, ok := r.namespaces[namespace]
	if !ok {
		return UnknownNamespace, nil
	}

	found, ok := methods[method]
	if !ok {
		return UnknownMethod, nil
	}

	// A denial names an attribute of `plan` itself, and the graph namespace is not hermetic-filtered, so a
	// qualified call that resolves is admissible.
	return Resolved, found
}

// AcceptsKeyword reports whether a method accepts a keyword argument.
//
// A method declaring `**kwargs` accepts any keyword, so this answers true for every name. That is why the
// resolver holds [op.Method] rather than a list of parameter names: a list cannot express it.
//
// Parameters:
//   - `method`: a method returned by [Resolver.Resolve].
//   - `keyword`: the keyword used at the call site.
//
// Returns:
//   - `bool`: true when the keyword is acceptable.
func (r *Resolver) AcceptsKeyword(method *op.Method, keyword string) bool {

	if method == nil {
		return false
	}

	for _, parameter := range method.Parameters() {
		if parameter.Kwargs {
			return true
		}
	}

	_, ok := method.ParameterByName(keyword)

	return ok
}

// Namespaces returns every graph namespace this resolver accepts, sorted.
//
// Returns:
//   - `[]string`: the provider names valid as `plan.<namespace>`.
func (r *Resolver) Namespaces() []string { return sortedKeys(r.namespaces) }

// Bare returns every attribute reachable directly on `plan` before either filter, sorted.
//
// Returns:
//   - `[]string`: the attribute names.
func (r *Resolver) Bare() []string { return sortedKeys(r.bare) }

// Denied returns the attributes lore withholds from phase scripts, sorted.
//
// Returns:
//   - `[]string`: the denied names.
func (r *Resolver) Denied() []string { return sortedKeys(r.denied) }

// endregion

// region Private functions

// methodsOf indexes a provider's methods by the Starlark attribute each is reached by.
//
// [op.CamelToSnake] is the conversion starlarkbridge itself applies when it builds a receiver's method
// index, so these keys are the names a script actually writes.
//
// Parameters:
//   - `provider`: the provider to index.
//
// Returns:
//   - `map[string]*op.Method`: attribute name to method.
func methodsOf(provider op.ProviderReceiverType) map[string]*op.Method {

	attributes := map[string]*op.Method{}

	for method := range provider.Methods() {

		name := op.CamelToSnake(method.Name())
		if name == "" || strings.HasPrefix(name, "_") {
			continue
		}

		attributes[name] = method
	}

	return attributes
}

// sortedKeys returns a map's keys in order.
//
// Returns:
//   - `[]string`: the keys, sorted.
func sortedKeys[V any](m map[string]V) []string {

	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}

// endregion
