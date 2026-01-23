// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package exec

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
	"filippo.io/age/armor"
	"github.com/NobleFactor/devlore-cli/internal/writ/segment"
	"github.com/NobleFactor/devlore-cli/internal/writ/tree"
)

func TestExecutorLink(t *testing.T) {
	// Create temp directories
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	// Create source file
	sourceFile := filepath.Join(sourceDir, ".bashrc")
	if err := os.WriteFile(sourceFile, []byte("# bashrc"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create tree with link operation
	deployTree := &tree.Tree{
		SourceRoot: sourceDir,
		TargetRoot: targetDir,
		Nodes: []*tree.Node{
			{
				Source:     sourceFile,
				Target:     filepath.Join(targetDir, ".bashrc"),
				RelSource:  ".bashrc",
				RelTarget:  ".bashrc",
				Operations: tree.Operations{tree.OpLink},
			},
		},
	}

	// Execute
	exec := &Executor{Output: &bytes.Buffer{}}
	results, err := exec.Execute(deployTree)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(results) != 1 || !results[0].Success {
		t.Errorf("expected 1 successful result, got %d", len(results))
	}

	// Verify symlink was created
	target := filepath.Join(targetDir, ".bashrc")
	info, err := os.Lstat(target)
	if err != nil {
		t.Fatalf("target not created: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("target is not a symlink")
	}

	// Verify symlink points to source
	link, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("readlink failed: %v", err)
	}
	if link != sourceFile {
		t.Errorf("symlink points to %q, want %q", link, sourceFile)
	}
}

func TestExecutorCopy(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	// Create source file
	sourceFile := filepath.Join(sourceDir, "config")
	content := []byte("test content")
	if err := os.WriteFile(sourceFile, content, 0644); err != nil {
		t.Fatal(err)
	}

	deployTree := &tree.Tree{
		SourceRoot: sourceDir,
		TargetRoot: targetDir,
		Nodes: []*tree.Node{
			{
				Source:     sourceFile,
				Target:     filepath.Join(targetDir, "config"),
				RelSource:  "config",
				RelTarget:  "config",
				Operations: tree.Operations{tree.OpCopy},
			},
		},
	}

	exec := &Executor{Output: &bytes.Buffer{}}
	results, err := exec.Execute(deployTree)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !results[0].Success {
		t.Errorf("expected success, got error: %v", results[0].Error)
	}

	// Verify file was copied (not symlink)
	target := filepath.Join(targetDir, "config")
	info, err := os.Lstat(target)
	if err != nil {
		t.Fatalf("target not created: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("target should not be a symlink")
	}

	// Verify content
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target failed: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("content = %q, want %q", got, content)
	}
}

func TestExecutorExpand(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	// Create template file
	sourceFile := filepath.Join(sourceDir, "config.template")
	template := []byte("Hello, {{.Name}}!")
	if err := os.WriteFile(sourceFile, template, 0644); err != nil {
		t.Fatal(err)
	}

	deployTree := &tree.Tree{
		SourceRoot: sourceDir,
		TargetRoot: targetDir,
		Nodes: []*tree.Node{
			{
				Source:     sourceFile,
				Target:     filepath.Join(targetDir, "config"),
				RelSource:  "config.template",
				RelTarget:  "config",
				Operations: tree.Operations{tree.OpExpand, tree.OpCopy},
			},
		},
	}

	exec := &Executor{
		Output:       &bytes.Buffer{},
		TemplateData: map[string]any{"Name": "World"},
	}
	results, err := exec.Execute(deployTree)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !results[0].Success {
		t.Errorf("expected success, got error: %v", results[0].Error)
	}

	// Verify expanded content
	target := filepath.Join(targetDir, "config")
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target failed: %v", err)
	}
	if string(got) != "Hello, World!" {
		t.Errorf("content = %q, want %q", got, "Hello, World!")
	}
}

func TestExecutorDecrypt(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	// Generate test identity
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}

	// Encrypt test content
	plaintext := []byte("secret content")
	var encrypted bytes.Buffer
	armorWriter := armor.NewWriter(&encrypted)
	encWriter, err := age.Encrypt(armorWriter, identity.Recipient())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := encWriter.Write(plaintext); err != nil {
		t.Fatal(err)
	}
	if err := encWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := armorWriter.Close(); err != nil {
		t.Fatal(err)
	}

	// Create encrypted source file
	sourceFile := filepath.Join(sourceDir, "secret.age")
	if err := os.WriteFile(sourceFile, encrypted.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	deployTree := &tree.Tree{
		SourceRoot: sourceDir,
		TargetRoot: targetDir,
		Nodes: []*tree.Node{
			{
				Source:     sourceFile,
				Target:     filepath.Join(targetDir, "secret"),
				RelSource:  "secret.age",
				RelTarget:  "secret",
				Operations: tree.Operations{tree.OpDecrypt, tree.OpCopy},
				Mode:       0600,
			},
		},
	}

	exec := &Executor{
		Output:     &bytes.Buffer{},
		Identities: []age.Identity{identity},
	}
	results, err := exec.Execute(deployTree)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !results[0].Success {
		t.Errorf("expected success, got error: %v", results[0].Error)
	}

	// Verify decrypted content
	target := filepath.Join(targetDir, "secret")
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target failed: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("content = %q, want %q", got, plaintext)
	}

	// Verify file mode
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("mode = %o, want %o", info.Mode().Perm(), 0600)
	}
}

func TestExecutorDecryptAndExpand(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	// Generate test identity
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}

	// Template content to encrypt
	template := []byte("Secret: {{.Secret}}")

	// Encrypt template
	var encrypted bytes.Buffer
	armorWriter := armor.NewWriter(&encrypted)
	encWriter, err := age.Encrypt(armorWriter, identity.Recipient())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := encWriter.Write(template); err != nil {
		t.Fatal(err)
	}
	if err := encWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := armorWriter.Close(); err != nil {
		t.Fatal(err)
	}

	// Create encrypted template source file
	sourceFile := filepath.Join(sourceDir, "config.template.age")
	if err := os.WriteFile(sourceFile, encrypted.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	deployTree := &tree.Tree{
		SourceRoot: sourceDir,
		TargetRoot: targetDir,
		Nodes: []*tree.Node{
			{
				Source:     sourceFile,
				Target:     filepath.Join(targetDir, "config"),
				RelSource:  "config.template.age",
				RelTarget:  "config",
				Operations: tree.Operations{tree.OpDecrypt, tree.OpExpand, tree.OpCopy},
				Mode:       0600,
			},
		},
	}

	exec := &Executor{
		Output:       &bytes.Buffer{},
		Identities:   []age.Identity{identity},
		TemplateData: map[string]any{"Secret": "password123"},
	}
	results, err := exec.Execute(deployTree)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !results[0].Success {
		t.Errorf("expected success, got error: %v", results[0].Error)
	}

	// Verify decrypted and expanded content
	target := filepath.Join(targetDir, "config")
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target failed: %v", err)
	}
	if string(got) != "Secret: password123" {
		t.Errorf("content = %q, want %q", got, "Secret: password123")
	}
}

func TestExecutorDryRun(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	// Create source file
	sourceFile := filepath.Join(sourceDir, ".bashrc")
	if err := os.WriteFile(sourceFile, []byte("# bashrc"), 0644); err != nil {
		t.Fatal(err)
	}

	deployTree := &tree.Tree{
		SourceRoot: sourceDir,
		TargetRoot: targetDir,
		Projects:   []string{"all"},
		MatchedDirs: []segment.MatchResult{
			{Path: filepath.Join(sourceDir, "all"), Project: "all"},
		},
		Nodes: []*tree.Node{
			{
				Source:     sourceFile,
				Target:     filepath.Join(targetDir, ".bashrc"),
				RelSource:  ".bashrc",
				RelTarget:  ".bashrc",
				Operations: tree.Operations{tree.OpLink},
				Project:    "all",
			},
		},
	}

	var output bytes.Buffer
	exec := &Executor{DryRun: true, Output: &output}
	results, err := exec.Execute(deployTree)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !results[0].Success {
		t.Errorf("expected success, got error: %v", results[0].Error)
	}

	// Verify target was NOT created
	target := filepath.Join(targetDir, ".bashrc")
	if _, err := os.Lstat(target); err == nil {
		t.Error("target should not exist in dry-run mode")
	}

	// Verify JSON output
	var dryRun DryRunOutput
	if err := json.Unmarshal(output.Bytes(), &dryRun); err != nil {
		t.Fatalf("failed to parse dry-run JSON: %v\nOutput: %s", err, output.String())
	}

	if dryRun.SourceRoot != sourceDir {
		t.Errorf("source_root = %q, want %q", dryRun.SourceRoot, sourceDir)
	}
	if dryRun.TargetRoot != targetDir {
		t.Errorf("target_root = %q, want %q", dryRun.TargetRoot, targetDir)
	}
	if len(dryRun.Operations) != 1 {
		t.Errorf("expected 1 operation, got %d", len(dryRun.Operations))
	}
	if len(dryRun.Operations) > 0 && dryRun.Operations[0].Operations[0] != "link" {
		t.Errorf("expected 'link' operation, got %v", dryRun.Operations[0].Operations)
	}
}

func TestExecutorConflictResolution(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	// Create source file
	sourceFile := filepath.Join(sourceDir, "config")
	if err := os.WriteFile(sourceFile, []byte("new content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create existing target
	targetFile := filepath.Join(targetDir, "config")
	if err := os.WriteFile(targetFile, []byte("old content"), 0644); err != nil {
		t.Fatal(err)
	}

	deployTree := &tree.Tree{
		SourceRoot: sourceDir,
		TargetRoot: targetDir,
		Nodes: []*tree.Node{
			{
				Source:     sourceFile,
				Target:     targetFile,
				RelSource:  "config",
				RelTarget:  "config",
				Operations: tree.Operations{tree.OpCopy},
			},
		},
	}

	// With ResolutionStop (default), should fail with error
	exec := &Executor{Output: &bytes.Buffer{}, ConflictResolution: ResolutionStop}
	_, err := exec.Execute(deployTree)
	if err == nil {
		t.Error("expected error with ResolutionStop when target exists")
	}

	// With ResolutionOverwrite, should succeed
	exec.ConflictResolution = ResolutionOverwrite
	results, err := exec.Execute(deployTree)
	if err != nil {
		t.Fatalf("Execute with overwrite failed: %v", err)
	}
	if len(results) == 0 || !results[0].Success {
		t.Errorf("expected success with overwrite")
	}

	// Verify new content
	got, _ := os.ReadFile(targetFile)
	if string(got) != "new content" {
		t.Errorf("content = %q, want %q", got, "new content")
	}

	// Test backup resolution
	// Recreate old content
	if err := os.WriteFile(targetFile, []byte("old content again"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourceFile, []byte("newer content"), 0644); err != nil {
		t.Fatal(err)
	}

	exec.ConflictResolution = ResolutionBackup
	exec.BackupSuffix = ".backup"
	results, err = exec.Execute(deployTree)
	if err != nil {
		t.Fatalf("Execute with backup failed: %v", err)
	}
	if len(results) == 0 || !results[0].Success {
		t.Errorf("expected success with backup")
	}

	// Verify new content deployed
	got, _ = os.ReadFile(targetFile)
	if string(got) != "newer content" {
		t.Errorf("content = %q, want %q", got, "newer content")
	}

	// Verify backup exists (with timestamp suffix)
	backups, _ := filepath.Glob(targetFile + ".backup.*")
	if len(backups) == 0 {
		t.Error("expected backup file to be created")
	} else {
		backupContent, _ := os.ReadFile(backups[0])
		if string(backupContent) != "old content again" {
			t.Errorf("backup content = %q, want %q", backupContent, "old content again")
		}
	}

	// Test skip resolution
	// Recreate target with different content
	if err := os.WriteFile(targetFile, []byte("should not change"), 0644); err != nil {
		t.Fatal(err)
	}

	exec.ConflictResolution = ResolutionSkip
	results, err = exec.Execute(deployTree)
	if err != nil {
		t.Fatalf("Execute with skip failed: %v", err)
	}
	if len(results) == 0 || !results[0].Skipped {
		t.Errorf("expected skipped result")
	}

	// Verify content unchanged
	got, _ = os.ReadFile(targetFile)
	if string(got) != "should not change" {
		t.Errorf("content changed with skip, got %q", got)
	}
}

func TestExecutorCreatesParentDirs(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	// Create source file
	sourceFile := filepath.Join(sourceDir, "config")
	if err := os.WriteFile(sourceFile, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Target is in nested directory that doesn't exist
	targetFile := filepath.Join(targetDir, ".config", "app", "config")

	deployTree := &tree.Tree{
		SourceRoot: sourceDir,
		TargetRoot: targetDir,
		Nodes: []*tree.Node{
			{
				Source:     sourceFile,
				Target:     targetFile,
				RelSource:  "config",
				RelTarget:  ".config/app/config",
				Operations: tree.Operations{tree.OpLink},
			},
		},
	}

	exec := &Executor{Output: &bytes.Buffer{}}
	results, err := exec.Execute(deployTree)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !results[0].Success {
		t.Errorf("expected success, got error: %v", results[0].Error)
	}

	// Verify symlink was created in nested directory
	if _, err := os.Lstat(targetFile); err != nil {
		t.Errorf("target not created: %v", err)
	}
}

func TestExecutorIdempotentSymlink(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	sourceFile := filepath.Join(sourceDir, "config")
	if err := os.WriteFile(sourceFile, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	targetFile := filepath.Join(targetDir, "config")

	deployTree := &tree.Tree{
		SourceRoot: sourceDir,
		TargetRoot: targetDir,
		Nodes: []*tree.Node{
			{
				Source:     sourceFile,
				Target:     targetFile,
				RelSource:  "config",
				RelTarget:  "config",
				Operations: tree.Operations{tree.OpLink},
			},
		},
	}

	exec := &Executor{Output: &bytes.Buffer{}}

	// First execution
	results, err := exec.Execute(deployTree)
	if err != nil {
		t.Fatalf("first Execute failed: %v", err)
	}
	if !results[0].Success {
		t.Errorf("first execution failed: %v", results[0].Error)
	}

	// Second execution should succeed (idempotent)
	results, err = exec.Execute(deployTree)
	if err != nil {
		t.Fatalf("second Execute failed: %v", err)
	}
	if !results[0].Success {
		t.Errorf("second execution failed: %v", results[0].Error)
	}
}

func TestExecutorDelegate(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	// Create packages.manifest file
	sourceFile := filepath.Join(sourceDir, "packages.manifest")
	if err := os.WriteFile(sourceFile, []byte("brew install jq"), 0644); err != nil {
		t.Fatal(err)
	}

	deployTree := &tree.Tree{
		SourceRoot: sourceDir,
		TargetRoot: targetDir,
		Nodes: []*tree.Node{
			{
				Source:     sourceFile,
				Target:     filepath.Join(targetDir, "packages.manifest"),
				RelSource:  "packages.manifest",
				RelTarget:  "packages.manifest",
				Operations: tree.Operations{tree.OpDelegate},
				Project:    "all",
			},
		},
	}

	exec := &Executor{Output: &bytes.Buffer{}}
	results, err := exec.Execute(deployTree)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !results[0].Success {
		t.Errorf("expected success, got error: %v", results[0].Error)
	}

	// Verify target was NOT created (delegate doesn't write files)
	target := filepath.Join(targetDir, "packages.manifest")
	if _, err := os.Lstat(target); err == nil {
		t.Error("delegate operation should not create target file")
	}

	// Verify we can get delegated nodes
	delegated := DelegatedNodes(results)
	if len(delegated) != 1 {
		t.Errorf("expected 1 delegated node, got %d", len(delegated))
	}
	if len(delegated) > 0 && delegated[0].RelSource != "packages.manifest" {
		t.Errorf("delegated node = %q, want %q", delegated[0].RelSource, "packages.manifest")
	}
}

func TestExecutorDryRunWithDelegate(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	// Create files
	bashrcFile := filepath.Join(sourceDir, ".bashrc")
	if err := os.WriteFile(bashrcFile, []byte("# bashrc"), 0644); err != nil {
		t.Fatal(err)
	}
	manifestFile := filepath.Join(sourceDir, "packages.manifest")
	if err := os.WriteFile(manifestFile, []byte("brew install jq"), 0644); err != nil {
		t.Fatal(err)
	}

	deployTree := &tree.Tree{
		SourceRoot: sourceDir,
		TargetRoot: targetDir,
		Projects:   []string{"all"},
		MatchedDirs: []segment.MatchResult{
			{Path: filepath.Join(sourceDir, "all"), Project: "all"},
		},
		Nodes: []*tree.Node{
			{
				Source:     bashrcFile,
				Target:     filepath.Join(targetDir, ".bashrc"),
				RelSource:  ".bashrc",
				RelTarget:  ".bashrc",
				Operations: tree.Operations{tree.OpLink},
				Project:    "all",
			},
			{
				Source:     manifestFile,
				Target:     filepath.Join(targetDir, "packages.manifest"),
				RelSource:  "packages.manifest",
				RelTarget:  "packages.manifest",
				Operations: tree.Operations{tree.OpDelegate},
				Project:    "all",
			},
		},
	}

	var output bytes.Buffer
	exec := &Executor{DryRun: true, Output: &output}
	_, err := exec.Execute(deployTree)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Parse JSON output
	var dryRun DryRunOutput
	if err := json.Unmarshal(output.Bytes(), &dryRun); err != nil {
		t.Fatalf("failed to parse dry-run JSON: %v\nOutput: %s", err, output.String())
	}

	// Verify operations and delegated are separate
	if len(dryRun.Operations) != 1 {
		t.Errorf("expected 1 operation, got %d", len(dryRun.Operations))
	}
	if len(dryRun.Delegated) != 1 {
		t.Errorf("expected 1 delegated, got %d", len(dryRun.Delegated))
	}

	// Verify delegated item
	if len(dryRun.Delegated) > 0 {
		if dryRun.Delegated[0].RelSource != "packages.manifest" {
			t.Errorf("delegated[0].rel_source = %q, want %q", dryRun.Delegated[0].RelSource, "packages.manifest")
		}
		if dryRun.Delegated[0].Operations[0] != "delegate" {
			t.Errorf("delegated[0].operations = %v, want [delegate]", dryRun.Delegated[0].Operations)
		}
	}
}

func TestExecutorBreadthFirstOrder(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	// Create source files at different depths
	files := []string{
		".config/app/nested/deep.conf", // depth 3
		".bashrc",                       // depth 0
		".config/app/config",            // depth 2
		".ssh/config",                   // depth 1
	}

	for _, f := range files {
		path := filepath.Join(sourceDir, f)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Create nodes in non-sorted order
	deployTree := &tree.Tree{
		SourceRoot: sourceDir,
		TargetRoot: targetDir,
		Nodes: []*tree.Node{
			{
				Source:     filepath.Join(sourceDir, ".config/app/nested/deep.conf"),
				Target:     filepath.Join(targetDir, ".config/app/nested/deep.conf"),
				RelSource:  ".config/app/nested/deep.conf",
				RelTarget:  ".config/app/nested/deep.conf",
				Operations: tree.Operations{tree.OpLink},
			},
			{
				Source:     filepath.Join(sourceDir, ".bashrc"),
				Target:     filepath.Join(targetDir, ".bashrc"),
				RelSource:  ".bashrc",
				RelTarget:  ".bashrc",
				Operations: tree.Operations{tree.OpLink},
			},
			{
				Source:     filepath.Join(sourceDir, ".config/app/config"),
				Target:     filepath.Join(targetDir, ".config/app/config"),
				RelSource:  ".config/app/config",
				RelTarget:  ".config/app/config",
				Operations: tree.Operations{tree.OpLink},
			},
			{
				Source:     filepath.Join(sourceDir, ".ssh/config"),
				Target:     filepath.Join(targetDir, ".ssh/config"),
				RelSource:  ".ssh/config",
				RelTarget:  ".ssh/config",
				Operations: tree.Operations{tree.OpLink},
			},
		},
	}

	// Use dry-run to capture order
	var output bytes.Buffer
	exec := &Executor{DryRun: true, Output: &output}
	_, err := exec.Execute(deployTree)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Parse JSON output
	var dryRun DryRunOutput
	if err := json.Unmarshal(output.Bytes(), &dryRun); err != nil {
		t.Fatalf("failed to parse dry-run JSON: %v", err)
	}

	// Verify breadth-first order (by depth)
	expectedOrder := []string{
		".bashrc",                       // depth 0
		".ssh/config",                   // depth 1
		".config/app/config",            // depth 2
		".config/app/nested/deep.conf",  // depth 3
	}

	if len(dryRun.Operations) != len(expectedOrder) {
		t.Fatalf("expected %d operations, got %d", len(expectedOrder), len(dryRun.Operations))
	}

	for i, expected := range expectedOrder {
		if dryRun.Operations[i].RelTarget != expected {
			t.Errorf("operations[%d].rel_target = %q, want %q", i, dryRun.Operations[i].RelTarget, expected)
		}
	}

	// Verify depth values in output
	expectedDepths := []int{0, 1, 2, 3}
	for i, expected := range expectedDepths {
		if dryRun.Operations[i].Depth != expected {
			t.Errorf("operations[%d].depth = %d, want %d", i, dryRun.Operations[i].Depth, expected)
		}
	}
}

func TestExecutorPreflight(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	// Create source files
	files := []string{"file1", "file2", "file3", "file4"}
	for _, f := range files {
		path := filepath.Join(sourceDir, f)
		if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Create various conflicts at target:
	// file1 - no conflict (doesn't exist)
	// file2 - regular file conflict
	if err := os.WriteFile(filepath.Join(targetDir, "file2"), []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}
	// file3 - foreign symlink conflict
	if err := os.Symlink("/some/other/path", filepath.Join(targetDir, "file3")); err != nil {
		t.Fatal(err)
	}
	// file4 - our symlink (already deployed)
	if err := os.Symlink(filepath.Join(sourceDir, "file4"), filepath.Join(targetDir, "file4")); err != nil {
		t.Fatal(err)
	}

	deployTree := &tree.Tree{
		SourceRoot: sourceDir,
		TargetRoot: targetDir,
		Nodes: []*tree.Node{
			{
				Source:     filepath.Join(sourceDir, "file1"),
				Target:     filepath.Join(targetDir, "file1"),
				RelSource:  "file1",
				RelTarget:  "file1",
				Operations: tree.Operations{tree.OpLink},
			},
			{
				Source:     filepath.Join(sourceDir, "file2"),
				Target:     filepath.Join(targetDir, "file2"),
				RelSource:  "file2",
				RelTarget:  "file2",
				Operations: tree.Operations{tree.OpLink},
			},
			{
				Source:     filepath.Join(sourceDir, "file3"),
				Target:     filepath.Join(targetDir, "file3"),
				RelSource:  "file3",
				RelTarget:  "file3",
				Operations: tree.Operations{tree.OpLink},
			},
			{
				Source:     filepath.Join(sourceDir, "file4"),
				Target:     filepath.Join(targetDir, "file4"),
				RelSource:  "file4",
				RelTarget:  "file4",
				Operations: tree.Operations{tree.OpLink},
			},
		},
	}

	exec := &Executor{Output: &bytes.Buffer{}}
	preflight := exec.Preflight(deployTree)

	// Check ready (no conflict)
	if len(preflight.Ready) != 1 {
		t.Errorf("expected 1 ready, got %d", len(preflight.Ready))
	} else if preflight.Ready[0].RelTarget != "file1" {
		t.Errorf("ready[0] = %q, want file1", preflight.Ready[0].RelTarget)
	}

	// Check conflicts
	if len(preflight.Conflicts) != 2 {
		t.Errorf("expected 2 conflicts, got %d", len(preflight.Conflicts))
	}

	// Verify conflict types
	var hasRegular, hasForeign bool
	for _, c := range preflight.Conflicts {
		switch c.Node.RelTarget {
		case "file2":
			if c.Type != ConflictRegularFile {
				t.Errorf("file2 conflict type = %v, want ConflictRegularFile", c.Type)
			}
			hasRegular = true
		case "file3":
			if c.Type != ConflictForeignSymlink {
				t.Errorf("file3 conflict type = %v, want ConflictForeignSymlink", c.Type)
			}
			hasForeign = true
		}
	}
	if !hasRegular {
		t.Error("missing regular file conflict")
	}
	if !hasForeign {
		t.Error("missing foreign symlink conflict")
	}

	// Check already deployed
	if len(preflight.AlreadyDone) != 1 {
		t.Errorf("expected 1 already done, got %d", len(preflight.AlreadyDone))
	} else {
		if preflight.AlreadyDone[0].Node.RelTarget != "file4" {
			t.Errorf("already_done[0] = %q, want file4", preflight.AlreadyDone[0].Node.RelTarget)
		}
		if preflight.AlreadyDone[0].Type != ConflictOurSymlink {
			t.Errorf("already_done[0] type = %v, want ConflictOurSymlink", preflight.AlreadyDone[0].Type)
		}
	}
}

func TestBuiltinTemplateVariables(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	// Template using various builtin variables
	templateContent := `OS={{.OS}}
ARCH={{.ARCH}}
Username={{.Username}}
Home={{.Home}}
Hostname={{.Hostname}}
ConfigDir={{.ConfigDir}}
Project={{.Project}}
CustomVar={{.CustomVar}}
EnvTest={{call .Env "HOME"}}`

	sourceFile := filepath.Join(sourceDir, "test.template")
	if err := os.WriteFile(sourceFile, []byte(templateContent), 0644); err != nil {
		t.Fatal(err)
	}

	deployTree := &tree.Tree{
		SourceRoot: sourceDir,
		TargetRoot: targetDir,
		Nodes: []*tree.Node{
			{
				Source:     sourceFile,
				Target:     filepath.Join(targetDir, "test"),
				RelSource:  "test.template",
				RelTarget:  "test",
				Project:    "myproject",
				Operations: tree.Operations{tree.OpExpand, tree.OpCopy},
			},
		},
	}

	exec := &Executor{
		Output: &bytes.Buffer{},
		Segments: map[string]string{
			"OS":     "Darwin",
			"ARCH":   "arm64",
			"DISTRO": "",
		},
		TemplateData: map[string]any{
			"CustomVar": "custom-value",
		},
	}

	results, err := exec.Execute(deployTree)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !results[0].Success {
		t.Errorf("expected success, got error: %v", results[0].Error)
	}

	// Read and verify expanded content
	got, err := os.ReadFile(filepath.Join(targetDir, "test"))
	if err != nil {
		t.Fatalf("read target failed: %v", err)
	}

	content := string(got)

	// Check platform variables from segments
	if !bytes.Contains(got, []byte("OS=Darwin")) {
		t.Errorf("expected OS=Darwin in output, got: %s", content)
	}
	if !bytes.Contains(got, []byte("ARCH=arm64")) {
		t.Errorf("expected ARCH=arm64 in output, got: %s", content)
	}

	// Check environment variables
	username := os.Getenv("USER")
	if !bytes.Contains(got, []byte("Username="+username)) {
		t.Errorf("expected Username=%s in output, got: %s", username, content)
	}

	home := os.Getenv("HOME")
	if !bytes.Contains(got, []byte("Home="+home)) {
		t.Errorf("expected Home=%s in output, got: %s", home, content)
	}

	// Check Env function
	if !bytes.Contains(got, []byte("EnvTest="+home)) {
		t.Errorf("expected EnvTest=%s in output, got: %s", home, content)
	}

	// Check node-specific variable
	if !bytes.Contains(got, []byte("Project=myproject")) {
		t.Errorf("expected Project=myproject in output, got: %s", content)
	}

	// Check user-defined variable
	if !bytes.Contains(got, []byte("CustomVar=custom-value")) {
		t.Errorf("expected CustomVar=custom-value in output, got: %s", content)
	}
}

func TestBuiltinTemplateVariablesWithoutSegments(t *testing.T) {
	// Test that builtins work even without explicit Segments (fallback to runtime)
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	templateContent := `OS={{.OS}}
Username={{.Username}}`

	sourceFile := filepath.Join(sourceDir, "test.template")
	if err := os.WriteFile(sourceFile, []byte(templateContent), 0644); err != nil {
		t.Fatal(err)
	}

	deployTree := &tree.Tree{
		SourceRoot: sourceDir,
		TargetRoot: targetDir,
		Nodes: []*tree.Node{
			{
				Source:     sourceFile,
				Target:     filepath.Join(targetDir, "test"),
				RelSource:  "test.template",
				RelTarget:  "test",
				Operations: tree.Operations{tree.OpExpand, tree.OpCopy},
			},
		},
	}

	exec := &Executor{
		Output: &bytes.Buffer{},
		// No Segments set - should use runtime fallback
	}

	results, err := exec.Execute(deployTree)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !results[0].Success {
		t.Errorf("expected success, got error: %v", results[0].Error)
	}

	got, err := os.ReadFile(filepath.Join(targetDir, "test"))
	if err != nil {
		t.Fatalf("read target failed: %v", err)
	}

	// OS should be set from runtime (Darwin, Linux, or Windows)
	if !bytes.Contains(got, []byte("OS=")) {
		t.Errorf("expected OS= in output, got: %s", got)
	}

	// Username should be set
	username := os.Getenv("USER")
	if !bytes.Contains(got, []byte("Username="+username)) {
		t.Errorf("expected Username=%s in output, got: %s", username, got)
	}
}
