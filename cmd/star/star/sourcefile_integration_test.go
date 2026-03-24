// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

//go:build integration

package star

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/star/config"
)

func TestSourceFile_StarlarkIntegration(t *testing.T) {

	dir := t.TempDir()
	config.SetGitWorkspaceRoot(dir)
	t.Cleanup(func() { config.ResetGitWorkspaceRoot() })

	// Minimal star config.
	if err := os.MkdirAll(filepath.Join(dir, "star"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "star", "config.yaml"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Extension with goast receiver.
	extDir := filepath.Join(dir, "star", "extensions", "com.test.SourceFile")
	cmdDir := filepath.Join(extDir, "commands")
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		t.Fatal(err)
	}

	extYAML := `extension: com.test.SourceFile
description: Test SourceFile integration
receivers:
  - name: goast
    builtin: true
    type: goast.Provider
    description: Go AST operations
commands:
  - name: test.source-file
    help: Test load_source_file
    implementation: commands/test-source-file.star
`
	if err := os.WriteFile(filepath.Join(extDir, "extension.yaml"), []byte(extYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write test Go source to a file.
	testGoSource := `package example

// Provider provides operations.
type Provider struct {
	name string
}

// NewProvider creates a new Provider.
//
// Parameters:
//   - name: the provider name.
//
// Returns:
//   - *Provider: the new provider.
func NewProvider(name string) *Provider {
	return &Provider{name: name}
}

// region EXPORTED METHODS

// Backup backs up data.
//
// Parameters:
//   - path: the file path to back up.
//
// Returns:
//   - string: the backup location.
//   - error: non-nil if the backup fails.
func (p *Provider) Backup(path string) (string, error) {
	return path, nil
}

// Restore restores data.
//
// Parameters:
//   - path: the file path to restore.
//
// Returns:
//   - error: non-nil if the restore fails.
func (p *Provider) Restore(path string) error {
	return nil
}

// endregion

// DefaultName is the default provider name.
const DefaultName = "default"

// MaxRetries is the maximum retry count.
var MaxRetries = 3
`
	testGoPath := filepath.Join(dir, "example.go")
	if err := os.WriteFile(testGoPath, []byte(testGoSource), 0o644); err != nil {
		t.Fatal(err)
	}

	starScript := `def run(command, ctx):
    ast = goast.load_source_file("` + testGoPath + `")

    # --- Package name (string return = eagerly evaluated property) ---
    if ast.package_name != "example":
        fail("expected package 'example', got '%s'" % ast.package_name)

    # --- Types ---
    types = list(ast.types)
    if len(types) != 1:
        fail("expected 1 type, got %d" % len(types))
    if types[0].name != "Provider":
        fail("expected type 'Provider', got '%s'" % types[0].name)
    if types[0].comment().text == None:
        fail("expected comment on Provider")

    # --- Get type by name ---
    provider = ast.get_type(name="Provider")
    if provider == None:
        fail("get_type('Provider') returned None")
    if provider.name != "Provider":
        fail("expected 'Provider', got '%s'" % provider.name)

    # --- Methods on type ---
    methods = list(provider.methods)
    if len(methods) != 2:
        fail("expected 2 methods on Provider, got %d" % len(methods))

    backup = provider.get_method(name="Backup")
    if backup == None:
        fail("get_method('Backup') returned None")
    if backup.name != "Backup":
        fail("expected 'Backup', got '%s'" % backup.name)
    if backup.returns == "":
        fail("expected returns on Backup")

    # --- Params on method ---
    params = list(backup.params)
    if len(params) != 1:
        fail("expected 1 param on Backup, got %d" % len(params))
    if params[0].name != "path":
        fail("expected param 'path', got '%s'" % params[0].name)
    if params[0].type != "string":
        fail("expected param type 'string', got '%s'" % params[0].type)

    # --- Top-level functions ---
    funcs = list(ast.funcs)
    if len(funcs) != 1:
        fail("expected 1 func, got %d" % len(funcs))
    if funcs[0].name != "NewProvider":
        fail("expected 'NewProvider', got '%s'" % funcs[0].name)
    if funcs[0].comment().text == None:
        fail("expected comment on NewProvider")

    new_provider = ast.get_func(name="NewProvider")
    if new_provider == None:
        fail("get_func('NewProvider') returned None")

    # --- Constants ---
    consts = list(ast.consts)
    if len(consts) != 1:
        fail("expected 1 const group, got %d" % len(consts))

    entries = list(consts[0].entries)
    if len(entries) != 1:
        fail("expected 1 const entry, got %d" % len(entries))
    if entries[0].name != "DefaultName":
        fail("expected 'DefaultName', got '%s'" % entries[0].name)
    if entries[0].value != "default":
        fail("expected 'default', got '%s'" % entries[0].value)

    # --- Variables ---
    vars = list(ast.vars)
    if len(vars) != 1:
        fail("expected 1 var, got %d" % len(vars))
    if vars[0].name != "MaxRetries":
        fail("expected 'MaxRetries', got '%s'" % vars[0].name)

    # --- Declarations in source order (includes floating comments) ---
    decls = list(ast.decls)
    comment_count = 0
    for d in decls:
        if d.decl_kind == "comment":
            comment_count += 1
    if comment_count != 2:
        fail("expected 2 floating comments, got %d" % comment_count)
`
	if err := os.WriteFile(filepath.Join(cmdDir, "test-source-file.star"), []byte(starScript), 0o644); err != nil {
		t.Fatal(err)
	}

	// Find project root BEFORE changing directories.
	projectRoot, err := findProjectRoot()
	if err != nil {
		t.Fatalf("findProjectRoot: %v", err)
	}

	origDir, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	r := NewRuntime()

	// Load the test extension.
	if err := r.LoadExtensionsFrom(filepath.Join(dir, "star", "extensions")); err != nil {
		t.Fatalf("LoadExtensionsFrom: %v", err)
	}

	// Also load project extensions so goast receiver is available.
	if err := r.LoadExtensionsFrom(filepath.Join(projectRoot, "cmd", "star", "extensions")); err != nil {
		t.Fatalf("LoadExtensionsFrom (project): %v", err)
	}

	cmd, ok := r.Commands()["test source-file"]
	if !ok {
		t.Fatalf("command 'test source-file' not found, available: %v", keys(r.Commands()))
	}

	if err := cmd.Run(map[string]string{}); err != nil {
		t.Fatalf("Starlark test failed: %v", err)
	}
}
