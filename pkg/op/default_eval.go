// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"reflect"
	"text/template/parse"
)

// region UNEXPORTED FUNCTIONS

// region Behaviors

// evalTree evaluates the root of a parsed deferred-default tree and returns the result as a natural Go
// value.
//
// The tree's root is expected to hold exactly one [parse.ActionNode] — the single `{{ ... }}` expression
// the directive body wrapped. Multi-action and bare-text trees are rejected; defaults are not templates,
// they're expressions that produce one value.
//
// Parameters:
//   - tree:     the parsed AST to evaluate.
//   - env:      live runtime environment passed through to every [DefaultFunc] call.
//   - siblings: already-filled slot values from the dispatching call, keyed by parameter name.
//
// Returns:
//   - any:   the resolved value, type determined by the deepest [DefaultFunc] in the pipeline.
//   - error: non-nil on any node-walking, lookup, or function-call failure.
func evalTree(tree *parse.Tree, env *RuntimeEnvironment, siblings map[string]any) (any, error) {

	if tree == nil || tree.Root == nil {
		return nil, fmt.Errorf("empty parse tree")
	}

	if len(tree.Root.Nodes) != 1 {
		return nil, fmt.Errorf("expected single action expression, got %d nodes", len(tree.Root.Nodes))
	}

	action, ok := tree.Root.Nodes[0].(*parse.ActionNode)
	if !ok {
		return nil, fmt.Errorf("expected action expression, got %T", tree.Root.Nodes[0])
	}

	rv, err := evalPipe(action.Pipe, env, siblings)
	if err != nil {
		return nil, err
	}

	if !rv.IsValid() {
		return nil, nil
	}

	return rv.Interface(), nil
}

// evalPipe evaluates a pipeline of commands `cmd1 | cmd2 | cmd3`. The result of each command becomes
// the trailing argument of the next; the pipeline's value is the last command's return.
//
// Parameters:
//   - pipe:     the pipeline node to evaluate.
//   - env:      live runtime environment.
//   - siblings: already-filled slot values.
//
// Returns:
//   - reflect.Value: the pipeline's final value.
//   - error:         non-nil if any command in the chain fails.
func evalPipe(pipe *parse.PipeNode, env *RuntimeEnvironment, siblings map[string]any) (reflect.Value, error) {

	if pipe == nil || len(pipe.Cmds) == 0 {
		return reflect.Value{}, fmt.Errorf("empty pipeline")
	}

	var prev reflect.Value
	hasPrev := false

	for _, cmd := range pipe.Cmds {

		var prevPtr *reflect.Value
		if hasPrev {
			prevPtr = &prev
		}

		rv, err := evalCommand(cmd, env, siblings, prevPtr)
		if err != nil {
			return reflect.Value{}, err
		}
		prev = rv
		hasPrev = true
	}

	return prev, nil
}

// evalCommand evaluates one command (function call). The command's first argument is the function
// identifier; the remaining arguments evaluate independently and feed the function. If a pipeline-chained
// previous value is supplied, it is appended as the trailing argument after the named arguments.
//
// Parameters:
//   - cmd:      the command node to evaluate.
//   - env:      live runtime environment.
//   - siblings: already-filled slot values.
//   - prev:     the previous pipeline stage's result, or nil if this is the first command.
//
// Returns:
//   - reflect.Value: the function's return value.
//   - error:         non-nil if the function is unknown, an argument fails to evaluate, or the function
//     itself returns an error.
func evalCommand(cmd *parse.CommandNode, env *RuntimeEnvironment, siblings map[string]any, prev *reflect.Value) (reflect.Value, error) {

	if cmd == nil || len(cmd.Args) == 0 {
		return reflect.Value{}, fmt.Errorf("empty command")
	}

	ident, ok := cmd.Args[0].(*parse.IdentifierNode)
	if !ok {
		return reflect.Value{}, fmt.Errorf("expected function identifier, got %T", cmd.Args[0])
	}

	fn, found := announced.defaultFunc(ident.Ident)
	if !found {
		return reflect.Value{}, fmt.Errorf("unknown default function %q", ident.Ident)
	}

	args := make([]reflect.Value, 0, len(cmd.Args)-1)
	for _, arg := range cmd.Args[1:] {
		rv, err := evalArg(arg, env, siblings)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("%s: %w", ident.Ident, err)
		}
		args = append(args, rv)
	}

	if prev != nil {
		args = append(args, *prev)
	}

	return fn(env, siblings, args)
}

// evalArg evaluates one argument node into a [reflect.Value]. Supported node types are the value-bearing
// leaves the directive grammar admits: numbers, strings, booleans, sibling-slot field references, and
// nested pipes (parenthesized sub-expressions). All other node types are rejected as unsupported in
// directive expressions.
//
// Parameters:
//   - node:     the argument node.
//   - env:      live runtime environment (passed only when the argument is a nested pipe).
//   - siblings: already-filled slot values (for FieldNode lookups).
//
// Returns:
//   - reflect.Value: the argument's evaluated value.
//   - error:         non-nil for unsupported node types or sibling-lookup failures.
func evalArg(node parse.Node, env *RuntimeEnvironment, siblings map[string]any) (reflect.Value, error) {

	switch n := node.(type) {

	case *parse.NumberNode:
		return evalNumber(n)

	case *parse.StringNode:
		return reflect.ValueOf(n.Text), nil

	case *parse.BoolNode:
		return reflect.ValueOf(n.True), nil

	case *parse.FieldNode:
		return evalField(n, siblings)

	case *parse.PipeNode:
		return evalPipe(n, env, siblings)

	default:
		return reflect.Value{}, fmt.Errorf("unsupported argument node %T", node)
	}
}

// evalNumber extracts the natural-typed Go value from a parsed number node. text/template/parse pre-
// parses numeric literals into Int64 / Uint64 / Float64 / Complex128 fields with corresponding flags;
// this picks the most natural representation in priority order int → uint → float → complex.
//
// Parameters:
//   - n: the number node.
//
// Returns:
//   - reflect.Value: the value as int64, uint64, float64, or complex128.
//   - error:         non-nil if the node has no valid representation.
func evalNumber(n *parse.NumberNode) (reflect.Value, error) {

	if n.IsInt {
		return reflect.ValueOf(n.Int64), nil
	}
	if n.IsUint {
		return reflect.ValueOf(n.Uint64), nil
	}
	if n.IsFloat {
		return reflect.ValueOf(n.Float64), nil
	}
	if n.IsComplex {
		return reflect.ValueOf(n.Complex128), nil
	}
	return reflect.Value{}, fmt.Errorf("number %q has no valid representation", n.Text)
}

// evalField resolves a single-segment field reference against the siblings map. Multi-segment chains
// (e.g., `.outer.inner`) are not supported in v1 — siblings is a flat map of parameter-name to value.
//
// Parameters:
//   - n:        the field node.
//   - siblings: already-filled slot values, keyed by parameter name.
//
// Returns:
//   - reflect.Value: the looked-up value, wrapped in reflect.
//   - error:         non-nil if the field is empty, multi-segment, or names an unfilled slot.
func evalField(n *parse.FieldNode, siblings map[string]any) (reflect.Value, error) {

	if len(n.Ident) == 0 {
		return reflect.Value{}, fmt.Errorf("empty field reference")
	}

	if len(n.Ident) > 1 {
		return reflect.Value{}, fmt.Errorf("nested field reference %v not supported", n.Ident)
	}

	name := n.Ident[0]
	val, ok := siblings[name]
	if !ok {
		return reflect.Value{}, fmt.Errorf("sibling slot %q is unfilled", name)
	}

	return reflect.ValueOf(val), nil
}

// endregion

// endregion