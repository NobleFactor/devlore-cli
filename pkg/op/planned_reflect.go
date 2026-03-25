// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"

	"go.starlark.net/starlark"
)

// PlanningReceiver wraps a provider's method signatures for planned-mode
// Starlark use. Each call creates a graph Node instead of executing the
// method directly.
type PlanningReceiver struct {
	receiver
	providerName string
	graph        *Graph
	project      string
	reg          *ActionRegistry
	methods      map[string]*plannedBridge
	attrList     []string
}

type plannedBridge struct {
	name   string
	bridge builtinFunc
}

// WrapProviderInPlanningReceiver wraps a provider for planned-mode use. Only methods with
// a corresponding registered action in reg are exposed.
func WrapProviderInPlanningReceiver(
	factory ReceiverFactory,
	graph *Graph,
	project string,
	reg *ActionRegistry,
	params MethodParams,
) *PlanningReceiver {

	receiverName := factory.ReceiverName()

	p := &PlanningReceiver{
		receiver:     newReceiver("plan." + receiverName),
		providerName: factory.ReceiverName(),
		graph:        graph,
		project:      project,
		reg:          reg,
		methods:      make(map[string]*plannedBridge),
	}

	// Enumerate exported methods on the pointer type (providers use pointer receivers).

	pt := reflect.PointerTo(factory.ProviderType())

	for i := range pt.NumMethod() {
		m := pt.Method(i)
		if !m.IsExported() || strings.HasPrefix(m.Name, "Compensate") {
			continue
		}

		snakeName := camelToSnake(m.Name)
		actionName := receiverName + "." + snakeName

		// Only expose methods that have a registered action.
		if _, ok := reg.Get(actionName); !ok {
			continue
		}

		paramNames, ok := params[m.Name]
		if !ok {
			continue
		}

		bridge := buildPlannedBridge(receiverName, snakeName, actionName, paramNames, m, graph, project, reg)
		p.methods[snakeName] = &plannedBridge{
			name:   snakeName,
			bridge: bridge,
		}
	}

	p.attrList = make([]string, 0, len(p.methods))
	for name := range p.methods {
		p.attrList = append(p.attrList, name)
	}
	sort.Strings(p.attrList)

	return p
}

// Override replaces a method's auto-generated planned bridge with a
// custom one.
func (p *PlanningReceiver) Override(name string, fn builtinFunc) {
	p.methods[name] = &plannedBridge{
		name:   name,
		bridge: fn,
	}
	if !slices.Contains(p.attrList, name) {
		p.attrList = append(p.attrList, name)
		sort.Strings(p.attrList)
	}
}

// Attr implements starlark.HasAttrs.
func (p *PlanningReceiver) Attr(name string) (starlark.Value, error) {
	if m, ok := p.methods[name]; ok {
		return starlark.NewBuiltin(p.receiver.name+"."+name, m.bridge), nil
	}
	return nil, NoSuchAttrError(p.receiver.name, name)
}

// AttrNames implements starlark.HasAttrs.
func (p *PlanningReceiver) AttrNames() []string {
	return p.attrList
}

// buildPlannedBridge creates a builtinFunc that creates a graph Node.
// The reflect.Method is used to inspect parameter types for catalog
// resolution: immediate values passed to Resource-typed parameters
// are resolved in the graph's catalog at plan time.
func buildPlannedBridge(
	providerName, snakeName, actionName string,
	paramNames []string,
	method reflect.Method,
	graph *Graph,
	project string,
	reg *ActionRegistry,
) builtinFunc {
	// Detect **kwargs param and build known-kwarg set for filtering.
	var kwargsName string
	knownKwargs := make(map[string]bool, len(paramNames))
	regularParams := make([]string, 0, len(paramNames))
	for _, name := range paramNames {
		if strings.HasPrefix(name, "**") {
			kwargsName = strings.TrimPrefix(name, "**")
		} else {
			regularParams = append(regularParams, name)
			clean := strings.TrimPrefix(name, "*")
			clean = strings.TrimSuffix(clean, "?")
			knownKwargs[clean] = true
		}
	}

	return func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		// 1. Unpack args as raw starlark.Value (slots store Starlark
		//    values for deferred execution — NOT Go types).
		//    Handle variadic params (*name) by stripping the prefix for
		//    UnpackArgs and collecting them separately.
		vals := make([]starlark.Value, len(regularParams))
		pairs := make([]any, 0, len(regularParams)*2)
		for i, name := range regularParams {
			pairs = append(pairs, strings.TrimPrefix(name, "*"), &vals[i])
		}

		// Split kwargs: known → UnpackArgs, unknown → **kwargs dict slots.
		var filteredKwargs []starlark.Tuple
		var extraKwargs []starlark.Tuple
		if kwargsName != "" {
			for _, kv := range kwargs {
				key, _ := starlark.AsString(kv[0])
				if knownKwargs[key] {
					filteredKwargs = append(filteredKwargs, kv)
				} else {
					extraKwargs = append(extraKwargs, kv)
				}
			}
		} else {
			filteredKwargs = kwargs
		}

		if err := starlark.UnpackArgs(snakeName, args, filteredKwargs, pairs...); err != nil {
			return nil, err
		}

		// 2. Create node.
		node := &Node{
			ID:      GenerateNodeID(providerName + "-" + snakeName),
			Action:  reg.MustGet(actionName),
			Project: project,
		}

		// 3. Fill slots from Starlark values and resolve Resource params.
		mt := method.Type
		for i, name := range regularParams {
			cleanName := strings.TrimSuffix(name, "?")
			cleanName = strings.TrimPrefix(cleanName, "*")
			sv := vals[i]
			if sv == nil {
				continue // Optional param not provided.
			}

			// Callable extraction: when a *starlark.Function is passed
			// to a func-typed parameter, extract it into a compiled
			// CallableResource and store as a slot immediate.
			if starFn, ok := sv.(*starlark.Function); ok && i+1 < mt.NumIn() && isFuncType(mt.In(i+1)) {
				funcType := providerName + "." + mt.In(i+1).Name()
				callable, err := extractCallable(starFn, funcType)
				if err != nil {
					return nil, fmt.Errorf("%s: param %s: extract callable: %w", snakeName, cleanName, err)
				}
				node.SetSlotImmediate(cleanName, callable)
				continue
			}

			if err := FillSlot(node, graph, cleanName, sv); err != nil {
				return nil, fmt.Errorf("%s: %w", cleanName, err)
			}

			// Plan-time type validation for immediate values.
			if i+1 < mt.NumIn() {
				if _, isOutput := sv.(*Promise); !isOutput {
					if _, isList := sv.(*starlark.List); !isList {
						if goVal := node.GetSlot(cleanName); goVal != nil {
							if err := validateSlotType(goVal, mt.In(i+1)); err != nil {
								return nil, fmt.Errorf("%s: param %s: %w", snakeName, cleanName, err)
							}
						}
					}
				}
			}

			// Resolve immediate Resource parameters in the catalog.
			// Skip promises — they're shadowed at execution time (step 5f).
			if i+1 < mt.NumIn() {
				resolveResourceParam(graph, sv, mt.In(i+1))
			}
		}

		// 4. Fill **kwargs as dict sub-slots.
		if kwargsName != "" {
			for _, kv := range extraKwargs {
				key, _ := starlark.AsString(kv[0])
				subSlot := fmt.Sprintf("%s.%s", kwargsName, key)
				if err := FillSlot(node, graph, subSlot, kv[1]); err != nil {
					return nil, fmt.Errorf("%s: kwarg %s: %w", snakeName, key, err)
				}
			}
		}

		// 5. Shadow output Resource for compensable methods.
		// For methods that return a Resource, shadow the last
		// Resource-typed param (the write target). This supersedes the
		// earlier Resolve, removing the destination from DiscoveryURIs
		// so pre-flight won't reject missing targets.
		if err := shadowOutputParam(graph, mt, vals, regularParams, node.ID); err != nil {
			return nil, err
		}

		// 6. Append to graph and return promise.
		graph.Nodes = append(graph.Nodes, node)
		return NewPromise(node, graph, ""), nil
	}
}

// shadowOutputParam shadows the output Resource parameter in the catalog at
// plan time. For compensable methods (3+ returns) that return a Resource type,
// the last Resource-typed parameter (conventionally the destination) is
// shadowed with the node's ID. This removes the destination from
// DiscoveryURIs so pre-flight won't reject files that don't exist yet.
func shadowOutputParam(graph *Graph, mt reflect.Type, vals []starlark.Value, paramNames []string, nodeID string) error {
	if graph.Catalog == nil {
		return nil
	}

	// Only compensable methods (numNonError >= 2) modify state.
	numNonError := mt.NumOut()
	if numNonError > 0 && mt.Out(mt.NumOut()-1).Implements(errorType) {
		numNonError--
	}
	if numNonError < 2 {
		return nil
	}

	// Check if the first non-error return is a Resource type.
	resultType := mt.Out(0)
	if resultType == noResultType {
		return nil
	}
	if !implementsResource(resultType) {
		return nil
	}

	// Find all Resource-typed parameter indices.
	var resourceParamIndices []int
	for i := range paramNames {
		paramIdx := i + 1 // skip receiver
		if paramIdx >= mt.NumIn() {
			break
		}
		if implementsResource(mt.In(paramIdx)) {
			resourceParamIndices = append(resourceParamIndices, i)
		}
	}

	if len(resourceParamIndices) == 0 {
		return nil
	}

	// Shadow the last Resource-typed parameter (destination convention).
	lastIdx := resourceParamIndices[len(resourceParamIndices)-1]
	sv := vals[lastIdx]
	if sv == nil {
		return nil
	}

	// Skip promises — they'll be shadowed at execution time.
	if _, ok := sv.(*Promise); ok {
		return nil
	}
	if _, ok := sv.(*starlark.List); ok {
		return nil
	}

	paramType := mt.In(lastIdx + 1)
	var goVal any
	if err := unmarshal(sv, &goVal); err != nil {
		return nil
	}

	r, ok := constructResource(paramType, goVal)
	if !ok {
		return nil
	}

	uri := r.URI()
	if uri != "" {
		if _, err := graph.Catalog.Shadow(r, nodeID); err != nil {
			return err
		}
	}
	return nil
}

// implementsResource returns true if t satisfies the Resource interface
// either directly or via pointer (for value types with pointer receivers).
func implementsResource(t reflect.Type) bool {
	if t.Implements(resourceType) {
		return true
	}
	return t.Kind() == reflect.Struct && reflect.PointerTo(t).Implements(resourceType)
}

// resolveResourceParam checks if a Starlark slot value targets a Resource-typed
// parameter. For immediate (non-promise) values, it constructs a plan-time
// Resource and calls catalog.Resolve to register the URI.
func resolveResourceParam(graph *Graph, sv starlark.Value, paramType reflect.Type) {
	if graph.Catalog == nil {
		return
	}

	// Skip promises and gathers — they'll be shadowed at execution time.
	if _, ok := sv.(*Promise); ok {
		return
	}
	if _, ok := sv.(*starlark.List); ok {
		return
	}

	// Check if the parameter type satisfies Resource.
	if !paramType.Implements(resourceType) &&
		!(paramType.Kind() == reflect.Struct && reflect.PointerTo(paramType).Implements(resourceType)) {
		return
	}

	// Unmarshal the Starlark value to get the Go representation.
	var goVal any
	if err := unmarshal(sv, &goVal); err != nil {
		return // best-effort; errors are caught later during execution
	}

	// Use the plan-time constructor to create a URI-only Resource.
	r, ok := constructResource(paramType, goVal)
	if !ok {
		return
	}

	uri := r.URI()
	if uri != "" {
		graph.Catalog.Resolve(uri)
	}
}
