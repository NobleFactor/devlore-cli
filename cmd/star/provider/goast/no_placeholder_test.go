// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package goast

import (
	"strings"
	"testing"
)

// undocumentedSource is a function the styler cannot summarize: there is no prose anywhere to draw a summary
// from, and neither a parameter's meaning nor a return value's is in its type.
const undocumentedSource = `// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package example

func Locate(root string, depth int) (string, error) {
	return root, nil
}
`

// TestFixWritesNoPlaceholder pins that the styler leaves a violation standing rather than silencing it.
//
// The styler emitted `// Locate TODO(go-style): add summary` for a missing doc comment, and a
// `Parameters:` section whose every item read `name: TODO(go-style): add description`. Both satisfy the
// compliance check -- `comment.present` becomes true, and the section check is
// `strings.Contains(text, "Parameters:")` -- while documenting nothing.
//
// Over the debt this was about to be run against that is **2,057** reported violations converted into 2,057
// TODO comments, with the tree reported clean and the linter that would otherwise have asked for them no
// longer able to see them. A gate a placeholder can satisfy is not a gate.
//
// Where the styler has nothing true to say it says nothing, and the violation stays visible for a person to
// answer (#994).
func TestFixWritesNoPlaceholder(t *testing.T) {

	got := cleanupAndSave(t, undocumentedSource)

	if strings.Contains(got, "TODO(go-style)") {
		t.Errorf("the styler wrote a placeholder instead of leaving the violation standing:\n%s", got)
	}
}

// TestFixLeavesTheViolationReportable pins the other half, which a check for absent text cannot: the
// violation must still be THERE.
//
// Deleting the placeholder would be worthless if the styler instead emitted an empty doc comment, or a
// `Parameters:` header with no items -- either would satisfy the compliance check just as the TODO did. So
// this asserts the outcome rather than the absence: after the styler has run, the declaration is still
// reported as missing its doc comment.
func TestFixLeavesTheViolationReportable(t *testing.T) {

	sf, err := parseSourceFile(undocumentedSource)
	if err != nil {
		t.Fatalf("parseSourceFile: %v", err)
	}

	before := violationMessages(sf.CheckCompliance())

	if len(before) == 0 {
		t.Fatalf("the fixture is not a fixture: it reports no violations to begin with")
	}

	styled, err := parseSourceFile(cleanupAndSave(t, undocumentedSource))
	if err != nil {
		t.Fatalf("parseSourceFile after styling: %v", err)
	}

	after := violationMessages(styled.CheckCompliance())

	if len(after) != len(before) {
		t.Errorf("the styler changed what is reportable.\n  before (%d):\n    %s\n  after (%d):\n    %s",
			len(before), strings.Join(before, "\n    "),
			len(after), strings.Join(after, "\n    "))
	}
}

// violationMessages reduces a compliance report to its messages, so a test can compare reports without
// depending on the order of the fields inside one.
//
// Parameters:
//   - `violations`: the report from [SourceFile.CheckCompliance].
//
// Returns:
//   - `[]string`: one message per violation, in the order reported.
func violationMessages(violations []ComplianceViolation) []string {

	messages := make([]string, 0, len(violations))

	for _, violation := range violations {
		messages = append(messages, violation.Message)
	}

	return messages
}
