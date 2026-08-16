// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package signing

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/xdg"
)

// Outcome classifies one artifact's verification result.
type Outcome int

const (
	// OutcomeValid means the signature verifies and the publisher key resolved to a trusted principal.
	OutcomeValid Outcome = iota

	// OutcomeUnsigned means the artifact carries no signature — a finding of absence, not a failure.
	OutcomeUnsigned

	// OutcomeInvalid means a signature is present but does not verify over the canonical bytes (the artifact
	// was altered after signing, or the signature is malformed).
	OutcomeInvalid

	// OutcomeUntrusted means the signature verifies but the publisher key resolved to no trusted principal in
	// the verifier's `allowed_signers`.
	OutcomeUntrusted
)

// String returns the outcome's lowercase label.
//
// Returns:
//   - `string`: "valid", "unsigned", "invalid", or "untrusted".
func (o Outcome) String() string {
	switch o {
	case OutcomeValid:
		return "valid"
	case OutcomeUnsigned:
		return "unsigned"
	case OutcomeInvalid:
		return "invalid"
	case OutcomeUntrusted:
		return "untrusted"
	default:
		return "invalid"
	}
}

// Verdict is one artifact's verification result: the outcome, the trusted principal when valid, and the
// human-readable detail otherwise.
type Verdict struct {

	// Outcome is the classification.
	Outcome Outcome

	// Principal is the trusted identity the publisher key resolved to; "" unless the outcome is valid.
	Principal string

	// Detail elaborates non-valid outcomes for human readers.
	Detail string
}

// Verify checks an artifact's signature over its namespace-prefixed canonical bytes and resolves the publisher
// against the trust list.
//
// Parameters:
//   - `signature`: the artifact's signature, or nil for unsigned.
//   - `namespace`: the artifact-kind domain separator ([NamespaceGraph] or [NamespaceTrace]).
//   - `canonical`: the artifact's canonical bytes.
//   - `allowedSignersPath`: the verifier's trust list; "" resolves to the default
//     (`<config>/devlore/allowed_signers`).
//
// Returns:
//   - `Verdict`: the classification, with the trusted principal on validity.
func Verify(signature *op.Signature, namespace string, canonical []byte, allowedSignersPath string) Verdict {

	if signature == nil {
		return Verdict{Outcome: OutcomeUnsigned, Detail: "no signature present"}
	}

	if signature.Algorithm != AlgorithmEd25519 {
		return Verdict{Outcome: OutcomeInvalid,
			Detail: fmt.Sprintf("unsupported algorithm %q (this tier implements %s)", signature.Algorithm, AlgorithmEd25519)}
	}

	public, err := ed25519FromWire(signature.PublicKey)
	if err != nil {
		return Verdict{Outcome: OutcomeInvalid, Detail: "malformed public key: " + err.Error()}
	}

	message := append([]byte(namespace), canonical...)
	if !ed25519.Verify(public, message, signature.Value) {
		return Verdict{Outcome: OutcomeInvalid, Detail: "signature does not verify over the canonical bytes"}
	}

	principal, err := trustedPrincipal(signature.PublicKey, namespace, allowedSignersPath, time.Now())
	if err != nil {
		return Verdict{Outcome: OutcomeUntrusted, Detail: err.Error()}
	}

	return Verdict{Outcome: OutcomeValid, Principal: principal}
}

// CanonicalDocument returns a serialized document's canonical bytes: the generically-decoded document with
// its integrity fields (`checksum`, `signature`) removed, re-marshaled.
//
// This is the verify-side dual of the live artifacts' CanonicalContent for document-form canonicalization
// (traces): decoding into the typed struct can be lossy (custom unmarshalers), so verification canonicalizes
// the bytes it was handed. yaml key ordering is stable, so the result matches the sign-time canonical.
//
// Parameters:
//   - `data`: the document bytes (YAML).
//
// Returns:
//   - `[]byte`: the canonical bytes.
//   - `error`: non-nil when the document does not decode or re-marshal.
func CanonicalDocument(data []byte) ([]byte, error) {

	var generic map[string]any
	if err := yaml.Unmarshal(data, &generic); err != nil {
		return nil, fmt.Errorf("signing.CanonicalDocument: %w", err)
	}
	delete(generic, "checksum")
	delete(generic, "signature")

	canonical, err := yaml.Marshal(generic)
	if err != nil {
		return nil, fmt.Errorf("signing.CanonicalDocument: %w", err)
	}
	return canonical, nil
}

// External reports whether a document path lies outside `storeHome` — the settled externality marker (a
// document under this machine's own store was produced by its own runs; anything else is external).
//
// Parameters:
//   - `documentPath`: the document's path as given.
//   - `storeHome`: the machine's store root (the devlore state home).
//
// Returns:
//   - `bool`: true when the document is external to the store.
func External(documentPath, storeHome string) bool {

	absDocument, err := filepath.Abs(documentPath)
	if err != nil {
		return true
	}
	absStore, err := filepath.Abs(storeHome)
	if err != nil {
		return true
	}
	return !strings.HasPrefix(absDocument, absStore+string(os.PathSeparator)) && absDocument != absStore
}

// region HELPER FUNCTIONS

// ed25519FromWire extracts the Ed25519 public key from its OpenSSH wire form.
//
// Parameters:
//   - `wire`: the OpenSSH wire-format public key.
//
// Returns:
//   - `ed25519.PublicKey`: the extracted key.
//   - `error`: non-nil when the wire form does not parse to an ssh-ed25519 key.
func ed25519FromWire(wire []byte) (ed25519.PublicKey, error) {

	parsed, err := ssh.ParsePublicKey(wire)
	if err != nil {
		return nil, err
	}

	cryptoKey, ok := parsed.(ssh.CryptoPublicKey)
	if !ok {
		return nil, fmt.Errorf("key type %s does not expose a crypto key", parsed.Type())
	}

	public, ok := cryptoKey.CryptoPublicKey().(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("key type %s is not ed25519", parsed.Type())
	}

	return public, nil
}

// trustedPrincipal resolves a publisher key against the verifier's `allowed_signers` file.
//
// The file format is OpenSSH's (one line: `principals [options] keytype base64-key [comment]`), parsed by
// devlore itself per the settled raw-signature decision. Supported options: `namespaces="a,b"` (glob patterns
// scope the key to signature namespaces) and `valid-after="YYYYMMDD"` / `valid-before="YYYYMMDD"` windows.
// `cert-authority` lines are recognized and skipped — SSH-certificate trust is a chartered follow-up of the
// default tier.
//
// Parameters:
//   - `publisherWire`: the signature's public key, OpenSSH wire format.
//   - `namespace`: the signature's namespace, checked against `namespaces=` scoping.
//   - `allowedSignersPath`: the trust file; "" resolves to `<config>/devlore/allowed_signers`.
//   - `at`: the verification moment for validity windows.
//
// Returns:
//   - `string`: the matched principal list (comma-separated identities).
//   - `error`: non-nil when the file is missing or no line trusts the key for this namespace and moment.
func trustedPrincipal(publisherWire []byte, namespace, allowedSignersPath string, at time.Time) (string, error) {

	if allowedSignersPath == "" {
		allowedSignersPath = xdg.ConfigPath("devlore", "allowed_signers")
	}

	data, err := os.ReadFile(allowedSignersPath)
	if err != nil {
		return "", fmt.Errorf("trust list %s: %w", allowedSignersPath, err)
	}

	for _, line := range strings.Split(string(data), "\n") {

		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		entry, ok := parseAllowedSigner(line)
		if !ok || entry.certAuthority {
			continue
		}

		if !bytes.Equal(entry.keyWire, publisherWire) {
			continue
		}
		if !entry.allowsNamespace(namespace) {
			continue
		}
		if !entry.validAt(at) {
			continue
		}

		return entry.principals, nil
	}

	return "", fmt.Errorf("publisher key is not in the trust list %s (namespace %s)", allowedSignersPath, namespace)
}

// allowedSigner is one parsed trust-list line.
type allowedSigner struct {

	// principals is the comma-separated identity list.
	principals string

	// namespaces holds the `namespaces=` glob patterns; empty allows every namespace.
	namespaces []string

	// validAfter / validBefore bound the validity window; zero values leave the bound open.
	validAfter  time.Time
	validBefore time.Time

	// certAuthority marks a CA line (recognized, unsupported in the default tier).
	certAuthority bool

	// keyWire is the trusted key in OpenSSH wire format.
	keyWire []byte
}

// allowsNamespace reports whether the entry's `namespaces=` scoping admits `namespace`.
func (a allowedSigner) allowsNamespace(namespace string) bool {

	if len(a.namespaces) == 0 {
		return true
	}
	for _, pattern := range a.namespaces {
		if matched, err := path.Match(pattern, namespace); err == nil && matched {
			return true
		}
	}
	return false
}

// validAt reports whether `at` falls inside the entry's validity window.
func (a allowedSigner) validAt(at time.Time) bool {

	if !a.validAfter.IsZero() && at.Before(a.validAfter) {
		return false
	}
	if !a.validBefore.IsZero() && !at.Before(a.validBefore) {
		return false
	}
	return true
}

// parseAllowedSigner parses one `allowed_signers` line.
//
// Parameters:
//   - `line`: the trimmed, non-comment line.
//
// Returns:
//   - `allowedSigner`: the parsed entry.
//   - `bool`: false when the line does not parse.
func parseAllowedSigner(line string) (allowedSigner, bool) {

	fields := strings.Fields(line)
	if len(fields) < 3 {
		return allowedSigner{}, false
	}

	entry := allowedSigner{principals: fields[0]}
	rest := fields[1:]

	// Options ride between the principals and the key type; the key type never contains '='.
	for len(rest) > 0 && (strings.Contains(rest[0], "=") || rest[0] == "cert-authority") {
		for _, option := range strings.Split(rest[0], ",") {
			switch {
			case option == "cert-authority":
				entry.certAuthority = true
			case strings.HasPrefix(option, "namespaces="):
				patterns := strings.Trim(strings.TrimPrefix(option, "namespaces="), `"`)
				entry.namespaces = strings.Split(patterns, ",")
			case strings.HasPrefix(option, "valid-after="):
				entry.validAfter = parseValidity(strings.Trim(strings.TrimPrefix(option, "valid-after="), `"`))
			case strings.HasPrefix(option, "valid-before="):
				entry.validBefore = parseValidity(strings.Trim(strings.TrimPrefix(option, "valid-before="), `"`))
			}
		}
		rest = rest[1:]
	}

	if len(rest) < 2 {
		return allowedSigner{}, false
	}

	keyWire, err := base64.StdEncoding.DecodeString(rest[1])
	if err != nil {
		return allowedSigner{}, false
	}
	entry.keyWire = keyWire

	return entry, true
}

// parseValidity parses ssh-keygen's YYYYMMDD[HHMM[SS]] validity form; unparseable input yields the zero time
// (an open bound).
func parseValidity(value string) time.Time {

	for _, layout := range []string{"20060102150405", "200601021504", "20060102"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t
		}
	}
	return time.Time{}
}

// endregion
