// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package verify implements `writ verify` — publisher-signature verification for graph and trace documents
// (phase-8 step 46).
//
// Each document is decoded (a graph loads through [op.LoadGraph], whose integrity check also validates the
// checksum; a trace decodes directly), re-canonicalized, and verified under the settled model: a raw
// ssh-ed25519 signature over the namespace-prefixed canonical bytes, the publisher resolved against the
// verifier's `allowed_signers`. What happens to each outcome is the [signing.Policy] ladder — the command
// reports every verdict and exits non-zero only when the policy rejects a document.
package verify

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/cmd/internal/devlore"
	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/iox"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/signing"
)

// Config carries the resolved settings for one verify invocation.
type Config struct {

	// Paths are the documents to verify.
	Paths []string

	// Policy governs what each verdict does to the exit status.
	Policy signing.Policy

	// AllowedSigners overrides the trust-list path; "" uses the default (`<config>/devlore/allowed_signers`).
	AllowedSigners string

	// JSON emits the reports as JSON instead of human-readable text.
}

// Report is one document's verification report.
type Report struct {

	// Path is the document as given.
	Path string `json:"path"`

	// Kind is "graph" or "trace".
	Kind string `json:"kind"`

	// Outcome is the verification classification.
	Outcome string `json:"outcome"`

	// Principal is the trusted publisher identity when valid.
	Principal string `json:"principal,omitempty"`

	// External marks a document from outside this machine's own store.
	External bool `json:"external"`

	// Detail elaborates non-valid outcomes.
	Detail string `json:"detail,omitempty"`

	// Rejected marks a document the policy refused.
	Rejected bool `json:"rejected,omitempty"`
}

// Execute verifies every document and presents the reports.
//
// Parameters:
//   - `ctx`: the context for graph loading.
//   - `cfg`: the resolved verify configuration.
//
// Returns:
//   - `[]Report`: one report per verified document, for the caller to render.
//   - `error`: non-nil when a document cannot be read/decoded, or when the policy rejects any document.
func Execute(ctx context.Context, cfg *Config) ([]Report, error) {

	var reports []Report
	var rejections []error

	for _, path := range cfg.Paths {

		report, err := verifyDocument(ctx, cfg, path)
		if err != nil {
			return nil, err
		}

		if judgment := cfg.Policy.Judge(verdictOf(report), report.External); judgment != nil {
			report.Rejected = true
			rejections = append(rejections, fmt.Errorf("%s: %w", path, judgment))
		}

		reports = append(reports, report)
	}

	if len(rejections) > 0 {
		return reports, fmt.Errorf("signing policy %s rejected %d document(s): %w",
			cfg.Policy, len(rejections), errors.Join(rejections...))
	}

	return reports, nil
}

// region HELPER FUNCTIONS

// verifyDocument decodes one document, re-canonicalizes it, and verifies its signature.
//
// Parameters:
//   - `ctx`: the context for the graph-loading environment.
//   - `cfg`: the verify configuration (trust-list override).
//   - `path`: the document to verify.
//
// Returns:
//   - `Report`: the verification report.
//   - `error`: non-nil when the document cannot be read or decoded as a graph or trace.
func verifyDocument(ctx context.Context, cfg *Config, path string) (Report, error) {

	data, err := os.ReadFile(path)
	if err != nil {
		return Report{}, err
	}

	report := Report{
		Path:     path,
		External: signing.External(path, devlore.StateHome()),
	}

	var sniff struct {
		Kind      string `yaml:"kind"`
		RunStatus *struct {
			Phase string `yaml:"phase"`
		} `yaml:"run_status"`
	}
	if err := yaml.Unmarshal(data, &sniff); err != nil {
		return Report{}, fmt.Errorf("%s: not a YAML document: %w", path, err)
	}

	var signature *op.Signature
	var canonical []byte
	var namespace string

	switch {
	case sniff.Kind == op.GraphKind:
		report.Kind = "graph"
		namespace = signing.NamespaceGraph

		graph, err := loadGraph(ctx, data)
		if err != nil {
			// The load path's integrity check refuses altered documents before any signature look — that IS
			// an invalid verdict, not a command failure.
			report.Outcome = signing.OutcomeInvalid.String()
			report.Detail = err.Error()
			//nolint:nilerr // an integrity-refused document IS the invalid verdict, not a command failure.
			return report, nil
		}
		signature = graph.Signature()
		if canonical, err = graph.CanonicalContent(); err != nil {
			return Report{}, err
		}

	case sniff.RunStatus != nil:
		report.Kind = "trace"
		namespace = signing.NamespaceTrace

		// Only the signature field decodes through the struct; the canonical bytes come from the RAW document
		// (the typed trace decode is lossy — custom stack unmarshaling — and must not feed canonicalization).
		var envelope struct {
			Signature *op.Signature `yaml:"signature"`
		}
		if err := yaml.Unmarshal(data, &envelope); err != nil {
			return Report{}, fmt.Errorf("%s: not a trace document: %w", path, err)
		}
		signature = envelope.Signature
		if canonical, err = signing.CanonicalDocument(data); err != nil {
			return Report{}, err
		}

	default:
		return Report{}, fmt.Errorf("%s: neither a graph (kind %q) nor a trace document", path, sniff.Kind)
	}

	verdict := signing.Verify(signature, namespace, canonical, cfg.AllowedSigners)
	report.Outcome = verdict.Outcome.String()
	report.Principal = verdict.Principal
	if report.Detail == "" {
		report.Detail = verdict.Detail
	}

	return report, nil
}

// verdictOf reconstructs the [signing.Verdict] a report was built from, for policy judgment.
func verdictOf(report Report) signing.Verdict {

	outcome := signing.OutcomeInvalid
	switch report.Outcome {
	case signing.OutcomeValid.String():
		outcome = signing.OutcomeValid
	case signing.OutcomeUnsigned.String():
		outcome = signing.OutcomeUnsigned
	case signing.OutcomeUntrusted.String():
		outcome = signing.OutcomeUntrusted
	}
	return signing.Verdict{Outcome: outcome, Principal: report.Principal, Detail: report.Detail}
}

// loadGraph loads a graph document through the sealed load path (integrity-checked).
//
// Parameters:
//   - `ctx`: the context for the loading environment.
//   - `data`: the document bytes.
//
// Returns:
//   - `*op.Graph`: the loaded graph.
//   - `error`: non-nil when loading (including the checksum integrity check) fails.
func loadGraph(ctx context.Context, data []byte) (graph *op.Graph, err error) {

	var environment *op.RuntimeEnvironment

	environment, err = op.NewRuntimeEnvironment(ctx, op.NewRuntimeEnvironmentSpec("writ").
		WithStatus(cli.UI()).
		WithRoot(string(filepath.Separator)).
		WithApplication(&application.Application{Name: "writ"}))
	if err != nil {
		return nil, err
	}

	defer iox.Close(&err, environment)
	return op.LoadGraph(environment, data, "yaml")
}

// endregion
