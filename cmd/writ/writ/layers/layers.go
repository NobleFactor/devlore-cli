// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package layers reads writ's layer registry: whether each layer is registered, and where its working tree is.
//
// A layer is a git working tree, because deploy pins layers from git history; `writ repo set` registers nothing else.
// Every part of writ that asks whether a layer is registered asks [Read], so an entry that is not a working tree,
// such as the empty directories `self install` once made where the layers go, is no layer to any of them (#1030).
package layers

import (
	"os"
	"path/filepath"

	"github.com/NobleFactor/devlore-cli/cmd/internal/devlore"
)

// State is where a layer stands in the registry.
type State string

// The states a layer's entry can be in.
const (
	// Registered is an entry that resolves to a git working tree.
	Registered State = "registered"

	// Unregistered is no entry, or a directory that is not a git working tree.
	Unregistered State = "unregistered"

	// Broken is a link that does not resolve, or resolves to something that is not a git working tree, or an entry
	// that is neither a link nor a directory.
	Broken State = "broken"

	// Unreadable is a link that cannot be read.
	Unreadable State = "unreadable"
)

// Layer is one layer's entry in the registry, read.
type Layer struct {
	Name   string // the layer: base, team or personal
	Path   string // its entry, beneath [devlore.WritLayersDir]
	Link   bool   // whether the entry is a symbolic link
	Target string // what the entry names: a link's target as written, else Path; empty when unregistered or unreadable
	Root   string // the working tree: a link's target resolved, else Path; empty unless registered
	State  State  // where the layer stands
}

// region EXPORTED FUNCTIONS

// IsWorkingTree reports whether `root` is a git working tree: a directory holding `.git`.
//
// `.git` may be a directory or, in a linked worktree, a file; either makes a working tree.
//
// Parameters:
//   - `root`: the directory to test.
//
// Returns:
//   - `bool`: true for a git working tree.
func IsWorkingTree(root string) bool {

	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return false
	}

	_, err = os.Stat(filepath.Join(root, ".git"))

	return err == nil
}

// Read reads the layer `name`'s entry in the registry.
//
// Parameters:
//   - `name`: the layer.
//
// Returns:
//   - `Layer`: the entry, and where the layer stands.
func Read(name string) Layer {

	layer := Layer{Name: name, Path: filepath.Join(devlore.WritLayersDir(), name), State: Unregistered}

	info, err := os.Lstat(layer.Path)
	if err != nil {
		return layer
	}

	layer.Link = info.Mode()&os.ModeSymlink != 0
	if !layer.Link {
		switch {
		case !info.IsDir():
			layer.Target = layer.Path
			layer.State = Broken
		case IsWorkingTree(layer.Path):
			layer.Target = layer.Path
			layer.Root = layer.Path
			layer.State = Registered
		}
		return layer
	}

	target, err := os.Readlink(layer.Path)
	if err != nil {
		layer.State = Unreadable
		return layer
	}
	layer.Target = target

	root, err := filepath.EvalSymlinks(layer.Path)
	if err != nil || !IsWorkingTree(root) {
		layer.State = Broken
		return layer
	}
	layer.Root = root
	layer.State = Registered

	return layer
}

// endregion
