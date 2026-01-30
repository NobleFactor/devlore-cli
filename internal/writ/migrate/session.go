// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package migrate

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/NobleFactor/devlore-cli/internal/console"
	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/model"
	"github.com/NobleFactor/devlore-cli/internal/lorepackage"
)

// SessionState represents a state in the migration state machine.
type SessionState int

const (
	StateWelcome SessionState = iota
	StateDetecting
	StateShowAnalysis
	StateConfirmRenames
	StateConfirmSecrets
	StatePreview
	StateConfirmExecute
	StateExecuting
	StateComplete
	StateError
)

// Session implements console.Session for interactive migration.
type Session struct {
	// Configuration
	opts Options

	// State machine
	state SessionState
	step  *console.Step

	// Analysis results
	graph    *execution.Graph
	analysis *MigrationAnalysis

	// User decisions
	confirmRenames bool
	confirmSecrets bool

	// Output
	result *SessionResult
	err    error
}

// SessionResult is the final output of a successful migration session.
type SessionResult struct {
	// Graph is the execution graph.
	Graph *execution.Graph

	// Analysis is the migration analysis.
	Analysis *MigrationAnalysis

	// Executed indicates whether the migration was executed.
	Executed bool
}

// NewSession creates a new migration session.
func NewSession(opts Options) *Session {
	return &Session{
		opts:  opts,
		state: StateWelcome,
	}
}

// NewSessionWithProvider creates a session with an AI provider for enhanced analysis.
func NewSessionWithProvider(opts Options, provider model.Provider, regClient *lorepackage.Registry) *Session {
	opts.Provider = provider
	opts.RegClient = regClient
	return NewSession(opts)
}

// Next advances the session and returns the next step.
func (s *Session) Next() *console.Step {
	switch s.state {
	case StateWelcome:
		s.step = s.welcomeStep()
	case StateDetecting:
		s.step = s.detectingStep()
	case StateShowAnalysis:
		s.step = s.showAnalysisStep()
	case StateConfirmRenames:
		s.step = s.confirmRenamesStep()
	case StateConfirmSecrets:
		s.step = s.confirmSecretsStep()
	case StatePreview:
		s.step = s.previewStep()
	case StateConfirmExecute:
		s.step = s.confirmExecuteStep()
	case StateExecuting:
		s.step = s.executingStep()
	case StateComplete:
		s.step = s.completeStep()
	case StateError:
		s.step = &console.Step{
			Type:  console.StepError,
			Title: "Migration Failed",
			Error: s.err,
		}
	}
	return s.step
}

// Respond processes the user's response to the current step.
func (s *Session) Respond(response string) error {
	switch s.state {
	case StateWelcome:
		s.state = StateDetecting
	case StateShowAnalysis:
		s.state = StateConfirmRenames
	case StateConfirmRenames:
		s.confirmRenames = (response == "yes")
		s.state = StateConfirmSecrets
	case StateConfirmSecrets:
		s.confirmSecrets = (response == "yes")
		s.state = StatePreview
	case StatePreview:
		s.state = StateConfirmExecute
	case StateConfirmExecute:
		if response == "yes" {
			s.state = StateExecuting
		} else {
			s.result = &SessionResult{
				Graph:    s.graph,
				Analysis: s.analysis,
				Executed: false,
			}
			s.state = StateComplete
		}
	case StateComplete, StateError:
		// Terminal states - no action
	}
	return nil
}

// Current returns the current step.
func (s *Session) Current() *console.Step {
	return s.step
}

// Complete returns true if the session has finished.
func (s *Session) Complete() bool {
	return s.state == StateComplete || s.state == StateError
}

// Result returns the migration result.
func (s *Session) Result() any {
	if s.result == nil {
		return nil
	}
	return s.result
}

// Error returns any error that terminated the session.
func (s *Session) Error() error {
	return s.err
}

// welcomeStep creates the welcome step.
func (s *Session) welcomeStep() *console.Step {
	content := `# Writ Migration

Welcome to the writ migration wizard. This tool will help you convert your
existing dotfiles into writ-managed configuration.

## What happens next

1. **Detection** - Identify your dotfile management system
2. **Analysis** - Analyze structure, detect secrets, suggest improvements
3. **Review** - Review and confirm proposed changes
4. **Execution** - Apply the migration (or export the plan)

Press **Enter** to begin.
`
	return &console.Step{
		Type:    console.StepInfo,
		Title:   "Welcome",
		Content: content,
	}
}

// detectingStep runs detection and analysis.
func (s *Session) detectingStep() *console.Step {
	// Run the full detection and analysis pipeline
	ctx := context.Background()
	graph, analysis, err := BuildMigration(ctx, s.opts)
	if err != nil {
		s.err = err
		s.state = StateError
		return s.Next()
	}

	s.graph = graph
	s.analysis = analysis
	s.state = StateShowAnalysis

	return &console.Step{
		Type:     console.StepProgress,
		Title:    "Analyzing",
		Content:  fmt.Sprintf("Analyzing `%s`...", s.opts.SourceRoot),
		Progress: 100,
	}
}

// showAnalysisStep displays the analysis results.
func (s *Session) showAnalysisStep() *console.Step {
	var content strings.Builder
	content.WriteString(fmt.Sprintf("## Source: %s\n\n", s.analysis.SourceRoot))
	content.WriteString(fmt.Sprintf("**System:** %s", s.analysis.System))
	if s.analysis.SystemConfidence > 0 {
		content.WriteString(fmt.Sprintf(" (%.0f%% confidence)", s.analysis.SystemConfidence*100))
	}
	content.WriteString("\n\n")

	// Summary statistics
	st := s.analysis.Stats
	content.WriteString("### Summary\n\n")
	content.WriteString(fmt.Sprintf("- **Files:** %d\n", st.TotalFiles))
	content.WriteString(fmt.Sprintf("- **Projects:** %d\n", st.Projects))
	content.WriteString(fmt.Sprintf("- **Platforms:** %d\n", st.Platforms))
	if st.LifecycleScripts > 0 {
		content.WriteString(fmt.Sprintf("- **Lifecycle Scripts:** %d\n", st.LifecycleScripts))
	}
	if st.Secrets > 0 {
		content.WriteString(fmt.Sprintf("- **Secrets:** %d\n", st.Secrets))
	}
	content.WriteString("\n")

	// Observations from analysis
	if len(s.analysis.Observations) > 0 {
		content.WriteString("### Observations\n\n")
		for _, obs := range s.analysis.Observations {
			content.WriteString(fmt.Sprintf("- %s\n", obs))
		}
		content.WriteString("\n")
	}

	// Warnings
	if len(s.analysis.Warnings) > 0 {
		content.WriteString("### ⚠️ Warnings\n\n")
		for _, warn := range s.analysis.Warnings {
			content.WriteString(fmt.Sprintf("- %s\n", warn))
		}
		content.WriteString("\n")
	}

	return &console.Step{
		Type:    console.StepInfo,
		Title:   "Analysis Results",
		Content: content.String(),
	}
}

// confirmRenamesStep confirms directory renames.
func (s *Session) confirmRenamesStep() *console.Step {
	renameNodes := filterNodesByOp(s.graph, "rename")
	if len(renameNodes) == 0 {
		s.confirmRenames = true
		s.state = StateConfirmSecrets
		return s.Next()
	}

	var content strings.Builder
	content.WriteString("## Directory Renames\n\n")
	content.WriteString("The following directories will be renamed to match writ naming conventions:\n\n")
	content.WriteString("```\n")
	for _, node := range renameNodes {
		source := shortenPath(node.Source, s.analysis.SourceRoot)
		target := shortenPath(node.Target, s.analysis.SourceRoot)
		content.WriteString(fmt.Sprintf("%s  →  %s\n", source, target))
	}
	content.WriteString("```\n\n")
	content.WriteString("Proceed with renames?")

	return &console.Step{
		Type:    console.StepConfirm,
		Title:   "Renames",
		Content: content.String(),
		Default: "yes",
	}
}

// confirmSecretsStep confirms secret handling.
func (s *Session) confirmSecretsStep() *console.Step {
	if len(s.analysis.SecretFindings) == 0 {
		s.confirmSecrets = true
		s.state = StatePreview
		return s.Next()
	}

	var content strings.Builder
	content.WriteString("## Secrets Detected\n\n")

	// Group by encryption status
	var encrypted, unencrypted []SecretFinding
	for _, sf := range s.analysis.SecretFindings {
		if sf.Encryption != EncryptNone {
			encrypted = append(encrypted, sf)
		} else {
			unencrypted = append(unencrypted, sf)
		}
	}

	if len(unencrypted) > 0 {
		content.WriteString("### 🔓 Unencrypted secrets\n\n")
		for _, sf := range unencrypted {
			content.WriteString(fmt.Sprintf("- `%s`\n", sf.RelPath))
			if sf.Reason != "" {
				content.WriteString(fmt.Sprintf("  %s\n", sf.Reason))
			}
		}
		content.WriteString("\n")
	}

	if len(encrypted) > 0 {
		content.WriteString("### 🔐 Already encrypted\n\n")
		for _, sf := range encrypted {
			content.WriteString(fmt.Sprintf("- `%s` (%s)\n", sf.RelPath, sf.Encryption))
		}
		content.WriteString("\n")
	}

	content.WriteString("Continue with migration? (Secrets should be encrypted post-migration)")

	return &console.Step{
		Type:    console.StepConfirm,
		Title:   "Secrets",
		Content: content.String(),
		Default: "yes",
	}
}

// previewStep shows the full migration plan.
func (s *Session) previewStep() *console.Step {
	var buf bytes.Buffer
	_ = FormatMigrationPlan(&buf, s.graph, s.analysis, "text")

	return &console.Step{
		Type:    console.StepInfo,
		Title:   "Migration Plan",
		Content: buf.String(),
	}
}

// confirmExecuteStep asks for final confirmation.
func (s *Session) confirmExecuteStep() *console.Step {
	verb := "Execute"
	if s.opts.Execute {
		verb = "Execute"
	} else {
		verb = "Export plan for"
	}

	content := fmt.Sprintf("Ready to proceed.\n\n**%s migration?**", verb)

	return &console.Step{
		Type:    console.StepConfirm,
		Title:   "Confirm",
		Content: content,
		Default: "yes",
	}
}

// executingStep performs the migration.
func (s *Session) executingStep() *console.Step {
	if s.opts.Execute {
		reg := execution.NewOperationRegistry()
		opts := execution.ExecutorOptions{
			DryRun: false,
		}
		eng := execution.NewGraphExecutor(reg, opts)

		// Run the graph
		_, err := eng.RunNodes(context.Background(), toExecutables(s.graph.Nodes), s.graph.Edges)
		if err != nil {
			s.err = fmt.Errorf("execution failed: %w", err)
			s.state = StateError
			return s.Next()
		}
	}

	s.result = &SessionResult{
		Graph:    s.graph,
		Analysis: s.analysis,
		Executed: s.opts.Execute,
	}
	s.state = StateComplete

	return &console.Step{
		Type:     console.StepProgress,
		Title:    "Migrating",
		Content:  "Executing migration...",
		Progress: 100,
	}
}

// toExecutables converts a slice of *execution.Node to []execution.Executable.
func toExecutables(nodes []*execution.Node) []execution.Executable {
	executables := make([]execution.Executable, len(nodes))
	for i, n := range nodes {
		executables[i] = n
	}
	return executables
}

// completeStep shows the completion message.
func (s *Session) completeStep() *console.Step {
	var content strings.Builder

	if s.result.Executed {
		content.WriteString("## Migration Complete ✓\n\n")
	} else {
		content.WriteString("## Migration Plan Ready\n\n")
		content.WriteString("The migration was not executed (dry run).\n\n")
	}

	content.WriteString("### Next Steps\n\n")
	for i, rec := range s.analysis.Recommendations {
		content.WriteString(fmt.Sprintf("%d. %s\n", i+1, rec))
	}

	return &console.Step{
		Type:    console.StepComplete,
		Title:   "Complete",
		Content: content.String(),
	}
}
