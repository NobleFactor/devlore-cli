// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package config

// ShowResult holds config with source information.
type ShowResult struct {
	Config  any          `starlark:"config"`
	Sources []SourceInfo `starlark:"sources"`
}

// SourceInfo represents a config source file and whether it exists.
type SourceInfo struct {
	Path   string `starlark:"path"`
	Exists bool   `starlark:"exists"`
}

// SyncResult holds the outcome of a config sync.
type SyncResult struct {
	GolangciLint   string `starlark:"golangci_lint"`
	MarkdownLint   string `starlark:"markdown_lint"`
	FilesGenerated int    `starlark:"files_generated"`
}
