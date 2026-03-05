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

// ReflectedPlanned wraps a provider's method signatures for planned-mode
// Starlark use. Each call creates a graph Node instead of executing the
// method directly.
type ReflectedPlanned struct {
	Receiver
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

// WrapPlanned wraps a provider for planned-mode use. Only methods with
// a corresponding registered action in reg are exposed.
func WrapPlanned(
	name string,
	providerType reflect.Type,
	graph *Graph,
	project string,
	reg *ActionRegistry,
	params MethodParams,
) *ReflectedPlanned {
	p := &ReflectedPlanned{
		Receiver:     newReceiver("plan." + name),
		providerName: name,
		graph:        graph,
		project:      project,
		reg:          reg,
		methods:      make(map[string]*plannedBridge),
	}

	// Enumerate exported methods.
	for i := range providerType.NumMethod() {
		m := providerType.Method(i)
		if !m.IsExported() || strings.HasPrefix(m.Name, "Compensate") {
			continue
		}

		snakeName := camelToSnake(m.Name)
		actionName := name + "." + snakeName

		// Only expose methods that have a registered action.
		if _, ok := reg.Get(actionName); !ok {
			continue
		}

		paramNames, ok := params[m.Name]
		if !ok {
			continue
		}

		bridge := buildPlannedBridge(name, snakeName, actionName, paramNames, m, graph, project, reg)
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
func (p *ReflectedPlanned) Override(name string, fn builtinFunc) {
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
func (p *ReflectedPlanned) Attr(name string) (starlark.Value, error) {
	if m, ok := p.methods[name]; ok {
		return MakeAttr(p.Receiver.name+"."+name, m.bridge), nil
	}
	return nil, NoSuchAttrError(p.Receiver.name, name)
}

// AttrNames implements starlark.HasAttrs.
func (p *ReflectedPlanned) AttrNames() []string {
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
	return func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		// 1. Unpack args as raw starlark.Value (slots store Starlark
		//    values for deferred execution — NOT Go types).
		vals := make([]starlark.Value, len(paramNames))
		pairs := make([]any, 0, len(paramNames)*2)
		for i, name := range paramNames {
			pairs = append(pairs, name, &vals[i])
		}
		if err := starlark.UnpackArgs(snakeName, args, kwargs, pairs...); err != nil {
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
		for i, name := range paramNames {
			cleanName := strings.TrimSuffix(name, "?")
			sv := vals[i]
			if sv == nil {
				continue // Optional param not provided.
			}
			if err := FillSlot(node, graph, cleanName, sv); err != nil {
				return nil, fmt.Errorf("%s: %w", cleanName, err)
			}

			// Resolve immediate Resource parameters in the catalog.
			// Skip promises — they're shadowed at execution time (step 5f).
			if i+1 < mt.NumIn() {
				resolveResourceParam(graph, sv, mt.In(i+1))
			}
		}

		// 4. Append to graph and return promise.
		graph.Nodes = append(graph.Nodes, node)
		return NewOutput(node, graph, ""), nil
	}
}

// resolveResourceParam checks if a Starlark slot value targets a Resource-typed
// parameter. For immediate (non-promise) values, it constructs a plan-time
// Resource and calls catalog.Resolve to register the URI.
func resolveResourceParam(graph *Graph, sv starlark.Value, paramType reflect.Type) {
	if graph.Catalog == nil {
		return
	}

	// Skip promises and gathers — they'll be shadowed at execution time.
	if _, ok := sv.(*Output); ok {
		return
	}
	if _, ok := sv.(*Gather); ok {
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
	r, ok := constructPlanTimeResource(paramType, goVal)
	if !ok {
		return
	}

	uri := r.URI()
	if uri != "" {
		graph.Catalog.Resolve(uri)
	}
}
