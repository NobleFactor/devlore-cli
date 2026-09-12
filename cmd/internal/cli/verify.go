// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/cmd/internal/devlore"
	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/iox"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/signing"
)

// region SUPPORTING TYPES

// VerifyConfig carries the resolved settings for one verification: the documents, the policy and the trust list.
//
// Each document is decoded (a definition loads through [op.LoadGraph], whose integrity check also validates the
// checksum; a trace decodes directly), re-canonicalized, and verified under the settled model: a raw ssh-ed25519
// signature over the namespace-prefixed canonical bytes, the publisher resolved against the verifier's
// `allowed_signers`. What happens to each outcome is the [signing.Policy] ladder: every verdict is reported, and
// the exit status is non-zero only when the policy rejects a document (phase-8 step 46; the shared `workflow
// verify` since #782).
type VerifyConfig struct {

	// Paths are the documents to verify.
	Paths []string

	// Policy governs what each verdict does to the exit status.
	Policy signing.Policy

	// AllowedSigners overrides the trust-list path; "" uses the default (`<config>/devlore/allowed_signers`).
	AllowedSigners string
}

// VerifyReport is one document's verification report.
type VerifyReport struct {

	// Path is the document as given.
	Path string `json:"path"`

	// Kind is "definition" or "trace": a workflow's definition, or one of its execution traces.
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

// endregion

// region EXPORTED FUNCTIONS

// VerifyDocuments verifies every document in `cfg.Paths` and returns one report per document.
//
// Parameters:
//   - `ctx`: the context for definition loading.
//   - `cfg`: the resolved verification configuration.
//
// Returns:
//   - `[]VerifyReport`: one report per verified document, in input order, for the caller to render.
//   - `error`: non-nil when a document cannot be read or decoded, or when the policy rejects any document; the
//     reports are returned alongside a rejection, since a rejection is the answer and not a reason to withhold it.
func VerifyDocuments(ctx context.Context, cfg *VerifyConfig) ([]VerifyReport, error) {

	// Empty, never nil: an empty selection is an empty result, and `-o json` renders `[]`, not `null` (§8, S8).
	reports := make([]VerifyReport, 0, len(cfg.Paths))
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

// endregion

// region HELPER FUNCTIONS

// verifyDocument decodes one document, re-canonicalizes it, and verifies its signature.
//
// Parameters:
//   - `ctx`: the context for the loading environment.
//   - `cfg`: the verification configuration (trust-list override).
//   - `path`: the document to verify.
//
// Returns:
//   - `VerifyReport`: the verification report.
//   - `error`: non-nil when the document cannot be read or decoded as a definition or a trace.
func verifyDocument(ctx context.Context, cfg *VerifyConfig, path string) (VerifyReport, error) {

	data, err := os.ReadFile(path)
	if err != nil {
		return VerifyReport{}, err
	}

	report := VerifyReport{
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
		return VerifyReport{}, fmt.Errorf("%s: not a YAML document: %w", path, err)
	}

	var signature *op.Signature
	var canonical []byte
	var namespace string

	switch {
	case sniff.Kind == op.GraphKind:
		report.Kind = "definition"
		namespace = signing.NamespaceGraph

		graph, err := loadDefinition(ctx, data)
		if err != nil {
			// The load path's integrity check refuses altered documents before any signature look; that IS an
			// invalid verdict, not a command failure.
			report.Outcome = signing.OutcomeInvalid.String()
			report.Detail = err.Error()
			//nolint:nilerr // an integrity-refused document IS the invalid verdict, not a command failure.
			return report, nil
		}
		signature = graph.Signature()
		if canonical, err = graph.CanonicalContent(); err != nil {
			return VerifyReport{}, err
		}

	case sniff.RunStatus != nil:
		report.Kind = "trace"
		namespace = signing.NamespaceTrace

		// Only the signature field decodes through the struct; the canonical bytes come from the RAW document (the
		// typed trace decode is lossy, with custom stack unmarshaling, and must not feed canonicalization).
		var envelope struct {
			Signature *op.Signature `yaml:"signature"`
		}
		if err := yaml.Unmarshal(data, &envelope); err != nil {
			return VerifyReport{}, fmt.Errorf("%s: not a trace document: %w", path, err)
		}
		signature = envelope.Signature
		if canonical, err = signing.CanonicalDocument(data); err != nil {
			return VerifyReport{}, err
		}

	default:
		return VerifyReport{}, fmt.Errorf(
			"%s: neither a workflow definition (kind %q) nor an execution trace", path, sniff.Kind)
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
//
// Parameters:
//   - `report`: the report.
//
// Returns:
//   - `signing.Verdict`: the verdict the report carries.
func verdictOf(report VerifyReport) signing.Verdict {

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

// loadDefinition loads a workflow definition through the sealed load path, integrity-checked.
//
// Parameters:
//   - `ctx`: the context for the loading environment.
//   - `data`: the document bytes.
//
// Returns:
//   - `*op.Graph`: the loaded definition.
//   - `error`: non-nil when loading, including the checksum integrity check, fails.
func loadDefinition(ctx context.Context, data []byte) (graph *op.Graph, err error) {

	var environment *op.RuntimeEnvironment

	environment, err = op.NewRuntimeEnvironment(ctx, op.NewRuntimeEnvironmentSpec("verify").
		WithStatus(UI()).
		WithRoot(string(filepath.Separator)).
		WithApplication(&application.Application{Name: "verify"}))
	if err != nil {
		return nil, err
	}

	defer iox.Close(&err, environment)
	return op.LoadGraph(environment, data, "yaml")
}

// endregion
