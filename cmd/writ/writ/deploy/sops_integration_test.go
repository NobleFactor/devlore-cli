// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package deploy_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"
	"github.com/getsops/sops/v3"
	"github.com/getsops/sops/v3/aes"
	sopsage "github.com/getsops/sops/v3/age"
	sopsyaml "github.com/getsops/sops/v3/stores/yaml"

	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/deploy"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/segment"
)

// TestExecute_SopsChains deploys the two encrypted pipelines end to end — the plain decrypt
// (`secret.yaml.sops` → decrypted `secret.yaml`) and the decrypt+render chain
// (`note.yaml.template.sops` → decrypted, rendered `note.yaml`) — asserting content and that both outputs are
// private in the platform's own terms: 0600 mode bits on unix, a protected DACL on Windows
// (assertPrivateFile, the portable-fact pair). The ambient age identity comes from SOPS_AGE_KEY, per the
// sealed config-free decryption model.
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
	if err := os.WriteFile(filepath.Join(sourceRoot, "myproj", "note.yaml.template.sops"), templated, 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SOPS_AGE_KEY", ageKey+"\n"+templateKey)

	cfg := &deploy.Config{
		SourceRoot: sourceRoot,
		TargetRoot: targetRoot,
		Projects:   []string{"myproj"},
		Segments:   segment.Segments{{Name: "OS", Value: "Darwin"}},
	}

	if err := deploy.Execute(context.Background(), cfg); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// The plain decrypt: content decrypted, mode 0600.
	decrypted, err := os.ReadFile(filepath.Join(targetRoot, "secret.yaml"))
	if err != nil {
		t.Fatalf("read decrypted secret: %v", err)
	}
	if !strings.Contains(string(decrypted), "credential: hello world") {
		t.Errorf("decrypted secret = %q, want the plaintext credential", decrypted)
	}
	assertPrivateFile(t, filepath.Join(targetRoot, "secret.yaml"))

	// The decrypt+render chain: decrypted, rendered, 0600.
	note, err := os.ReadFile(filepath.Join(targetRoot, "note.yaml"))
	if err != nil {
		t.Fatalf("read rendered note: %v", err)
	}
	if !strings.Contains(string(note), "greeting: hi Darwin") {
		t.Errorf("rendered note = %q, want the rendered greeting", note)
	}
	assertPrivateFile(t, filepath.Join(targetRoot, "note.yaml"))
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
