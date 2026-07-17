// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package signing

import "fmt"

// Policy governs what a consumer does with a verification [Verdict] — the settled four-tier ladder (phase-8
// step 46, question 4; prior art: PowerShell ExecutionPolicy, pacman SigLevel, Kubernetes admission
// enforce/warn/audit).
//
// Like its prior art, the policy is a safety feature, not a security boundary: it prevents accidents and
// surfaces facts; it does not stop a local adversary who can change the policy.
type Policy int

const (
	// PolicyReport verifies and reports findings — both unsigned (absence) and invalid/untrusted (failure) —
	// but refuses nothing. The floor: unsigned stores keep working while signature state becomes visible.
	PolicyReport Policy = iota

	// PolicyIgnore performs no verification at all.
	PolicyIgnore

	// PolicyRejectExternal rejects unsigned, invalid, or untrusted documents from OUTSIDE this machine's own
	// store; own-store documents behave as [PolicyReport]. The store boundary is the externality marker.
	PolicyRejectExternal

	// PolicyReject rejects every document that is not valid — unsigned, invalid, or untrusted alike.
	PolicyReject
)

// String returns the policy's config/flag value (snake_case per the config convention).
//
// Returns:
//   - `string`: "ignore", "report", "reject_external", or "reject".
func (p Policy) String() string {
	switch p {
	case PolicyIgnore:
		return "ignore"
	case PolicyReport:
		return "report"
	case PolicyRejectExternal:
		return "reject_external"
	case PolicyReject:
		return "reject"
	default:
		return "report"
	}
}

// ParsePolicy parses a config/flag policy value.
//
// Parameters:
//   - `value`: "ignore", "report", "reject_external", or "reject"; "" parses as the [PolicyReport] floor.
//
// Returns:
//   - `Policy`: the parsed policy.
//   - `error`: non-nil for any other value.
func ParsePolicy(value string) (Policy, error) {
	switch value {
	case "report", "":
		return PolicyReport, nil
	case "ignore":
		return PolicyIgnore, nil
	case "reject_external":
		return PolicyRejectExternal, nil
	case "reject":
		return PolicyReject, nil
	default:
		return PolicyReport, fmt.Errorf("invalid signing policy %q: must be ignore, report, reject_external, or reject", value)
	}
}

// Judge applies the policy to a verdict: the one enforcement point every consumer shares.
//
// Parameters:
//   - `verdict`: the artifact's verification result.
//   - `external`: whether the document came from outside this machine's own store (see [External]).
//
// Returns:
//   - `error`: the rejection when the policy refuses this verdict; nil to proceed (the caller reports the
//     verdict regardless — except under [PolicyIgnore], where there is nothing to report).
func (p Policy) Judge(verdict Verdict, external bool) error {

	if verdict.Outcome == OutcomeValid || p == PolicyIgnore || p == PolicyReport {
		return nil
	}

	if p == PolicyRejectExternal && !external {
		return nil
	}

	detail := verdict.Detail
	if detail == "" {
		detail = verdict.Outcome.String()
	}
	return fmt.Errorf("signing policy %s rejects this document: %s", p, detail)
}
