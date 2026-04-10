// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package star

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"gopkg.in/yaml.v3"

	_ "github.com/NobleFactor/devlore-cli/cmd/star/inventory"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/inventory"
)

// =============================================================================
// Test: DiscoverAndLoad
// =============================================================================

func TestRuntime_DiscoverAndLoad(t *testing.T) {
	t.Run("empty embedded FS loads without error", func(t *testing.T) {
		r := NewApplication()
		loader := NewExtensionLoader(fstest.MapFS{})

		err := r.DiscoverAndLoad(loader)
		if err != nil {
			t.Errorf("DiscoverAndLoad() error = %v, want nil", err)
		}
	})

	t.Run("loads embedded extensions", func(t *testing.T) {
		r := NewApplication()
		loader := NewExtensionLoader(buildTestEmbeddedFS(t))

		err := r.DiscoverAndLoad(loader)
		if err != nil {
			t.Fatalf("DiscoverAndLoad() error = %v", err)
		}

		// Verify extension was registered
		ext := r.Registry().Get("com.example.TestEmbed")
		if ext == nil {
			t.Error("expected extension 'com.example.TestEmbed' to be registered")
		}

		// Verify command was loaded
		cmds := r.Commands()
		if _, ok := cmds["test embed"]; !ok {
			t.Errorf("expected command 'test embed', got keys: %v", keys(cmds))
		}
	})
}

// =============================================================================
// Test: loadExtensionCommands
// =============================================================================

func TestRuntime_loadExtensionCommands(t *testing.T) {
	t.Run("command name transformation", func(t *testing.T) {
		ext := buildTestExtensionFromDir(t, "com.example.LintCopyright", "lint.copyright", "commands/lint-copyright.star")
		r := NewApplication()

		err := r.loadExtensionCommands(ext)
		if err != nil {
			t.Fatalf("loadExtensionCommands() error = %v", err)
		}

		cmds := r.Commands()
		if _, ok := cmds["lint copyright"]; !ok {
			t.Errorf("expected command 'lint copyright', got keys: %v", keys(cmds))
		}
	})

	t.Run("applies flag defaults from spec", func(t *testing.T) {
		tmpDir := t.TempDir()
		extDir := filepath.Join(tmpDir, "com.example.TestExt")
		cmdDir := filepath.Join(extDir, "commands")
		if err := os.MkdirAll(cmdDir, 0755); err != nil {
			t.Fatal(err)
		}

		starContent := "def run(command, ctx):\n    pass\n"
		if err := os.WriteFile(filepath.Join(cmdDir, "test.star"), []byte(starContent), 0644); err != nil {
			t.Fatal(err)
		}

		ext := &Extension{
			Name: "com.example.TestExt",
			Dir:  extDir,
			Commands: []*Command{
				{
					Name:           "test.ext",
					Help:           "Test extension",
					Implementation: "commands/test.star",
					Flags: []Flag{
						{Name: "fix", Type: "bool", Default: "false", Help: "Fix issues"},
						{Name: "path", Type: "string", Default: ".", Help: "Path to check"},
					},
				},
			},
		}
		for _, cmd := range ext.Commands {
			cmd.Extension = ext
		}

		r := NewApplication()
		err := r.loadExtensionCommands(ext)
		if err != nil {
			t.Fatalf("loadExtensionCommands() error = %v", err)
		}

		cmd := r.Commands()["test ext"]
		if cmd == nil {
			t.Fatal("command not found")
		}

		flagsByName := make(map[string]Flag)
		for _, f := range cmd.Flags {
			flagsByName[f.Name] = f
		}

		if f, ok := flagsByName["fix"]; !ok {
			t.Error("missing 'fix' flag")
		} else {
			if f.Default != "false" {
				t.Errorf("fix.Default = %q, want %q", f.Default, "false")
			}
			if f.Help != "Fix issues" {
				t.Errorf("fix.Help = %q, want %q", f.Help, "Fix issues")
			}
		}

		if f, ok := flagsByName["path"]; !ok {
			t.Error("missing 'path' flag from spec")
		} else if f.Default != "." {
			t.Errorf("path.Default = %q, want %q", f.Default, ".")
		}
	})

	t.Run("no commands is no-op", func(t *testing.T) {
		r := NewApplication()
		ext := &Extension{Name: "com.example.NoCommand"}

		err := r.loadExtensionCommands(ext)
		if err != nil {
			t.Errorf("loadExtensionCommands() error = %v, want nil", err)
		}
	})

	t.Run("empty implementation is skipped", func(t *testing.T) {
		r := NewApplication()
		ext := &Extension{
			Name: "com.example.EmptyImpl",
			Commands: []*Command{
				{Name: "empty.impl", Help: "Empty impl", Implementation: ""},
			},
		}

		err := r.loadExtensionCommands(ext)
		if err != nil {
			t.Errorf("loadExtensionCommands() error = %v, want nil", err)
		}
	})

	t.Run("multiple commands loaded", func(t *testing.T) {
		tmpDir := t.TempDir()
		extDir := filepath.Join(tmpDir, "com.example.MultiCmd")
		cmdDir := filepath.Join(extDir, "commands")
		if err := os.MkdirAll(cmdDir, 0755); err != nil {
			t.Fatal(err)
		}

		starContent := "def run(command, ctx):\n    pass\n"
		if err := os.WriteFile(filepath.Join(cmdDir, "one.star"), []byte(starContent), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cmdDir, "two.star"), []byte(starContent), 0644); err != nil {
			t.Fatal(err)
		}

		ext := &Extension{
			Name: "com.example.MultiCmd",
			Dir:  extDir,
			Commands: []*Command{
				{Name: "multi.one", Help: "First", Implementation: "commands/one.star"},
				{Name: "multi.two", Help: "Second", Implementation: "commands/two.star"},
			},
		}
		for _, cmd := range ext.Commands {
			cmd.Extension = ext
		}

		r := NewApplication()
		err := r.loadExtensionCommands(ext)
		if err != nil {
			t.Fatalf("loadExtensionCommands() error = %v", err)
		}

		cmds := r.Commands()
		if _, ok := cmds["multi one"]; !ok {
			t.Errorf("expected command 'multi one', got keys: %v", keys(cmds))
		}
		if _, ok := cmds["multi two"]; !ok {
			t.Errorf("expected command 'multi two', got keys: %v", keys(cmds))
		}
	})

	t.Run("command references parent extension", func(t *testing.T) {
		ext := buildTestExtensionFromDir(t, "com.example.ParentRef", "parent.ref", "commands/ref.star")
		r := NewApplication()

		err := r.loadExtensionCommands(ext)
		if err != nil {
			t.Fatalf("loadExtensionCommands() error = %v", err)
		}

		cmd := r.Commands()["parent ref"]
		if cmd == nil {
			t.Fatal("command not found")
		}
		if cmd.Extension != ext {
			t.Error("command.Extension should reference the parent extension")
		}
		if cmd.Extension.Name != "com.example.ParentRef" {
			t.Errorf("command.Extension.Name = %q, want %q", cmd.Extension.Name, "com.example.ParentRef")
		}
	})
}

// =============================================================================
// Test: Config
// =============================================================================

func TestRuntime_Config(t *testing.T) {
	r := NewApplication()

	if r.config != nil {
		t.Error("expected config to be nil initially")
	}

	cfg := r.Config()
	if cfg == nil {
		t.Fatal("Config() returned nil")
	}

	cfg2 := r.Config()
	if cfg != cfg2 {
		t.Error("Config() should return same instance")
	}
}

// =============================================================================
// Test: Extension
// =============================================================================

func TestExtension_Immutable(t *testing.T) {
	ext := &Extension{
		Name:        "com.example.Immutable",
		Description: "Test immutability",
		Source:      SourceEmbedded,
	}

	if ext.Name != "com.example.Immutable" {
		t.Errorf("Name = %q, want %q", ext.Name, "com.example.Immutable")
	}
	if ext.Source != SourceEmbedded {
		t.Errorf("Source = %v, want %v", ext.Source, SourceEmbedded)
	}
}

func TestExtension_StarlarkValue(t *testing.T) {
	ext := &Extension{
		Name:        "com.example.Starlark",
		Description: "Starlark test",
		Source:      SourceProjectLocal,
	}

	if ext.Type() != "extension" {
		t.Errorf("Type() = %q, want %q", ext.Type(), "extension")
	}
	if ext.Truth() != true {
		t.Error("Truth() should be true")
	}

	nameVal, err := ext.Attr("name")
	if err != nil {
		t.Fatalf("Attr(name) error = %v", err)
	}
	if nameVal.String() != `"com.example.Starlark"` {
		t.Errorf("Attr(name) = %v, want %q", nameVal, "com.example.Starlark")
	}
}

func TestExtension_YAMLDeserialization(t *testing.T) {
	t.Run("parses YAML into Extension", func(t *testing.T) {
		input := `extension: com.example.Test
description: Test extension
commands:
  - name: test.cmd
    help: A test command
    implementation: commands/test.star
    flags:
      - name: verbose
        type: bool
        default: "false"
        help: Enable verbose output
config:
  type: TestConfig
  fields:
    enabled: bool
  defaults:
    enabled: false
`
		var ext Extension
		if err := yaml.Unmarshal([]byte(input), &ext); err != nil {
			t.Fatalf("yaml.Unmarshal error = %v", err)
		}

		if ext.Name != "com.example.Test" {
			t.Errorf("Name = %q, want %q", ext.Name, "com.example.Test")
		}
		if len(ext.Commands) != 1 {
			t.Fatalf("len(Commands) = %d, want 1", len(ext.Commands))
		}
		if ext.Commands[0].Name != "test.cmd" {
			t.Errorf("Command.Name = %q, want %q", ext.Commands[0].Name, "test.cmd")
		}
		if ext.Config == nil {
			t.Fatal("Config should not be nil")
		}
		if ext.Config.Type != "TestConfig" {
			t.Errorf("Config.Type = %q, want %q", ext.Config.Type, "TestConfig")
		}
	})

	t.Run("validates after deserialization", func(t *testing.T) {
		input := `extension: com.example.Empty
description: No commands
`
		var ext Extension
		if err := yaml.Unmarshal([]byte(input), &ext); err != nil {
			t.Fatalf("yaml.Unmarshal error = %v", err)
		}

		err := ext.Validate()
		if err == nil {
			t.Error("expected validation error for extension with no commands")
		}
	})
}

// =============================================================================
// Test: Source
// =============================================================================

func TestSource_String(t *testing.T) {
	tests := []struct {
		source Source
		want   string
	}{
		{SourceProjectLocal, "project-local"},
		{SourceUser, "user"},
		{SourceSystem, "system"},
		{SourceEmbedded, "embedded"},
	}

	for _, tt := range tests {
		if got := tt.source.String(); got != tt.want {
			t.Errorf("Source(%d).String() = %q, want %q", tt.source, got, tt.want)
		}
	}
}

// =============================================================================
// Helpers
// =============================================================================

// buildTestExtensionFromDir creates a temp dir with a .star file and returns the extension.
func buildTestExtensionFromDir(t *testing.T, extName, cmdName, implPath string) *Extension {
	t.Helper()

	tmpDir := t.TempDir()
	extDir := filepath.Join(tmpDir, extName)
	cmdDir := filepath.Join(extDir, "commands")
	if err := os.MkdirAll(cmdDir, 0755); err != nil {
		t.Fatal(err)
	}

	starContent := "def run(command, ctx):\n    pass\n"
	if err := os.WriteFile(filepath.Join(extDir, implPath), []byte(starContent), 0644); err != nil {
		t.Fatal(err)
	}

	ext := &Extension{
		Name:   extName,
		Dir:    extDir,
		Source: SourceProjectLocal,
		Commands: []*Command{
			{
				Name:           cmdName,
				Help:           "Test command",
				Implementation: implPath,
			},
		},
	}
	for _, cmd := range ext.Commands {
		cmd.Extension = ext
	}
	return ext
}

// buildTestEmbeddedFS creates an in-memory FS with a test extension.
func buildTestEmbeddedFS(t *testing.T) fs.FS {
	t.Helper()

	return fstest.MapFS{
		"com.example.TestEmbed/extension.yaml": &fstest.MapFile{
			Data: []byte(`extension: com.example.TestEmbed
description: Test embedded extension
commands:
  - name: test.embed
    help: Test embedded command
    implementation: commands/test-embed.star
`),
		},
		"com.example.TestEmbed/commands/test-embed.star": &fstest.MapFile{
			Data: []byte("def run(command, ctx):\n    pass\n"),
		},
	}
}

func keys[K comparable, V any](m map[K]V) []K {
	result := make([]K, 0, len(m))
	for k := range m {
		result = append(result, k)
	}
	return result
}
