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
	bridge BuiltinFunc
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
		Receiver:     NewReceiver("plan." + name),
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

		snakeName := CamelToSnake(m.Name)
		actionName := name + "." + snakeName

		// Only expose methods that have a registered action.
		if _, ok := reg.Get(actionName); !ok {
			continue
		}

		paramNames, ok := params[m.Name]
		if !ok {
			continue
		}

		bridge := buildPlannedBridge(name, snakeName, actionName, paramNames, graph, project, reg)
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
func (p *ReflectedPlanned) Override(name string, fn BuiltinFunc) {
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

// buildPlannedBridge creates a BuiltinFunc that creates a graph Node.
func buildPlannedBridge(
	providerName, snakeName, actionName string,
	paramNames []string,
	graph *Graph,
	project string,
	reg *ActionRegistry,
) BuiltinFunc {
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

		// 3. Fill slots from Starlark values.
		for i, name := range paramNames {
			cleanName := strings.TrimSuffix(name, "?")
			sv := vals[i]
			if sv == nil {
				continue // Optional param not provided.
			}
			if err := FillSlot(node, graph, cleanName, sv); err != nil {
				return nil, fmt.Errorf("%s: %w", cleanName, err)
			}
		}

		// 4. Append to graph and return promise.
		graph.Nodes = append(graph.Nodes, node)
		return NewOutput(node, graph, ""), nil
	}
}
