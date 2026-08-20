// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package signing

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// isolate builds a signer whose every input points inside the test sandbox, and returns it with its seeded
// allowed_signers path.
//
// Both locations are passed in, so nothing here depends on where the process thinks home is — which is what
// makes the isolation real. Home resolves from the account database ahead of the environment, so setting
// `HOME` would sandbox nothing and the SSH tier would read the developer's own `~/.ssh/id_ed25519`. Naming an
// identity path that does not exist drives resolution to the generated key deterministically, on every host.
func isolate(t *testing.T) (signer Signer, allowedSigners string) {

	t.Helper()

	root := t.TempDir()

	// The caller owns the root, exactly as cmd/internal/cli does in production; signing receives it.
	// Production creates the tree before opening it (cli.OpenTree); the MkdirAll is that step here.
	configDir := filepath.Join(root, "config", "devlore")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}

	configRoot, err := fsroot.OpenConfined(configDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = configRoot.Close() })

	signer, err = DefaultSigner(configRoot, filepath.Join(root, "absent", "id_ed25519"))
	if err != nil {
		t.Fatalf("DefaultSigner: %v", err)
	}

	// Seed the trust list from the generated .pub (authorized_keys format: "ssh-ed25519 <base64> comment").
	publicLine, err := os.ReadFile(filepath.Join(root, "config", "devlore", "signing", "ed25519.pub"))
	if err != nil {
		t.Fatalf("read generated .pub: %v", err)
	}
	fields := strings.Fields(string(publicLine))
	if len(fields) < 2 {
		t.Fatalf("unexpected .pub shape: %q", publicLine)
	}

	trustPath := filepath.Join(root, "config", "devlore", "allowed_signers")
	line := "dev@example.com " + fields[0] + " " + fields[1] + " test key\n"
	if err := os.WriteFile(trustPath, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}

	return signer, trustPath
}

// TestSignVerify_RoundTrip pins the default tier end to end: sign canonical bytes, verify, resolve the
// principal.
func TestSignVerify_RoundTrip(t *testing.T) {

	signer, trustPath := isolate(t)

	canonical := []byte("canonical: bytes\n")
	signature, err := signer.Sign(NamespaceGraph, canonical)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if signature.Algorithm != AlgorithmEd25519 {
		t.Errorf("algorithm = %q, want %s", signature.Algorithm, AlgorithmEd25519)
	}

	verdict := Verify(signature, NamespaceGraph, canonical, trustPath)
	if verdict.Outcome != OutcomeValid || verdict.Principal != "dev@example.com" {
		t.Errorf("verdict = %+v, want valid for dev@example.com", verdict)
	}
}

// TestVerify_Classifications pins the non-valid outcomes: unsigned, tampered bytes, wrong namespace, and an
// untrusted (unlisted) publisher.
func TestVerify_Classifications(t *testing.T) {

	signer, trustPath := isolate(t)
	canonical := []byte("canonical: bytes\n")
	signature, err := signer.Sign(NamespaceGraph, canonical)
	if err != nil {
		t.Fatal(err)
	}

	if v := Verify(nil, NamespaceGraph, canonical, trustPath); v.Outcome != OutcomeUnsigned {
		t.Errorf("nil signature = %v, want unsigned", v.Outcome)
	}

	if v := Verify(signature, NamespaceGraph, []byte("tampered\n"), trustPath); v.Outcome != OutcomeInvalid {
		t.Errorf("tampered bytes = %v, want invalid", v.Outcome)
	}

	// The namespace is part of the signed message — a graph signature must not verify as a trace signature.
	if v := Verify(signature, NamespaceTrace, canonical, trustPath); v.Outcome != OutcomeInvalid {
		t.Errorf("cross-namespace = %v, want invalid (domain separation)", v.Outcome)
	}

	if v := Verify(signature, NamespaceGraph, canonical, filepath.Join(t.TempDir(), "empty")); v.Outcome != OutcomeUntrusted {
		t.Errorf("missing trust list = %v, want untrusted", v.Outcome)
	}

	unsupported := &op.Signature{Algorithm: "ecdsa-sha2-nistp256", PublicKey: signature.PublicKey, Value: signature.Value}
	if v := Verify(unsupported, NamespaceGraph, canonical, trustPath); v.Outcome != OutcomeInvalid {
		t.Errorf("unsupported algorithm = %v, want invalid", v.Outcome)
	}
}

// TestAllowedSigners_Scoping pins the trust-list options: namespaces= globs and validity windows.
func TestAllowedSigners_Scoping(t *testing.T) {

	signer, _ := isolate(t)
	canonical := []byte("c\n")
	signature, err := signer.Sign(NamespaceGraph, canonical)
	if err != nil {
		t.Fatal(err)
	}
	keyB64 := base64.StdEncoding.EncodeToString(signature.PublicKey)

	write := func(line string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "allowed_signers")
		if err := os.WriteFile(path, []byte(line+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	// Namespace glob admits the graph namespace.
	scoped := write(`ci@example.com namespaces="devlore.*" ssh-ed25519 ` + keyB64)
	if v := Verify(signature, NamespaceGraph, canonical, scoped); v.Outcome != OutcomeValid {
		t.Errorf("devlore.* scope = %+v, want valid", v)
	}

	// A foreign namespace scope refuses the key.
	foreign := write(`ci@example.com namespaces="git.commit" ssh-ed25519 ` + keyB64)
	if v := Verify(signature, NamespaceGraph, canonical, foreign); v.Outcome != OutcomeUntrusted {
		t.Errorf("git.commit scope = %v, want untrusted", v.Outcome)
	}

	// An expired validity window refuses the key.
	expired := write(`ci@example.com valid-before="20200101" ssh-ed25519 ` + keyB64)
	if v := Verify(signature, NamespaceGraph, canonical, expired); v.Outcome != OutcomeUntrusted {
		t.Errorf("expired window = %v, want untrusted", v.Outcome)
	}

	// A cert-authority line is recognized but unsupported in the default tier.
	authority := write(`*@example.com cert-authority ssh-ed25519 ` + keyB64)
	if v := Verify(signature, NamespaceGraph, canonical, authority); v.Outcome != OutcomeUntrusted {
		t.Errorf("cert-authority line = %v, want untrusted (unsupported tier)", v.Outcome)
	}
}

// TestPolicy_Judge pins the enforcement ladder across outcomes and externality.
func TestPolicy_Judge(t *testing.T) {

	valid := Verdict{Outcome: OutcomeValid}
	unsigned := Verdict{Outcome: OutcomeUnsigned}
	invalid := Verdict{Outcome: OutcomeInvalid}

	cases := []struct {
		policy   Policy
		verdict  Verdict
		external bool
		rejects  bool
	}{
		{PolicyIgnore, invalid, true, false},
		{PolicyReport, unsigned, true, false},
		{PolicyReport, invalid, true, false},
		{PolicyRejectExternal, unsigned, false, false}, // own store: report semantics
		{PolicyRejectExternal, unsigned, true, true},
		{PolicyRejectExternal, invalid, true, true},
		{PolicyReject, unsigned, false, true},
		{PolicyReject, valid, true, false},
	}

	for _, c := range cases {
		err := c.policy.Judge(c.verdict, c.external)
		if (err != nil) != c.rejects {
			t.Errorf("Judge(%s, %s, external=%v) = %v, want rejects=%v",
				c.policy, c.verdict.Outcome, c.external, err, c.rejects)
		}
	}
}

// TestParsePolicy pins the snake_case config vocabulary and the report floor.
func TestParsePolicy(t *testing.T) {

	for value, want := range map[string]Policy{
		"":                PolicyReport,
		"report":          PolicyReport,
		"ignore":          PolicyIgnore,
		"reject_external": PolicyRejectExternal,
		"reject":          PolicyReject,
	} {
		got, err := ParsePolicy(value)
		if err != nil || got != want {
			t.Errorf("ParsePolicy(%q) = %v, %v; want %v", value, got, err, want)
		}
	}

	if _, err := ParsePolicy("reject-external"); err == nil {
		t.Error("ParsePolicy accepted a hyphenated value; config uses snake_case")
	}
}

// TestExternal pins the store-boundary externality marker.
func TestExternal(t *testing.T) {

	store := t.TempDir()

	if External(filepath.Join(store, "graphs", "x.yaml"), store) {
		t.Error("a store-resident document classified external")
	}
	if !External(filepath.Join(t.TempDir(), "shared.yaml"), store) {
		t.Error("an outside document classified as own-store")
	}
}
