// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

//go:build integration

package star

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/star/config"
)

// =============================================================================
// Test Fixtures
// =============================================================================

const mitLicenseText = `MIT License

Copyright (c) 2024 Test Corp

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
`

const apacheLicenseText = `Apache License
Version 2.0, January 2004
http://www.apache.org/licenses/

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
`

func starYAMLEnabled(holder, license string) string {
	if license == "" {
		license = "auto"
	}
	return `lint:
  copyright:
    enabled: true
    holder: "` + holder + `"
    license: "` + license + `"
    exclude: []
`
}

func starYAMLWithExcludes(holder string, excludes []string) string {
	excludeYAML := "["
	for i, e := range excludes {
		if i > 0 {
			excludeYAML += ", "
		}
		excludeYAML += `"` + e + `"`
	}
	excludeYAML += "]"

	return `lint:
  copyright:
    enabled: true
    holder: "` + holder + `"
    license: "auto"
    exclude: ` + excludeYAML + `
`
}

const goFileCorrectMIT = `// SPDX-License-Identifier: MIT
// Copyright Test Corp. All rights reserved.

package main

func main() {}
`

const goFileCorrectApache = `// SPDX-License-Identifier: Apache-2.0
// Copyright Test Corp. All rights reserved.

package main

func main() {}
`

const goFileNoHeader = `package main

func main() {}
`

const goFileWrongLicense = `// SPDX-License-Identifier: Apache-2.0
// Copyright Test Corp. All rights reserved.

package main

func main() {}
`

const goFileWrongHolder = `// SPDX-License-Identifier: MIT
// Copyright Other Corp. All rights reserved.

package main

func main() {}
`

const starFileNoHeader = `def hello():
    print("Hello, world!")
`

const shellFileNoHeader = `#!/bin/bash
echo "Hello, world!"
`

const shellFileCorrect = `#!/bin/bash

# SPDX-License-Identifier: MIT
# Copyright Test Corp. All rights reserved.

echo "Hello, world!"
`

// =============================================================================
// Test Helpers
// =============================================================================

type testFile struct {
	path    string
	content string
}

func setupTestDir(t *testing.T, files []testFile) string {
	t.Helper()
	dir := t.TempDir()

	// Set git workspace root to temp dir so config loading works
	config.SetGitWorkspaceRoot(dir)
	t.Cleanup(func() {
		config.ResetGitWorkspaceRoot()
	})

	for _, f := range files {
		path := filepath.Join(dir, f.path)

		// Create parent directories
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("creating directory for %s: %v", f.path, err)
		}

		if err := os.WriteFile(path, []byte(f.content), 0644); err != nil {
			t.Fatalf("writing %s: %v", f.path, err)
		}
	}

	return dir
}

func setupExtension(t *testing.T, testDir string) (*Application, error) {
	t.Helper()

	// Clear global extension registry

	// Find project root BEFORE changing directories
	projectRoot, err := findProjectRoot()
	if err != nil {
		return nil, err
	}

	// Change to test directory BEFORE creating runtime so file.Provider's
	// Root is set to the test directory.
	origDir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if err := os.Chdir(testDir); err != nil {
		return nil, err
	}
	t.Cleanup(func() {
		os.Chdir(origDir)
	})

	// Create runtime (Root picks up current working directory)
	r := NewApplication()

	// Load extensions from the project's star/extensions directory
	extDir := filepath.Join(projectRoot, "cmd", "star", "extensions")
	if err := r.LoadExtensionsFrom(extDir); err != nil {
		return nil, err
	}

	return r, nil
}

func findProjectRoot() (string, error) {
	// Start from current directory and walk up to find go.mod
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

func runLintCopyright(t *testing.T, r *Application, fix bool, path string) (passed bool, output string, err error) {
	t.Helper()

	cmd, ok := r.Commands()["lint copyright"]
	if !ok {
		return false, "", os.ErrNotExist
	}

	// Build args
	args := map[string]string{
		"fix":  "false",
		"path": path,
	}
	if fix {
		args["fix"] = "true"
	}

	// Capture output by running the command
	// The command returns an error on failure
	err = cmd.Run(args)
	passed = err == nil

	return passed, "", err
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}

// =============================================================================
// Integration Tests
// =============================================================================

func TestLintCopyright_CheckMode_CorrectHeaders(t *testing.T) {
	dir := setupTestDir(t, []testFile{
		{"star/config.yaml", starYAMLEnabled("Test Corp", "")},
		{"LICENSE", mitLicenseText},
		{"main.go", goFileCorrectMIT},
	})

	r, err := setupExtension(t, dir)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	passed, _, err := runLintCopyright(t, r, false, ".")
	if !passed {
		t.Errorf("expected check to pass, got error: %v", err)
	}
}

func TestLintCopyright_CheckMode_MissingHeaders(t *testing.T) {
	dir := setupTestDir(t, []testFile{
		{"star/config.yaml", starYAMLEnabled("Test Corp", "")},
		{"LICENSE", mitLicenseText},
		{"main.go", goFileNoHeader},
		{"util.go", goFileNoHeader},
	})

	r, err := setupExtension(t, dir)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	passed, _, err := runLintCopyright(t, r, false, ".")
	if passed {
		t.Error("expected check to fail for files with missing headers")
	}
	if err == nil {
		t.Error("expected error for missing headers")
	}
}

func TestLintCopyright_CheckMode_WrongLicense(t *testing.T) {
	dir := setupTestDir(t, []testFile{
		{"star/config.yaml", starYAMLEnabled("Test Corp", "MIT")},
		{"LICENSE", mitLicenseText},
		{"main.go", goFileWrongLicense}, // Has Apache-2.0
	})

	r, err := setupExtension(t, dir)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	passed, _, err := runLintCopyright(t, r, false, ".")
	if passed {
		t.Error("expected check to fail for wrong license")
	}
}

func TestLintCopyright_CheckMode_WrongHolder(t *testing.T) {
	dir := setupTestDir(t, []testFile{
		{"star/config.yaml", starYAMLEnabled("Test Corp", "MIT")},
		{"LICENSE", mitLicenseText},
		{"main.go", goFileWrongHolder}, // Has "Other Corp"
	})

	r, err := setupExtension(t, dir)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	passed, _, err := runLintCopyright(t, r, false, ".")
	if passed {
		t.Error("expected check to fail for wrong holder")
	}
}

func TestLintCopyright_FixMode_AddsHeaders(t *testing.T) {
	dir := setupTestDir(t, []testFile{
		{"star/config.yaml", starYAMLEnabled("Test Corp", "MIT")},
		{"LICENSE", mitLicenseText},
		{"main.go", goFileNoHeader},
		{"lib.star", starFileNoHeader},
	})

	r, err := setupExtension(t, dir)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Run fix
	passed, _, err := runLintCopyright(t, r, true, ".")
	if !passed {
		t.Errorf("fix failed: %v", err)
	}

	// Verify Go file was fixed
	goContent := readFile(t, filepath.Join(dir, "main.go"))
	if !strings.HasPrefix(goContent, "// SPDX-License-Identifier: MIT") {
		t.Errorf("Go file missing SPDX header, got:\n%s", goContent)
	}
	if !strings.Contains(goContent, "// Copyright Test Corp.") {
		t.Errorf("Go file missing copyright line, got:\n%s", goContent)
	}

	// Verify Starlark file was fixed
	starContent := readFile(t, filepath.Join(dir, "lib.star"))
	if !strings.HasPrefix(starContent, "# SPDX-License-Identifier: MIT") {
		t.Errorf("Starlark file missing SPDX header, got:\n%s", starContent)
	}
	if !strings.Contains(starContent, "# Copyright Test Corp.") {
		t.Errorf("Starlark file missing copyright line, got:\n%s", starContent)
	}
}

func TestLintCopyright_FixMode_ShebangHandling(t *testing.T) {
	dir := setupTestDir(t, []testFile{
		{"star/config.yaml", starYAMLEnabled("Test Corp", "MIT")},
		{"LICENSE", mitLicenseText},
		{"script.sh", shellFileNoHeader},
	})

	r, err := setupExtension(t, dir)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Run fix
	passed, _, err := runLintCopyright(t, r, true, ".")
	if !passed {
		t.Errorf("fix failed: %v", err)
	}

	// Verify shebang is preserved at line 1
	content := readFile(t, filepath.Join(dir, "script.sh"))
	lines := strings.Split(content, "\n")

	if len(lines) < 4 {
		t.Fatalf("expected at least 4 lines, got %d:\n%s", len(lines), content)
	}

	if !strings.HasPrefix(lines[0], "#!/bin/bash") {
		t.Errorf("shebang not preserved at line 1, got: %s", lines[0])
	}

	// SPDX should be after shebang (line 2 or 3 depending on blank line)
	hasSPDX := false
	for i, line := range lines[1:5] {
		if strings.HasPrefix(line, "# SPDX-License-Identifier:") {
			hasSPDX = true
			if i > 2 {
				t.Errorf("SPDX header too far from shebang, at line %d", i+2)
			}
			break
		}
	}
	if !hasSPDX {
		t.Errorf("SPDX header not found after shebang:\n%s", content)
	}
}

func TestLintCopyright_LicenseAutoDetection(t *testing.T) {
	tests := []struct {
		name            string
		licenseContent  string
		expectedLicense string
	}{
		{
			name:            "MIT license",
			licenseContent:  mitLicenseText,
			expectedLicense: "MIT",
		},
		{
			name:            "Apache license",
			licenseContent:  apacheLicenseText,
			expectedLicense: "Apache-2.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// File with correct header for expected license
			var goFile string
			if tt.expectedLicense == "MIT" {
				goFile = goFileCorrectMIT
			} else {
				goFile = goFileCorrectApache
			}

			dir := setupTestDir(t, []testFile{
				{"star/config.yaml", starYAMLEnabled("Test Corp", "auto")},
				{"LICENSE", tt.licenseContent},
				{"main.go", goFile},
			})

			r, err := setupExtension(t, dir)
			if err != nil {
				t.Fatalf("setup: %v", err)
			}

			passed, _, err := runLintCopyright(t, r, false, ".")
			if !passed {
				t.Errorf("expected check to pass with auto-detected %s license: %v", tt.expectedLicense, err)
			}
		})
	}
}

func TestLintCopyright_ExclusionPatterns(t *testing.T) {
	dir := setupTestDir(t, []testFile{
		{"star/config.yaml", starYAMLWithExcludes("Test Corp", []string{"vendor/**"})},
		{"LICENSE", mitLicenseText},
		{"main.go", goFileCorrectMIT},
		{"vendor/dep.go", goFileNoHeader}, // Should be excluded
	})

	r, err := setupExtension(t, dir)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Should pass because vendor is excluded
	passed, _, err := runLintCopyright(t, r, false, ".")
	if !passed {
		t.Errorf("expected check to pass with vendor excluded: %v", err)
	}
}

func TestLintCopyright_CheckMode_ShellScriptCorrect(t *testing.T) {
	dir := setupTestDir(t, []testFile{
		{"star/config.yaml", starYAMLEnabled("Test Corp", "MIT")},
		{"LICENSE", mitLicenseText},
		{"script.sh", shellFileCorrect},
	})

	r, err := setupExtension(t, dir)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	passed, _, err := runLintCopyright(t, r, false, ".")
	if !passed {
		t.Errorf("expected check to pass for correct shell script: %v", err)
	}
}

func TestLintCopyright_FixMode_AlreadyCorrect(t *testing.T) {
	dir := setupTestDir(t, []testFile{
		{"star/config.yaml", starYAMLEnabled("Test Corp", "MIT")},
		{"LICENSE", mitLicenseText},
		{"main.go", goFileCorrectMIT},
	})

	r, err := setupExtension(t, dir)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Run fix on already-correct files
	passed, _, err := runLintCopyright(t, r, true, ".")
	if !passed {
		t.Errorf("fix failed on already-correct files: %v", err)
	}

	// Verify file wasn't mangled
	content := readFile(t, filepath.Join(dir, "main.go"))
	if content != goFileCorrectMIT {
		t.Errorf("file was modified when it shouldn't have been:\ngot:\n%s\nwant:\n%s", content, goFileCorrectMIT)
	}
}
