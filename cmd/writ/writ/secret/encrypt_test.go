// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package secret

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"

	"github.com/NobleFactor/devlore-cli/cmd/internal/devlore"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/inventory"
	"github.com/NobleFactor/devlore-cli/pkg/sops"
)

// setupLayer sandboxes the XDG triplet, creates a layer working tree, and registers it as `personal`.
//
// Parameters:
//   - `t`: the test context.
//   - `withConfig`: whether the layer root receives a catch-all `.sops.yaml` for the generated identity.
//
// Returns:
//   - `string`: the canonical layer root.
//   - `*age.X25519Identity`: the identity whose recipient governs the layer.
func setupLayer(t *testing.T, withConfig bool) (string, *age.X25519Identity) {
	t.Helper()

	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(base, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(base, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(base, "config"))

	tree := filepath.Join(base, "personal")
	mustMkdirAll(t, filepath.Join(tree, "Home", "demo"))

	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}

	if withConfig {
		config := "creation_rules:\n  - path_regex: .*\n    age: " + identity.Recipient().String() + "\n"
		mustWriteFile(t, filepath.Join(tree, ".sops.yaml"), config)
	}

	layers := filepath.Join(base, "data", "devlore", "writ", "layers")
	mustMkdirAll(t, layers)
	if err := os.Symlink(tree, filepath.Join(layers, "personal")); err != nil {
		t.Fatalf("register layer: %v", err)
	}

	canonical, err := filepath.EvalSymlinks(tree)
	if err != nil {
		t.Fatalf("canonicalize tree: %v", err)
	}
	return canonical, identity
}

func TestExecuteEncrypt_RoundTrip(t *testing.T) {

	tree, identity := setupLayer(t, true)
	source := filepath.Join(tree, "Home", "demo", "app.yaml")
	mustWriteFile(t, source, "credential: hello world\n")

	if _, err := ExecuteEncrypt(context.Background(), &EncryptConfig{Files: []string{source}}); err != nil {
		t.Fatalf("ExecuteEncrypt: %v", err)
	}

	plaintext, err := os.ReadFile(source)
	if err != nil || string(plaintext) != "credential: hello world\n" {
		t.Fatalf("plaintext source altered: %q, %v", plaintext, err)
	}

	ciphertext, err := os.ReadFile(source + ".sops")
	if err != nil {
		t.Fatalf("destination missing: %v", err)
	}
	if strings.Contains(string(ciphertext), "hello world") {
		t.Fatalf("destination carries plaintext")
	}

	t.Setenv("SOPS_AGE_KEY", identity.String())
	decrypted, err := (&sops.Client{}).Decrypt(ciphertext, source+".sops")
	if err != nil {
		t.Fatalf("round-trip decrypt: %v", err)
	}
	if string(decrypted) != "credential: hello world\n" {
		t.Fatalf("round-trip mismatch: %q", decrypted)
	}

	graphs, err := os.ReadDir(devlore.StatePath("graphs"))
	if err != nil || len(graphs) != 1 {
		t.Fatalf("expected exactly one persisted graph, got %d, %v", len(graphs), err)
	}
}

func TestExecuteEncrypt_RefusesExistingDestination(t *testing.T) {

	tree, _ := setupLayer(t, true)
	source := filepath.Join(tree, "Home", "demo", "app.yaml")
	mustWriteFile(t, source, "credential: hello\n")
	mustWriteFile(t, source+".sops", "occupied")

	_, err := ExecuteEncrypt(context.Background(), &EncryptConfig{Files: []string{source}})
	if err == nil || !strings.Contains(err.Error(), "never overwrites") {
		t.Fatalf("expected overwrite refusal, got %v", err)
	}

	occupant, readErr := os.ReadFile(source + ".sops")
	if readErr != nil || string(occupant) != "occupied" {
		t.Fatalf("existing destination altered: %q, %v", occupant, readErr)
	}
}

func TestExecuteEncrypt_RefusesOutsideLayer(t *testing.T) {

	_, _ = setupLayer(t, true)
	outside := filepath.Join(t.TempDir(), "loose.yaml")
	mustWriteFile(t, outside, "credential: hello\n")

	_, err := ExecuteEncrypt(context.Background(), &EncryptConfig{Files: []string{outside}})
	if err == nil || !strings.Contains(err.Error(), "writ repo add") {
		t.Fatalf("expected containment refusal naming writ repo add, got %v", err)
	}
}

func TestExecuteEncrypt_NoGoverningRule(t *testing.T) {

	tree, _ := setupLayer(t, false)
	source := filepath.Join(tree, "Home", "demo", "app.yaml")
	mustWriteFile(t, source, "credential: hello\n")

	_, err := ExecuteEncrypt(context.Background(), &EncryptConfig{Files: []string{source}})
	if err == nil || !strings.Contains(err.Error(), "no .sops.yaml creation rule governs") {
		t.Fatalf("expected the resolver's no-rule error verbatim, got %v", err)
	}
}

func TestExecuteEncrypt_MultipleFilesOneGraph(t *testing.T) {

	tree, _ := setupLayer(t, true)
	first := filepath.Join(tree, "Home", "demo", "first.yaml")
	second := filepath.Join(tree, "Home", "demo", "second.yaml")
	mustWriteFile(t, first, "a: 1\n")
	mustWriteFile(t, second, "b: 2\n")

	if _, err := ExecuteEncrypt(context.Background(), &EncryptConfig{Files: []string{first, second}}); err != nil {
		t.Fatalf("ExecuteEncrypt: %v", err)
	}

	for _, source := range []string{first, second} {
		if _, err := os.Stat(source + ".sops"); err != nil {
			t.Fatalf("destination missing for %s: %v", source, err)
		}
	}

	graphs, err := os.ReadDir(devlore.StatePath("graphs"))
	if err != nil || len(graphs) != 1 {
		t.Fatalf("expected one graph for one layer, got %d, %v", len(graphs), err)
	}
}

func TestExecuteEncrypt_DryRunWritesNothing(t *testing.T) {

	tree, _ := setupLayer(t, true)
	source := filepath.Join(tree, "Home", "demo", "app.yaml")
	mustWriteFile(t, source, "credential: hello\n")

	graphs, err := ExecuteEncrypt(context.Background(), &EncryptConfig{Files: []string{source}, DryRun: true})
	if err != nil {
		t.Fatalf("ExecuteEncrypt dry-run: %v", err)
	}
	if len(graphs) == 0 {
		t.Fatal("ExecuteEncrypt dry-run returned no graphs; the plan is the result")
	}

	if _, err := os.Lstat(source + ".sops"); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote a destination: %v", err)
	}
	if _, err := os.ReadDir(devlore.StatePath("graphs")); !os.IsNotExist(err) {
		t.Fatalf("dry-run persisted to the store: %v", err)
	}
}

// mustMkdirAll creates the directory tree or fails the test.
//
// Parameters:
//   - `t`: the test context.
//   - `dir`: the directory path to create.
func mustMkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
}

// mustWriteFile writes content or fails the test.
//
// Parameters:
//   - `t`: the test context.
//   - `path`: the file path to write.
//   - `content`: the file content.
func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
