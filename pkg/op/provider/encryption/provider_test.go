// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package encryption

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"
	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/getsops/sops/v3"
	"github.com/getsops/sops/v3/aes"
	sopsage "github.com/getsops/sops/v3/age"
	"github.com/getsops/sops/v3/stores/yaml"

	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
)

// testActivation wraps runtimeEnvironment in an [op.ActivationRecord] for non-graph dispatch (nil Graph and Unit).
func testActivation(t *testing.T, runtimeEnvironment *op.RuntimeEnvironment) *op.ActivationRecord {
	t.Helper()
	return op.NewActivationRecord(nil, "", runtimeEnvironment)
}

// testProvider creates a Provider with a RootReaderWriter for the given directory. It goes through NewProvider so the
// Encrypter is wired (EncryptFile needs it).
// testEnvironment builds a session rooted at `dir` through the real constructor.
//
// Tests travel the same construction path production does: the session mints the root from the spec's anchor and
// wires the recovery site and resource catalog itself, so nothing here hand-assembles filesystem access.
func testEnvironment(t *testing.T, dir string) *op.RuntimeEnvironment {

	t.Helper()

	runtimeEnvironment, err := op.NewRuntimeEnvironment(context.Background(),
		op.NewRuntimeEnvironmentSpec("test").
			WithRoot(dir, fsroot.ModeWritableUnconfined).
			WithApplication(&application.Application{Name: "test"}))
	if err != nil {
		t.Fatalf("op.NewRuntimeEnvironment: %v", err)
	}
	t.Cleanup(func() { _ = runtimeEnvironment.Close() })

	return runtimeEnvironment
}

func testProvider(t *testing.T, dir string) *Provider {
	t.Helper()
	runtimeEnvironment := testEnvironment(t, dir)
	return NewProvider(runtimeEnvironment)
}

// testProviderWithSops creates a Provider for the decrypt tests. Decryption is config-free — it reads the file's
// embedded SOPS metadata and the ambient SOPS_AGE_KEY — so no sops client configuration is needed.
func testProviderWithSops(t *testing.T, dir string) *Provider {
	t.Helper()
	runtimeEnvironment := testEnvironment(t, dir)
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
	resource, err := file.DiscoverRegular(p.RuntimeEnvironment(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.CompensateDecryptSopsFile(testActivation(t, p.RuntimeEnvironment()), &Receipt{ReceiptBase: op.NewReceiptBase(resource)}); err != nil {
		t.Fatalf("compensate: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should have been removed")
	}
}

func TestCompensateDecryptSopsFile_EmptyPath(t *testing.T) {
	p := testProvider(t, t.TempDir())
	if err := p.CompensateDecryptSopsFile(testActivation(t, p.RuntimeEnvironment()), &Receipt{}); err != nil {
		t.Fatalf("compensate with empty receipt should succeed: %v", err)
	}
}

func TestCompensateDecryptSopsFile_MissingFile(t *testing.T) {
	tmp := t.TempDir()
	p := testProvider(t, tmp)
	resource, err := file.DiscoverRegular(p.RuntimeEnvironment(), filepath.Join(tmp, "nonexistent"))
	if err != nil {
		t.Fatal(err)
	}
	err = p.CompensateDecryptSopsFile(testActivation(t, p.RuntimeEnvironment()), &Receipt{ReceiptBase: op.NewReceiptBase(resource)})
	if err == nil {
		t.Fatal("expected error removing nonexistent file")
	}
}

// --- the secret-mode floor ---

// TestEnforceSecretFloor_RefusesAccessBeyondOwner pins the rule that makes the mode parameter safe to expose.
//
// Encrypting a file is the declaration that its contents are sensitive, so the decrypted product's mode follows from
// that declaration. Accepting a mode without a floor would reopen the decision to a caller who can get it wrong, and a
// world-readable secret fails silently: the file is written, the run succeeds, and nothing reports it.
func TestEnforceSecretFloor_RefusesAccessBeyondOwner(t *testing.T) {

	cases := []struct {
		name string
		mode os.FileMode
		want bool
	}{
		{"owner read-write", 0o600, true},
		{"owner read-only", 0o400, true},
		{"owner all", 0o700, true},
		{"group read", 0o640, false},
		{"other read", 0o604, false},
		{"world readable", 0o644, false},
		{"world writable", 0o666, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := enforceSecretFloor(tc.mode)
			if tc.want && err != nil {
				t.Errorf("enforceSecretFloor(%04o) = %v, want nil", tc.mode, err)
			}
			if !tc.want && err == nil {
				t.Errorf("enforceSecretFloor(%04o) = nil, want an error", tc.mode)
			}
		})
	}
}

// TestDecryptSopsFile_RefusesReadableMode proves the floor is enforced at the action, not merely available as a helper
// -- and that it is refused BEFORE any decryption happens, so a rejected call never materializes plaintext.
func TestDecryptSopsFile_RefusesReadableMode(t *testing.T) {

	tmp := t.TempDir()
	p := testProviderWithSops(t, tmp)
	source, _ := file.DiscoverRegular(p.RuntimeEnvironment(), filepath.Join(tmp, "encrypted.yaml"))
	destination := filepath.Join(tmp, "out.yaml")

	_, _, err := p.DecryptSopsFile(testActivation(t, p.RuntimeEnvironment()), source, destination, 0o644)
	if err == nil {
		t.Fatal("DecryptSopsFile with mode 0644: want an error, got nil")
	}
	if !strings.Contains(err.Error(), "beyond its owner") {
		t.Errorf("error = %q, want it to name the floor", err.Error())
	}
	if _, statErr := os.Stat(destination); statErr == nil {
		t.Error("a refused decrypt wrote its destination anyway")
	}
}

// --- DecryptSopsFile ---

func TestDecryptSopsFile_SourceReadFailure(t *testing.T) {
	tmp := t.TempDir()
	p := testProviderWithSops(t, tmp)
	runtimeEnvironment := p.RuntimeEnvironment()
	source, _ := file.DiscoverRegular(runtimeEnvironment, "/nonexistent/encrypted.yaml")
	destination := filepath.Join(tmp, "out.yaml")

	_, _, err := p.DecryptSopsFile(testActivation(t, p.RuntimeEnvironment()), source, destination, 0o600)
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
	source, _ := file.DiscoverRegular(runtimeEnvironment, srcPath)
	if err := source.Resolve(); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(tmp, "out.yaml")

	_, _, err := p.DecryptSopsFile(testActivation(t, p.RuntimeEnvironment()), source, destination, 0o600)
	if err == nil {
		t.Fatal("expected error when SopsClient is nil")
	}
}

// sopsEncrypt generates age keys and encrypts plainYAML with SOPS.
// Returns the encrypted bytes and the age identity string for decryption.
func sopsEncrypt(t *testing.T, plainYAML []byte) (encrypted []byte, identity string) {
	t.Helper()

	ageIdentity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}

	store := &yaml.Store{}
	branches, err := store.LoadPlainFile(plainYAML)
	if err != nil {
		t.Fatalf("loading plain YAML: %v", err)
	}

	masterKey := &sopsage.MasterKey{
		Recipient: ageIdentity.Recipient().String(),
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

	encrypted, err = store.EmitEncryptedFile(tree)
	if err != nil {
		t.Fatalf("emitting encrypted file: %v", err)
	}

	return encrypted, ageIdentity.String()
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
	source, _ := file.DiscoverRegular(runtimeEnvironment, srcPath)
	if err := source.Resolve(); err != nil {
		t.Fatalf("resolving source: %v", err)
	}

	destination := filepath.Join(tmp, "secret.dec.yaml")

	result, receipt, err := p.DecryptSopsFile(testActivation(t, p.RuntimeEnvironment()), source, destination, 0o600)
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
	resource, ok := receipt.Resource().(*file.Regular)
	if !ok {
		t.Fatalf("receipt resource = %T, want *file.Regular", receipt.Resource())
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
	source, _ := file.DiscoverRegular(runtimeEnvironment, srcPath)
	if err := source.Resolve(); err != nil {
		t.Fatal(err)
	}

	destination := filepath.Join(tmp, "secret.dec.yaml")

	_, receipt, err := p.DecryptSopsFile(testActivation(t, p.RuntimeEnvironment()), source, destination, 0o600)
	if err != nil {
		t.Fatalf("DecryptSopsFile: %v", err)
	}

	// Decrypted file exists
	if _, err := os.Stat(destination); err != nil {
		t.Fatalf("decrypted file should exist: %v", err)
	}

	// undo removes it
	if err := p.CompensateDecryptSopsFile(testActivation(t, p.RuntimeEnvironment()), receipt); err != nil {
		t.Fatalf("compensate: %v", err)
	}

	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Error("compensate should have removed decrypted file")
	}
}

// --- CompensateEncryptFile ---

func TestCompensateEncryptFile_RemovesFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "secret.enc.yaml")
	if err := os.WriteFile(path, []byte("ciphertext"), 0o600); err != nil {
		t.Fatal(err)
	}

	p := testProvider(t, tmp)
	resource, err := file.DiscoverRegular(p.RuntimeEnvironment(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.CompensateEncryptFile(testActivation(t, p.RuntimeEnvironment()), &Receipt{ReceiptBase: op.NewReceiptBase(resource)}); err != nil {
		t.Fatalf("compensate: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should have been removed")
	}
}

func TestCompensateEncryptFile_EmptyPath(t *testing.T) {
	p := testProvider(t, t.TempDir())
	if err := p.CompensateEncryptFile(testActivation(t, p.RuntimeEnvironment()), &Receipt{}); err != nil {
		t.Fatalf("compensate with empty receipt should succeed: %v", err)
	}
}

func TestCompensateEncryptFile_MissingFile(t *testing.T) {
	tmp := t.TempDir()
	p := testProvider(t, tmp)
	resource, err := file.DiscoverRegular(p.RuntimeEnvironment(), filepath.Join(tmp, "nonexistent"))
	if err != nil {
		t.Fatal(err)
	}
	err = p.CompensateEncryptFile(testActivation(t, p.RuntimeEnvironment()), &Receipt{ReceiptBase: op.NewReceiptBase(resource)})
	if err == nil {
		t.Fatal("expected error removing nonexistent file")
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
	source, err := file.DiscoverRegular(p.RuntimeEnvironment(), srcPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.Resolve(); err != nil {
		t.Fatal(err)
	}

	destPath := filepath.Join(tmp, "secret.enc.yaml")

	result, receipt, err := p.EncryptFile(testActivation(t, p.RuntimeEnvironment()), source, destPath, 0o600)
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
	encResource, err := file.DiscoverRegular(p.RuntimeEnvironment(), destPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := encResource.Resolve(); err != nil {
		t.Fatal(err)
	}
	decPath := filepath.Join(tmp, "secret.dec.yaml")
	if _, _, err := p.DecryptSopsFile(testActivation(t, p.RuntimeEnvironment()), encResource, decPath, 0o600); err != nil {
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
	if err := p.CompensateEncryptFile(testActivation(t, p.RuntimeEnvironment()), receipt); err != nil {
		t.Fatalf("CompensateEncryptFile: %v", err)
	}
	if _, err := os.Stat(destPath); !os.IsNotExist(err) {
		t.Error("CompensateEncryptFile should have removed the encrypted file")
	}
}
