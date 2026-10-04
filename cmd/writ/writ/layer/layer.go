// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package layer reads a layer's registration: whether it is registered, and where its working tree is.
//
// A layer is a git working tree, because deploy pins layers from git history; `writ repo set` registers nothing else.
// Every part of writ that asks whether a layer is registered asks [Read], so an entry that is not a working tree,
// such as the empty directories `self install` once made where the layers go, is no layer to any of them (#1030).
package layer

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

// Registration is a layer's entry in the registry, read.
type Registration struct {
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

// Read reads the layer `name`'s registration.
//
// Parameters:
//   - `name`: the layer.
//
// Returns:
//   - `Registration`: the layer's entry, and where it stands.
func Read(name string) Registration {

	registration := Registration{Name: name, Path: filepath.Join(devlore.WritLayersDir(), name), State: Unregistered}

	info, err := os.Lstat(registration.Path)
	if err != nil {
		return registration
	}

	registration.Link = info.Mode()&os.ModeSymlink != 0
	if !registration.Link {
		switch {
		case !info.IsDir():
			registration.Target = registration.Path
			registration.State = Broken
		case IsWorkingTree(registration.Path):
			registration.Target = registration.Path
			registration.Root = registration.Path
			registration.State = Registered
		}
		return registration
	}

	target, err := os.Readlink(registration.Path)
	if err != nil {
		registration.State = Unreadable
		return registration
	}
	registration.Target = target

	root, err := filepath.EvalSymlinks(registration.Path)
	if err != nil || !IsWorkingTree(root) {
		registration.State = Broken
		return registration
	}
	registration.Root = root
	registration.State = Registered

	return registration
}

// endregion
