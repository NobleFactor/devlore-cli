// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package deploy_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
	"github.com/getsops/sops/v3"
	"github.com/getsops/sops/v3/aes"
	sopsage "github.com/getsops/sops/v3/age"
	sopsyaml "github.com/getsops/sops/v3/stores/yaml"

	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/deploy"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/segment"
	writsops "github.com/NobleFactor/devlore-cli/pkg/sops"
)

// TestExecute_SopsChains deploys the two encrypted pipelines end to end — the plain decrypt
// (`secret.yaml.sops` → decrypted `secret.yaml`, and `token.sops`, a name with no format, → `token`) and the
// decrypt+render chain (`note.yaml.tmpl.sops` → decrypted, rendered `note.yaml`) — asserting each output's content
// exactly and that each is private in the platform's own terms: 0600 mode bits on unix, a protected DACL on Windows
// (assertPrivateFile, the portable-fact pair). The ambient age identity comes from SOPS_AGE_KEY, per the sealed
// config-free decryption model.
func TestExecute_SopsChains(t *testing.T) {

	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))

	sourceRoot := filepath.Join(root, "src")
	targetRoot := filepath.Join(root, "home")

	if err := os.MkdirAll(filepath.Join(sourceRoot, "myproj"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	secret, ageKey := sopsEncrypt(t, []byte("credential: hello world\n"))
	if err := os.WriteFile(filepath.Join(sourceRoot, "myproj", "secret.yaml.sops"), secret, 0o644); err != nil {
		t.Fatal(err)
	}

	templated, templateKey := sopsEncrypt(t, []byte("greeting: hi {{ .Segments.OS }}\n"))
	if err := os.WriteFile(filepath.Join(sourceRoot, "myproj", "note.yaml.tmpl.sops"), templated, 0o644); err != nil {
		t.Fatal(err)
	}

	// A name with no format, encrypted the way writ encrypts it (#977).
	token, tokenKey := writEncrypt(t, filepath.Join(root, "authoring"), "token", []byte("hunter2\n"))
	if err := os.WriteFile(filepath.Join(sourceRoot, "myproj", "token.sops"), token, 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SOPS_AGE_KEY", ageKey+"\n"+templateKey+"\n"+tokenKey)

	cfg := &deploy.Config{
		SourceRoot: sourceRoot,
		TargetRoot: targetRoot,
		Projects:   []string{"myproj"},
		Segments:   segment.Segments{{Name: "OS", Value: "Darwin"}},
	}

	if _, err := deploy.Execute(context.Background(), cfg); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// The plain decrypt: content decrypted, mode 0600.
	decrypted, err := os.ReadFile(filepath.Join(targetRoot, "secret.yaml"))
	if err != nil {
		t.Fatalf("read decrypted secret: %v", err)
	}
	if string(decrypted) != "credential: hello world\n" {
		t.Errorf("decrypted secret = %q, want %q", decrypted, "credential: hello world\n")
	}
	assertPrivateFile(t, filepath.Join(targetRoot, "secret.yaml"))

	// A name with no format: the bytes that were encrypted, 0600.
	plainToken, err := os.ReadFile(filepath.Join(targetRoot, "token"))
	if err != nil {
		t.Fatalf("read decrypted token: %v", err)
	}
	if string(plainToken) != "hunter2\n" {
		t.Errorf("decrypted token = %q, want %q", plainToken, "hunter2\n")
	}
	assertPrivateFile(t, filepath.Join(targetRoot, "token"))

	// The decrypt+render chain: decrypted, rendered, 0600.
	note, err := os.ReadFile(filepath.Join(targetRoot, "note.yaml"))
	if err != nil {
		t.Fatalf("read rendered note: %v", err)
	}
	if string(note) != "greeting: hi Darwin\n" {
		t.Errorf("rendered note = %q, want %q", note, "greeting: hi Darwin\n")
	}
	assertPrivateFile(t, filepath.Join(targetRoot, "note.yaml"))
}

// writEncrypt encrypts cleartext with writ's own encrypter, as `writ secret encrypt` does, for a fresh age identity.
//
// The `.sops.yaml` that names the identity's recipient is written to dir, outside the deployed source tree.
//
// Parameters:
//   - t: the test.
//   - dir: the directory the cleartext is authored in, which holds the `.sops.yaml`.
//   - name: the cleartext's file name.
//   - cleartext: the bytes to encrypt.
//
// Returns:
//   - []byte: the encrypted document.
//   - string: the age identity that decrypts it.
func writEncrypt(t *testing.T, dir, name string, cleartext []byte) (encrypted []byte, identity string) {
	t.Helper()

	ageIdentity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	sopsYAML := "creation_rules:\n  - path_regex: .*\n    age: " + ageIdentity.Recipient().String() + "\n"
	if err := os.WriteFile(filepath.Join(dir, ".sops.yaml"), []byte(sopsYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	encrypted, err = writsops.NewEncrypter().Encrypt(cleartext, filepath.Join(dir, name), dir)
	if err != nil {
		t.Fatalf("Encrypt %s: %v", name, err)
	}
	return encrypted, ageIdentity.String()
}

// sopsEncrypt generates an age identity and encrypts plainYAML with SOPS, returning the encrypted bytes and
// the identity string for decryption. Lifted from the encryption provider's test fixture pattern
// (pkg/op/provider/encryption/provider_test.go) per the slice-4 ruling.
func sopsEncrypt(t *testing.T, plainYAML []byte) (encrypted []byte, identity string) {
	t.Helper()

	ageIdentity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}

	store := &sopsyaml.Store{}
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
