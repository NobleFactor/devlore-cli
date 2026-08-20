// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package migrate

import (
	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// actionName returns the bound action's name, or the empty string when no action is bound.
//
// Parameters:
//   - `node`: the node to read the action name from.
//
// Returns:
//   - `op.ActionName`: the action name, or the empty string.
func actionName(node *op.Node) op.ActionName {

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
func filterNodesByAction(graph *op.Graph, name op.ActionName) []*op.Node {

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

	if value, ok := binding.Resolve(nil, nil).(string); ok {
		return value
	}
	return ""
}

// migrateSpec constructs a fresh [op.RuntimeEnvironmentSpec] confined at `root` for one phase of a migrate flow
// (planning or execution — each phase mints its own Root because the environment's Close closes it).
//
// The bare [application.Application] satisfies the runtime environment's non-nil requirement; no flag plumbing
// rides it — migrate's slots are immediates.
//
// Parameters:
//   - `root`: the absolute path the confined Root is anchored at.
//
// Returns:
//   - `*op.RuntimeEnvironmentSpec`: the constructed spec.
func migrateSpec(root string) *op.RuntimeEnvironmentSpec {

	return op.NewRuntimeEnvironmentSpec("writ").
		WithStatus(cli.UI()).
		WithRoot(root).
		WithApplication(&application.Application{Name: "writ"})
}
