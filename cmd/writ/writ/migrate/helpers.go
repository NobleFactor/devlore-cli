// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package migrate

import (
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// actionName returns the bound action's name, or the empty string when no action is bound.
//
// Parameters:
//   - `node`: the node to read the action name from.
//
// Returns:
//   - `string`: the action name, or the empty string.
func actionName(node *op.Node) string {

	action := node.Action()
	if action == nil {
		return ""
	}
	return action.Name()
}

// filterNodesByAction returns the graph's nodes bound to the named action.
//
// Parameters:
//   - `graph`: the graph whose nodes to filter.
//   - `name`: the dotted action name to match (e.g. "file.move").
//
// Returns:
//   - `[]*op.Node`: the matching nodes, in graph order; empty when none match.
func filterNodesByAction(graph *op.Graph, name string) []*op.Node {

	var nodes []*op.Node
	for _, node := range graph.Nodes() {
		if actionName(node) == name {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// immediateString returns the string value of `node`'s immediate `slot` binding.
//
// The empty string stands for "absent": a missing slot, a non-immediate binding (a promise or variable
// reference), or a non-string value all yield "". Reporting-only — the executor resolves slots for dispatch;
// this helper serves the human-facing surfaces (the migration plan view, the .writ-migrated marker, and the
// pre-run conflict check).
//
// Parameters:
//   - `node`: the node whose slot to read.
//   - `slot`: the slot name.
//
// Returns:
//   - `string`: the immediate string value, or "" when absent.
func immediateString(node *op.Node, slot string) string {

	binding, ok := node.Slots()[slot].(op.ImmediateBinding)
	if !ok {
		return ""
	}

	value, _ := binding.Resolve(nil, nil).(string)
	return value
}
