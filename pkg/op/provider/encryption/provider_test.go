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
)

// testProvider creates a Provider with a RootReaderWriter for the given directory. It goes through NewProvider so the
// Encrypter is wired (EncryptFile needs it).
func testProvider(t *testing.T, dir string) *Provider {
	t.Helper()
	root := op.NewRootReaderWriter(dir)
	runtimeEnvironment := &op.RuntimeEnvironment{Root: root}
	return NewProvider(runtimeEnvironment)
}

// testProviderWithSops creates a Provider for the decrypt tests. Decryption is config-free — it reads the file's
// embedded SOPS metadata and the ambient SOPS_AGE_KEY — so no sops client configuration is needed.
func testProviderWithSops(t *testing.T, dir string) *Provider {
	t.Helper()
	root := op.NewRootReaderWriter(dir)
	runtimeEnvironment := &op.RuntimeEnvironment{Root: root}
	return &Provider{ProviderBase: op.NewProviderBase(runtimeEnvironment)}
}

// --- CompensateDecryptSopsFile ---

func TestCompensateDecryptSopsFile_RemovesFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "decrypted.yaml")
	if err := os.WriteFile(path, []byte("cleartext"), 0o600); err != nil {
		t.Fatal(err)
	}

	p := testProvider(t, tmp)
	resource, err := file.DiscoverResource(p.RuntimeEnvironment(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.CompensateDecryptSopsFile(&Receipt{ReceiptBase: op.NewReceiptBase(resource)}); err != nil {
		t.Fatalf("compensate: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should have been removed")
	}
}

func TestCompensateDecryptSopsFile_EmptyPath(t *testing.T) {
	p := testProvider(t, t.TempDir())
	if err := p.CompensateDecryptSopsFile(&Receipt{}); err != nil {
		t.Fatalf("compensate with empty receipt should succeed: %v", err)
	}
}

func TestCompensateDecryptSopsFile_MissingFile(t *testing.T) {
	tmp := t.TempDir()
	p := testProvider(t, tmp)
	resource, err := file.DiscoverResource(p.RuntimeEnvironment(), filepath.Join(tmp, "nonexistent"))
	if err != nil {
		t.Fatal(err)
	}
	err = p.CompensateDecryptSopsFile(&Receipt{ReceiptBase: op.NewReceiptBase(resource)})
	if err == nil {
		t.Fatal("expected error removing nonexistent file")
	}
}

// --- DecryptSopsFile ---

func TestDecryptSopsFile_SourceReadFailure(t *testing.T) {
	tmp := t.TempDir()
	p := testProviderWithSops(t, tmp)
	runtimeEnvironment := p.RuntimeEnvironment()
	source, _ := file.DiscoverResource(runtimeEnvironment, "/nonexistent/encrypted.yaml")
	destination := filepath.Join(tmp, "out.yaml")

	_, _, err := p.DecryptSopsFile(source, destination)
	if err == nil {
		t.Fatal("expected error for unresolvable source")
	}
}

func TestDecryptSopsFile_NilSopsClient(t *testing.T) {
	t.Skip("pending sops rewrite: config-free decrypt removed the nil-client error path")

	tmp := t.TempDir()
	p := testProvider(t, tmp) // no SopsClient

	srcPath := filepath.Join(tmp, "secret.yaml")
	if err := os.WriteFile(srcPath, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	runtimeEnvironment := p.RuntimeEnvironment()
	source, _ := file.DiscoverResource(runtimeEnvironment, srcPath)
	if err := source.Resolve(); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(tmp, "out.yaml")

	_, _, err := p.DecryptSopsFile(source, destination)
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
	runtimeEnvironment := p.RuntimeEnvironment()
	source, _ := file.DiscoverResource(runtimeEnvironment, srcPath)
	if err := source.Resolve(); err != nil {
		t.Fatalf("resolving source: %v", err)
	}

	destination := filepath.Join(tmp, "secret.dec.yaml")

	result, tombstone, err := p.DecryptSopsFile(source, destination)
	if err != nil {
		t.Fatalf("DecryptSopsFile: %v", err)
	}

	decrypted, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("reading decrypted file: %v", err)
	}

	if !bytes.Contains(decrypted, []byte("hello")) {
		t.Errorf("decrypted content missing 'hello': %s", decrypted)
	}
	if !bytes.Contains(decrypted, []byte("world")) {
		t.Errorf("decrypted content missing 'world': %s", decrypted)
	}

	if result.SourcePath.Abs() != destination {
		t.Errorf("result path = %q, want %q", result.SourcePath.Abs(), destination)
	}
	resource, ok := tombstone.Resource().(*file.Resource)
	if !ok {
		t.Fatalf("receipt resource = %T, want *file.Resource", tombstone.Resource())
	}
	if resource.SourcePath.Abs() != destination {
		t.Errorf("receipt resource path = %q, want %q", resource.SourcePath.Abs(), destination)
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
	runtimeEnvironment := p.RuntimeEnvironment()
	source, _ := file.DiscoverResource(runtimeEnvironment, srcPath)
	if err := source.Resolve(); err != nil {
		t.Fatal(err)
	}

	destination := filepath.Join(tmp, "secret.dec.yaml")

	_, tombstone, err := p.DecryptSopsFile(source, destination)
	if err != nil {
		t.Fatalf("DecryptSopsFile: %v", err)
	}

	// Decrypted file exists
	if _, err := os.Stat(destination); err != nil {
		t.Fatalf("decrypted file should exist: %v", err)
	}

	// undo removes it
	if err := p.CompensateDecryptSopsFile(tombstone); err != nil {
		t.Fatalf("compensate: %v", err)
	}

	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Error("compensate should have removed decrypted file")
	}
}

// --- EncryptFile ---

func TestEncryptFile_RoundTrip(t *testing.T) {

	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SOPS_AGE_KEY", identity.String())

	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // isolate: no XDG fallback

	// .sops.yaml governs the tree with a catch-all rule for the age recipient.
	sopsYAML := "creation_rules:\n  - path_regex: .*\n    age: " + identity.Recipient().String() + "\n"
	if err := os.WriteFile(filepath.Join(tmp, ".sops.yaml"), []byte(sopsYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	// Cleartext source on disk.
	srcPath := filepath.Join(tmp, "secret.yaml")
	plaintext := []byte("greeting: hello\nname: world\n")
	if err := os.WriteFile(srcPath, plaintext, 0o644); err != nil {
		t.Fatal(err)
	}

	p := testProvider(t, tmp)
	source, err := file.DiscoverResource(p.RuntimeEnvironment(), srcPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.Resolve(); err != nil {
		t.Fatal(err)
	}

	destPath := filepath.Join(tmp, "secret.enc.yaml")

	result, receipt, err := p.EncryptFile(source, destPath)
	if err != nil {
		t.Fatalf("EncryptFile: %v", err)
	}

	// The encrypted file exists, does not leak the plaintext values, and the result names it.
	encrypted, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read encrypted: %v", err)
	}
	if bytes.Contains(encrypted, []byte("hello")) || bytes.Contains(encrypted, []byte("world")) {
		t.Fatalf("plaintext leaked into the encrypted file:\n%s", encrypted)
	}
	if result.SourcePath.Abs() != destPath {
		t.Errorf("result path = %q, want %q", result.SourcePath.Abs(), destPath)
	}

	// Round-trip: decrypt it back and confirm the original content.
	encResource, err := file.DiscoverResource(p.RuntimeEnvironment(), destPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := encResource.Resolve(); err != nil {
		t.Fatal(err)
	}
	decPath := filepath.Join(tmp, "secret.dec.yaml")
	if _, _, err := p.DecryptSopsFile(encResource, decPath); err != nil {
		t.Fatalf("DecryptSopsFile: %v", err)
	}
	decrypted, err := os.ReadFile(decPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(decrypted, []byte("hello")) || !bytes.Contains(decrypted, []byte("world")) {
		t.Errorf("decrypted = %q, want to contain hello + world", decrypted)
	}

	// Compensation removes the encrypted file.
	if err := p.CompensateEncryptFile(receipt); err != nil {
		t.Fatalf("CompensateEncryptFile: %v", err)
	}
	if _, err := os.Stat(destPath); !os.IsNotExist(err) {
		t.Error("CompensateEncryptFile should have removed the encrypted file")
	}
}
