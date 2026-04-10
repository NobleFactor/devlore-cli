// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package encryption_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"filippo.io/age"
	"github.com/NobleFactor/devlore-cli/pkg/op/bind"
	"github.com/getsops/sops/v3"
	"github.com/getsops/sops/v3/aes"
	sopsage "github.com/getsops/sops/v3/age"
	"github.com/getsops/sops/v3/stores/yaml"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	encryptionprov "github.com/NobleFactor/devlore-cli/pkg/op/provider/encryption"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/encryption/gen"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/file/gen"
	sopsclient "github.com/NobleFactor/devlore-cli/pkg/op/sops"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

// sopsEncrypt generates age keys and encrypts plainYAML with SOPS.
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

	masterKey := &sopsage.MasterKey{Recipient: identity.Recipient().String()}

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

func testCtx(t *testing.T, dir string) *op.ExecutionContext {
	t.Helper()

	sopsConfig := filepath.Join(dir, ".sops.yaml")
	if err := os.WriteFile(sopsConfig, []byte("creation_rules:\n  - path_regex: .*\n    age: age1abc\n"), 0o644); err != nil {
		t.Fatalf("write .sops.yaml: %v", err)
	}

	client, err := sopsclient.NewClient(dir)
	if err != nil {
		t.Fatalf("sops.NewClient: %v", err)
	}

	root := op.NewRootReaderWriter(dir)
	return &op.ExecutionContext{
		Context:    context.Background(),
		Writer:     &bytes.Buffer{},
		Root:       root,
		Registry:   op.NewReceiverRegistry(),
		SopsClient: client,
	}
}

func receiverType(t *testing.T) op.ProviderReceiverType {
	t.Helper()
	reg := op.NewReceiverRegistry()
	rt, ok := reg.TypeByReflection(reflect.TypeFor[encryptionprov.Provider]())
	if !ok {
		t.Fatal("encryption provider type not registered")
	}
	return rt.(op.ProviderReceiverType)
}

// region Starlark integration

func TestStarlark(t *testing.T) {
	plainYAML := []byte("greeting: hello\nname: world\n")
	encrypted, ageKey := sopsEncrypt(t, plainYAML)

	dir := t.TempDir()
	srcPath := filepath.Join(dir, "secret.enc.yaml")
	if err := os.WriteFile(srcPath, encrypted, 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SOPS_AGE_KEY", ageKey)

	ctx := testCtx(t, dir)

	source, err := file.NewResource(ctx, srcPath)
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}
	if err := source.Resolve(); err != nil {
		t.Fatalf("resolve source: %v", err)
	}

	dstPath := filepath.Join(dir, "secret.dec.yaml")

	receiver := bind.NewProvider(receiverType(t), encryptionprov.NewProvider(ctx))

	globals := starlark.StringDict{
		"encryption":  receiver,
		"test_source": starlark.String(srcPath),
		"test_dest":   starlark.String(dstPath),
	}

	thread := &starlark.Thread{
		Name:  "encryption-integration",
		Print: func(_ *starlark.Thread, msg string) { t.Logf("[star] %s", msg) },
	}

	data, err := os.ReadFile("testdata/integration.star")
	if err != nil {
		t.Fatalf("reading script: %v", err)
	}

	opts := &syntax.FileOptions{Set: true, GlobalReassign: true, TopLevelControl: true}
	result, err := starlark.ExecFileOptions(opts, thread, "testdata/integration.star", data, globals)
	if err != nil {
		t.Fatalf("exec script: %v", err)
	}

	assertBool(t, result, "result_done")

	// Verify decryption.
	decrypted, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("reading decrypted: %v", err)
	}
	if !bytes.Contains(decrypted, []byte("hello")) {
		t.Errorf("decrypted missing 'hello': %s", decrypted)
	}
}

// endregion

// region Action dispatch

func TestActions_DecryptSopsFile(t *testing.T) {
	plainYAML := []byte("secret: value\n")
	encrypted, ageKey := sopsEncrypt(t, plainYAML)

	dir := t.TempDir()
	srcPath := filepath.Join(dir, "secret.enc.yaml")
	if err := os.WriteFile(srcPath, encrypted, 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SOPS_AGE_KEY", ageKey)

	ctx := testCtx(t, dir)

	source, err := file.NewResource(ctx, srcPath)
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}
	if err := source.Resolve(); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	dstPath := filepath.Join(dir, "secret.dec.yaml")
	dest, err := file.NewResource(ctx, dstPath)
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	a, err := ctx.ActionByName("encryption.decrypt_sops_file")
	if err != nil {
		t.Fatalf("action encryption.decrypt_sops_file not registered: %v", err)
	}

	result, complement, doErr := a.Do(ctx, map[string]any{
		"source":      source,
		"destination": dest,
	})
	if doErr != nil {
		t.Fatalf("Do() error = %v", doErr)
	}

	res, ok := result.(*file.Resource)
	if !ok {
		t.Fatalf("result type = %T, want *file.Resource", result)
	}
	if res.SourcePath.Abs() != dstPath {
		t.Errorf("result path = %q, want %q", res.SourcePath.Abs(), dstPath)
	}
	if complement == nil {
		t.Error("complement = nil, want non-nil tombstone")
	}

	decrypted, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("reading decrypted: %v", err)
	}
	if !bytes.Contains(decrypted, []byte("value")) {
		t.Errorf("decrypted missing 'value': %s", decrypted)
	}
}

// endregion

// region Assertions

func assertBool(t *testing.T, globals starlark.StringDict, key string) {
	t.Helper()
	v, ok := globals[key]
	if !ok {
		t.Errorf("missing global %q", key)
		return
	}
	if v != starlark.True {
		t.Errorf("%s = %v, want true", key, v)
	}
}

// endregion
