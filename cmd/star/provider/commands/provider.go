// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

// Package commands provides command tree navigation and execution for the star runtime.
package commands

import (
	"fmt"
	"sort"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

var _ op.Provider = (*Provider)(nil)

// DataKeyCommandTree is the context Data key for the CommandTree.
const DataKeyCommandTree = "command_tree"

// DataKeyCurrentCommand is the context Data key for the current command name.
const DataKeyCurrentCommand = "current_command"

// Provider provides command tree navigation and execution operations.
//
// +devlore:access=immediate
type Provider struct {
	op.ProviderBase
}

// NewProvider creates a commands provider bound to the given context.
func NewProvider(ctx *op.RuntimeEnvironment) *Provider {
	return &Provider{ProviderBase: op.NewProviderBase(ctx)}
}

func (p *Provider) tree() CommandTree {
	t, _ := p.RuntimeEnvironment().Data[DataKeyCommandTree].(CommandTree)
	return t
}

func (p *Provider) currentCommand() string {
	s, _ := p.RuntimeEnvironment().Data[DataKeyCurrentCommand].(string)
	return s
}

// Current returns the name of the currently executing command.
//
// Returns:
//   - string: the dot-separated command name
//   - error: never
func (p *Provider) Current() (string, error) {
	return p.currentCommand(), nil
}

// Parent returns the parent path of the current command.
//
// Returns:
//   - string: the parent path (e.g., "lint" for "lint.all")
//   - error: never
func (p *Provider) Parent() (string, error) {
	return getParentPath(p.currentCommand()), nil
}

// Siblings returns all sibling commands (same parent, excluding self).
//
// Returns:
//   - []*CommandRef: sibling commands
//   - error: never
func (p *Provider) Siblings() ([]*CommandRef, error) {
	tree := p.tree()
	if tree == nil {
		return nil, nil
	}

	parent := getParentPath(p.currentCommand())
	var siblings []*CommandRef

	for _, name := range tree.CommandNames() {
		dotName := strings.ReplaceAll(name, " ", ".")
		if getParentPath(dotName) == parent && dotName != p.currentCommand() {
			siblings = append(siblings, NewCommandRef(dotName, tree))
		}
	}

	sort.Slice(siblings, func(i, j int) bool {
		return siblings[i].name < siblings[j].name
	})

	return siblings, nil
}

// Children returns all child commands of the given parent path.
//
// +devlore:defaults parent=""
//
// Parameters:
//   - parent: parent path to search (default: current command's parent)
//
// Returns:
//   - []*CommandRef: child commands
//   - error: never
func (p *Provider) Children(parent string) ([]*CommandRef, error) {
	tree := p.tree()
	if tree == nil {
		return nil, nil
	}

	if parent == "" {
		parent = getParentPath(p.currentCommand())
	}

	var children []*CommandRef
	for _, name := range tree.CommandNames() {
		dotName := strings.ReplaceAll(name, " ", ".")
		if getParentPath(dotName) == parent {
			children = append(children, NewCommandRef(dotName, tree))
		}
	}

	sort.Slice(children, func(i, j int) bool {
		return children[i].name < children[j].name
	})

	return children, nil
}

// Query returns commands matching a pattern with * wildcards.
//
// Parameters:
//   - pattern: glob pattern (e.g., "lint.*", "*.go", "*")
//
// Returns:
//   - []*CommandRef: matching commands
//   - error: never
func (p *Provider) Query(pattern string) ([]*CommandRef, error) {
	tree := p.tree()
	if tree == nil {
		return nil, nil
	}

	var matches []*CommandRef
	for _, name := range tree.CommandNames() {
		dotName := strings.ReplaceAll(name, " ", ".")
		if matchPattern(dotName, pattern) {
			matches = append(matches, NewCommandRef(dotName, tree))
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].name < matches[j].name
	})

	return matches, nil
}

// Get returns a specific command by name, or nil if not found.
//
// Parameters:
//   - name: dot-separated command name
//
// Returns:
//   - *CommandRef: the command, or nil
//   - error: never
func (p *Provider) Get(name string) (*CommandRef, error) {
	tree := p.tree()
	if tree == nil {
		return nil, nil
	}

	spaceName := strings.ReplaceAll(name, ".", " ")
	for _, n := range tree.CommandNames() {
		if n == spaceName {
			return NewCommandRef(name, tree), nil
		}
	}

	return nil, nil
}

// Run executes a command by name with the given arguments.
//
// Parameters:
//   - name: dot-separated command name
//   - flags: keyword arguments passed to the command
//
// Returns:
//   - RunResult: pass/fail and error message
//   - error: if command not found
func (p *Provider) Run(name string, flags map[string]string) (RunResult, error) {
	tree := p.tree()
	if tree == nil {
		return RunResult{}, fmt.Errorf("no command tree available")
	}

	spaceName := strings.ReplaceAll(name, ".", " ")
	err := tree.RunCommand(spaceName, flags)

	errStr := ""
	if err != nil {
		errStr = err.Error()
	}

	return RunResult{Passed: err == nil, Error: errStr}, nil
}
