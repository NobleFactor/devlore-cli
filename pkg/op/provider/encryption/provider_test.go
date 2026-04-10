// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package encryption

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
	"github.com/getsops/sops/v3"
	"github.com/getsops/sops/v3/aes"
	sopsage "github.com/getsops/sops/v3/age"
	"github.com/getsops/sops/v3/stores/yaml"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	sopsclient "github.com/NobleFactor/devlore-cli/pkg/op/sops"
)

// testProvider creates a Provider with a RootReaderWriter for the given directory.
func testProvider(t *testing.T, dir string) *Provider {
	t.Helper()
	root := op.NewRootReaderWriter(dir)
	ctx := &op.ExecutionContext{Root: root}
	return &Provider{ProviderBase: op.NewProviderBase(ctx)}
}

// testProviderWithSops creates a Provider with a SopsClient configured from dir.
func testProviderWithSops(t *testing.T, dir string) *Provider {
	t.Helper()

	// Write a minimal .sops.yaml so NewClient succeeds
	sopsConfig := filepath.Join(dir, ".sops.yaml")
	if err := os.WriteFile(sopsConfig, []byte("creation_rules:\n  - path_regex: .*\n    age: age1abc\n"), 0o644); err != nil {
		t.Fatalf("write .sops.yaml: %v", err)
	}

	client, err := sopsclient.NewClient(dir)
	if err != nil {
		t.Fatalf("sops.NewClient: %v", err)
	}

	root := op.NewRootReaderWriter(dir)
	ctx := &op.ExecutionContext{Root: root, SopsClient: client}
	return &Provider{ProviderBase: op.NewProviderBase(ctx)}
}

// --- CompensateDecryptSopsFile ---

func TestCompensateDecryptSopsFile_RemovesFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "decrypted.yaml")
	if err := os.WriteFile(path, []byte("cleartext"), 0o600); err != nil {
		t.Fatal(err)
	}

	p := testProvider(t, tmp)
	if err := p.CompensateDecryptSopsFile(Tombstone{DestinationPath: path}); err != nil {
		t.Fatalf("compensate: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should have been removed")
	}
}

func TestCompensateDecryptSopsFile_EmptyPath(t *testing.T) {
	p := testProvider(t, t.TempDir())
	if err := p.CompensateDecryptSopsFile(Tombstone{}); err != nil {
		t.Fatalf("compensate with empty path should succeed: %v", err)
	}
}

func TestCompensateDecryptSopsFile_MissingFile(t *testing.T) {
	p := testProvider(t, t.TempDir())
	err := p.CompensateDecryptSopsFile(Tombstone{DestinationPath: "/nonexistent/path"})
	if err == nil {
		t.Fatal("expected error removing nonexistent file")
	}
}

// --- DecryptSopsFile ---

func TestDecryptSopsFile_SourceReadFailure(t *testing.T) {
	tmp := t.TempDir()
	p := testProviderWithSops(t, tmp)
	ctx := p.ExecutionContext()
	source, _ := file.NewResource(ctx, "/nonexistent/encrypted.yaml")
	dest, _ := file.NewResource(ctx, filepath.Join(tmp, "out.yaml"))

	_, _, err := p.DecryptSopsFile(source, dest)
	if err == nil {
		t.Fatal("expected error for unresolvable source")
	}
}

func TestDecryptSopsFile_NilSopsClient(t *testing.T) {
	tmp := t.TempDir()
	p := testProvider(t, tmp) // no SopsClient

	srcPath := filepath.Join(tmp, "secret.yaml")
	if err := os.WriteFile(srcPath, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := p.ExecutionContext()
	source, _ := file.NewResource(ctx, srcPath)
	if err := source.Resolve(); err != nil {
		t.Fatal(err)
	}
	dest, _ := file.NewResource(ctx, filepath.Join(tmp, "out.yaml"))

	_, _, err := p.DecryptSopsFile(source, dest)
	if err == nil {
		t.Fatal("expected error when SopsClient is nil")
	}
}

// sopsEncrypt generates age keys and encrypts plainYAML with SOPS.
// Returns the encrypted bytes and the age identity string for decryption.
func sopsEncrypt(t *testing.T, plainYAML []byte) ([]byte, string) {
	t.Helper()

	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}

	store := &yaml.Store{}
	branches, err := store.LoadPlainFile(plainYAML)
	if err != nil {
		t.Fatalf("loading plain YAML: %v", err)
	}

	masterKey := &sopsage.MasterKey{
		Recipient: identity.Recipient().String(),
	}

	tree := sops.Tree{
		Branches: branches,
		Metadata: sops.Metadata{
			KeyGroups: []sops.KeyGroup{{masterKey}},
			Version:   "3.7.0",
		},
	}

	dataKey, errs := tree.GenerateDataKey()
	if len(errs) > 0 {
		t.Fatalf("GenerateDataKey: %v", errs)
	}

	cipher := aes.NewCipher()
	mac, err := tree.Encrypt(dataKey, cipher)
	if err != nil {
		t.Fatalf("encrypting tree: %v", err)
	}

	encryptedMac, err := cipher.Encrypt(mac, dataKey, tree.Metadata.LastModified.Format("2006-01-02T15:04:05Z"))
	if err != nil {
		t.Fatalf("encrypting MAC: %v", err)
	}
	tree.Metadata.MessageAuthenticationCode = encryptedMac

	encrypted, err := store.EmitEncryptedFile(tree)
	if err != nil {
		t.Fatalf("emitting encrypted file: %v", err)
	}

	return encrypted, identity.String()
}

func TestDecryptSopsFile_RoundTrip(t *testing.T) {
	plainYAML := []byte("greeting: hello\nname: world\n")
	encrypted, ageKey := sopsEncrypt(t, plainYAML)

	tmp := t.TempDir()
	srcPath := filepath.Join(tmp, "secret.enc.yaml")
	if err := os.WriteFile(srcPath, encrypted, 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SOPS_AGE_KEY", ageKey)

	p := testProviderWithSops(t, tmp)
	ctx := p.ExecutionContext()
	source, _ := file.NewResource(ctx, srcPath)
	if err := source.Resolve(); err != nil {
		t.Fatalf("resolving source: %v", err)
	}

	dstPath := filepath.Join(tmp, "secret.dec.yaml")
	dest, _ := file.NewResource(ctx, dstPath)
	result, tombstone, err := p.DecryptSopsFile(source, dest)
	if err != nil {
		t.Fatalf("DecryptSopsFile: %v", err)
	}

	decrypted, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("reading decrypted file: %v", err)
	}

	if !bytes.Contains(decrypted, []byte("hello")) {
		t.Errorf("decrypted content missing 'hello': %s", decrypted)
	}
	if !bytes.Contains(decrypted, []byte("world")) {
		t.Errorf("decrypted content missing 'world': %s", decrypted)
	}

	if result.SourcePath.Abs() != dstPath {
		t.Errorf("result path = %q, want %q", result.SourcePath.Abs(), dstPath)
	}
	if tombstone.DestinationPath != dstPath {
		t.Errorf("tombstone path = %q, want %q", tombstone.DestinationPath, dstPath)
	}
}

func TestDecryptSopsFile_CompensateRoundTrip(t *testing.T) {
	plainYAML := []byte("secret: value\n")
	encrypted, ageKey := sopsEncrypt(t, plainYAML)

	tmp := t.TempDir()
	srcPath := filepath.Join(tmp, "secret.enc.yaml")
	if err := os.WriteFile(srcPath, encrypted, 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SOPS_AGE_KEY", ageKey)

	p := testProviderWithSops(t, tmp)
	ctx := p.ExecutionContext()
	source, _ := file.NewResource(ctx, srcPath)
	if err := source.Resolve(); err != nil {
		t.Fatal(err)
	}

	dstPath := filepath.Join(tmp, "secret.dec.yaml")
	dest, _ := file.NewResource(ctx, dstPath)
	_, tombstone, err := p.DecryptSopsFile(source, dest)
	if err != nil {
		t.Fatalf("DecryptSopsFile: %v", err)
	}

	// Decrypted file exists
	if _, err := os.Stat(dstPath); err != nil {
		t.Fatalf("decrypted file should exist: %v", err)
	}

	// undo removes it
	if err := p.CompensateDecryptSopsFile(tombstone); err != nil {
		t.Fatalf("compensate: %v", err)
	}

	if _, err := os.Stat(dstPath); !os.IsNotExist(err) {
		t.Error("compensate should have removed decrypted file")
	}
}
