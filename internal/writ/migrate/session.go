// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package migrate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/internal/console"
	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/lorepackage"
	"github.com/NobleFactor/devlore-cli/internal/model"
)

// SessionState represents a state in the migration session.
type SessionState int

const (
	StateAnalyzing SessionState = iota
	StateConversing
	StatePlanProposed
	StateExecuting
	StateComplete
	StateError
)

// Session implements console.Session for interactive migration.
type Session struct {
	opts Options

	state    SessionState
	step     *console.Step
	err      error
	result   *SessionResult

	// Analysis results
	graph    *execution.Graph
	analysis *MigrationAnalysis

	// Conversation state
	history    []model.Message
	aiResponse string
}

// SessionResult is the final output of a migration session.
type SessionResult struct {
	Graph       *execution.Graph
	Analysis    *MigrationAnalysis
	Executed    bool
	ReceiptPath string
}

// NewSession creates a new migration session.
func NewSession(opts Options) *Session {
	return &Session{
		opts:    opts,
		state:   StateAnalyzing,
		history: []model.Message{},
	}
}

// NewSessionWithProvider creates a session with an AI provider.
func NewSessionWithProvider(opts Options, provider model.Provider, regClient *lorepackage.Registry) *Session {
	opts.Provider = provider
	opts.RegClient = regClient
	return NewSession(opts)
}

// Next advances the session and returns the next step.
func (s *Session) Next() *console.Step {
	switch s.state {
	case StateAnalyzing:
		s.step = s.runAnalysis()
	case StateConversing:
		s.step = s.conversationStep()
	case StatePlanProposed:
		s.step = s.planConfirmStep()
	case StateExecuting:
		s.step = s.executeStep()
	case StateComplete:
		s.step = s.completeStep()
	case StateError:
		s.step = &console.Step{
			Type:  console.StepError,
			Title: "Error",
			Error: s.err,
		}
	}
	return s.step
}

// Respond processes the user's input.
func (s *Session) Respond(response string) error {
	response = strings.TrimSpace(response)

	// Handle slash commands
	if strings.HasPrefix(response, "/") {
		return s.handleSlashCommand(response)
	}

	switch s.state {
	case StateConversing:
		return s.processConversation(response)
	case StatePlanProposed:
		return s.processPlanResponse(response)
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

// Result returns the session result.
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

// runAnalysis performs initial detection and analysis.
func (s *Session) runAnalysis() *console.Step {
	ctx := context.Background()
	graph, analysis, err := BuildMigration(ctx, s.opts)
	if err != nil {
		s.err = err
		s.state = StateError
		return s.Next()
	}

	s.graph = graph
	s.analysis = analysis

	// Generate initial AI response with findings and recommendations
	s.aiResponse = s.generateInitialResponse()
	s.state = StateConversing

	return &console.Step{
		Type:     console.StepProgress,
		Title:    "Analyzing",
		Content:  fmt.Sprintf("Analyzing `%s`...", s.opts.SourceRoot),
		Progress: 100,
	}
}

// generateInitialResponse creates the AI's initial assessment.
func (s *Session) generateInitialResponse() string {
	if s.opts.Provider == nil {
		return s.generateStaticInitialResponse()
	}

	ctx := context.Background()
	analysisJSON, _ := json.MarshalIndent(s.analysis, "", "  ")

	prompt := `You are helping the user migrate their environment to writ.
You have just analyzed their directory. Present your findings conversationally:
1. What you detected (source system, structure)
2. Key observations (projects, platforms, scripts, secrets)
3. Any warnings or concerns
4. Your recommendations for how to proceed

Be concise but helpful. End by asking how they'd like to proceed.
Do not output JSON. Write in a friendly, conversational tone.`

	resp, err := s.opts.Provider.Chat(ctx, model.ChatRequest{
		SystemPrompt: prompt,
		Messages: []model.Message{
			{Role: model.RoleUser, Content: fmt.Sprintf("Here's the analysis:\n\n%s", string(analysisJSON))},
		},
		Temperature: 0.3,
	})
	if err != nil {
		return s.generateStaticInitialResponse()
	}

	return resp.Content
}

// generateStaticInitialResponse creates a non-AI initial response.
func (s *Session) generateStaticInitialResponse() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("## Analysis of %s\n\n", s.opts.SourceRoot))
	sb.WriteString(fmt.Sprintf("**Detected system:** %s", s.analysis.System))
	if s.analysis.SystemConfidence > 0 {
		sb.WriteString(fmt.Sprintf(" (%.0f%% confidence)", s.analysis.SystemConfidence*100))
	}
	sb.WriteString("\n\n")

	st := s.analysis.Stats
	sb.WriteString(fmt.Sprintf("**Files:** %d | **Projects:** %d | **Platforms:** %d\n\n",
		st.TotalFiles, st.Projects, st.Platforms))

	if len(s.analysis.Observations) > 0 {
		sb.WriteString("### Observations\n")
		for _, obs := range s.analysis.Observations {
			sb.WriteString(fmt.Sprintf("- %s\n", obs))
		}
		sb.WriteString("\n")
	}

	if len(s.analysis.Warnings) > 0 {
		sb.WriteString("### Warnings\n")
		for _, warn := range s.analysis.Warnings {
			sb.WriteString(fmt.Sprintf("- %s\n", warn))
		}
		sb.WriteString("\n")
	}

	if len(s.analysis.Recommendations) > 0 {
		sb.WriteString("### Recommendations\n")
		for _, rec := range s.analysis.Recommendations {
			sb.WriteString(fmt.Sprintf("- %s\n", rec))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("How would you like to proceed?")
	return sb.String()
}

// conversationStep returns the current conversation step.
func (s *Session) conversationStep() *console.Step {
	return &console.Step{
		Type:    console.StepInput,
		Title:   "Migration",
		Content: s.aiResponse,
		Default: "",
	}
}

// processConversation handles user input during conversation.
func (s *Session) processConversation(input string) error {
	if s.opts.Provider == nil {
		s.aiResponse = "AI provider not available. Use /help to see available commands."
		return nil
	}

	// Add user message to history
	s.history = append(s.history, model.Message{
		Role:    model.RoleUser,
		Content: input,
	})

	// Build context for AI including both analysis and current execution graph
	ctx := context.Background()
	analysisJSON, _ := json.MarshalIndent(s.analysis, "", "  ")
	graphJSON := s.formatGraphForPrompt()

	systemPrompt := fmt.Sprintf(`You are helping the user migrate their environment to writ.

## Current Analysis
%s

## Current Execution Graph
%s

## Your Role
1. Answer questions about the analysis and planned operations
2. Help the user refine the execution graph through conversation
3. When the user wants to modify the graph, respond with changes in this format:

   **Graph Modifications:**
   - ADD_RENAME: source_path -> target_path
   - REMOVE_RENAME: source_path

   [Your explanation of the changes]

4. When the user is satisfied and wants to proceed, include:

   **Ready to Execute**

   The migration will perform the following renames:
   [list the renames]

   Type "approve" to execute or continue refining.

## Available Modifications
- ADD_RENAME: Add a directory rename (source -> target)
- REMOVE_RENAME: Remove a planned rename by source path

## Writ Conventions
- Directory naming: <group>.<Platform> (e.g., all.Darwin, noblefactor.Unix)
- Groups typically live in Home/Configs/ or Home/<project>/

Be conversational and helpful. Explain your reasoning.`, string(analysisJSON), graphJSON)

	// Call AI
	resp, err := s.opts.Provider.Chat(ctx, model.ChatRequest{
		SystemPrompt: systemPrompt,
		Messages:     s.history,
		Temperature:  0.3,
	})
	if err != nil {
		s.aiResponse = fmt.Sprintf("Error communicating with AI: %v", err)
		return nil
	}

	// Parse and apply any graph modifications from the response
	s.applyGraphModifications(resp.Content)

	s.aiResponse = resp.Content

	// Add assistant response to history
	s.history = append(s.history, model.Message{
		Role:    model.RoleAssistant,
		Content: resp.Content,
	})

	// Check if this looks like ready-to-execute
	if s.detectReadyToExecute(resp.Content) {
		s.state = StatePlanProposed
	}

	return nil
}

// formatGraphForPrompt formats the execution graph for the AI prompt.
func (s *Session) formatGraphForPrompt() string {
	if s.graph == nil || len(s.graph.Nodes) == 0 {
		return "No operations planned (repository may already be writ-compatible)"
	}

	var sb strings.Builder
	sb.WriteString("Planned renames:\n")
	for _, node := range s.graph.Nodes {
		for _, op := range node.Operations {
			if op == "move" {
				// Show relative paths for readability
				source := strings.TrimPrefix(node.GetSlot("source"), s.opts.SourceRoot+"/")
				target := strings.TrimPrefix(node.GetSlot("path"), s.opts.SourceRoot+"/")
				sb.WriteString(fmt.Sprintf("  %s -> %s\n", source, target))
			}
		}
	}
	return sb.String()
}

// applyGraphModifications parses AI response for graph modifications and applies them.
func (s *Session) applyGraphModifications(content string) {
	lines := strings.Split(content, "\n")
	inModifications := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.Contains(strings.ToLower(line), "graph modifications:") {
			inModifications = true
			continue
		}

		// Stop parsing modifications when we hit another section
		if inModifications && strings.HasPrefix(line, "**") && !strings.Contains(strings.ToLower(line), "graph") {
			inModifications = false
			continue
		}

		if !inModifications {
			continue
		}

		// Parse ADD_RENAME: source -> target
		if strings.HasPrefix(line, "- ADD_RENAME:") || strings.HasPrefix(line, "ADD_RENAME:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				pathParts := strings.Split(parts[1], "->")
				if len(pathParts) == 2 {
					source := strings.TrimSpace(pathParts[0])
					target := strings.TrimSpace(pathParts[1])
					s.addRenameToGraph(source, target)
				}
			}
		}

		// Parse REMOVE_RENAME: source
		if strings.HasPrefix(line, "- REMOVE_RENAME:") || strings.HasPrefix(line, "REMOVE_RENAME:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				source := strings.TrimSpace(parts[1])
				s.removeRenameFromGraph(source)
			}
		}
	}
}

// addRenameToGraph adds a rename operation to the execution graph.
func (s *Session) addRenameToGraph(source, target string) {
	// Make paths absolute if they're relative
	if !strings.HasPrefix(source, "/") {
		source = s.opts.SourceRoot + "/" + source
	}
	if !strings.HasPrefix(target, "/") {
		target = s.opts.SourceRoot + "/" + target
	}

	// Check if this rename already exists
	for _, node := range s.graph.Nodes {
		if node.GetSlot("source") == source {
			// Update existing rename
			node.SetSlotImmediate("path", target)
			return
		}
	}

	// Add new rename node
	plan := execution.NewPlan("migrate")
	newNode := plan.Rename(source, target)
	s.graph.Nodes = append(s.graph.Nodes, newNode)
}

// removeRenameFromGraph removes a rename operation from the execution graph.
func (s *Session) removeRenameFromGraph(source string) {
	// Make path absolute if relative
	if !strings.HasPrefix(source, "/") {
		source = s.opts.SourceRoot + "/" + source
	}

	// Find and remove the node
	for i, node := range s.graph.Nodes {
		if node.GetSlot("source") == source {
			s.graph.Nodes = append(s.graph.Nodes[:i], s.graph.Nodes[i+1:]...)
			return
		}
	}
}

// detectReadyToExecute checks if the AI response indicates ready to execute.
func (s *Session) detectReadyToExecute(content string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(lower, "ready to execute") ||
		(strings.Contains(lower, "approve") && strings.Contains(lower, "execute"))
}

// planConfirmStep shows the plan confirmation prompt.
func (s *Session) planConfirmStep() *console.Step {
	return &console.Step{
		Type:    console.StepInput,
		Title:   "Ready to Execute",
		Content: s.aiResponse + "\n\n_Type **approve** to execute the migration, or describe changes you'd like._",
		Default: "",
	}
}

// processPlanResponse handles user response to a proposed plan.
func (s *Session) processPlanResponse(input string) error {
	lower := strings.ToLower(strings.TrimSpace(input))

	if lower == "approve" || lower == "yes" || lower == "ok" || lower == "proceed" || lower == "👍" {
		s.state = StateExecuting
		return nil
	}

	// User wants changes - go back to conversation
	s.state = StateConversing
	return s.processConversation(input)
}

// executeStep runs the execution graph.
func (s *Session) executeStep() *console.Step {
	// Execute the graph
	reg := execution.NewOperationRegistry()
	opts := execution.ExecutorOptions{DryRun: false}
	eng := execution.NewGraphExecutor(reg, opts)

	ctx := context.Background()
	_, err := eng.RunNodes(ctx, toExecutables(s.graph.Nodes), s.graph.Edges)
	if err != nil {
		s.err = fmt.Errorf("execution failed: %w", err)
		s.state = StateError
		return s.Next()
	}

	// Write the migration marker
	if err := WriteMigratedMarker(s.opts.SourceRoot, s.graph, s.analysis); err != nil {
		cli.Warn("Failed to write migration marker: %v", err)
	}

	// Save receipt
	receiptPath, err := cli.WriteReceipt(s.graph, "writ-migrate")
	if err != nil {
		// Non-fatal - warn but continue
		s.aiResponse = fmt.Sprintf("Migration complete, but failed to save receipt: %v", err)
	} else {
		s.aiResponse = fmt.Sprintf("Migration complete. Receipt saved to:\n`%s`", receiptPath)
	}

	// Output analysis + execution_graph JSON to stdout
	s.outputJSON()

	s.result = &SessionResult{
		Graph:       s.graph,
		Analysis:    s.analysis,
		Executed:    true,
		ReceiptPath: receiptPath,
	}
	s.state = StateComplete

	return &console.Step{
		Type:     console.StepProgress,
		Title:    "Executing",
		Content:  "Executing migration...",
		Progress: 100,
	}
}

// outputJSON writes the analysis and execution graph to stdout in JSON format.
func (s *Session) outputJSON() {
	// Use the same format as FormatMigrationPlan with "json"
	var buf bytes.Buffer
	if err := FormatMigrationPlan(&buf, s.graph, s.analysis, "json"); err != nil {
		cli.Warn("Failed to format JSON output: %v", err)
		return
	}
	fmt.Fprintln(os.Stdout, buf.String())
}

// completeStep shows the completion message.
func (s *Session) completeStep() *console.Step {
	var content strings.Builder
	content.WriteString("## Migration Complete\n\n")
	content.WriteString(s.aiResponse)
	content.WriteString("\n\n")

	if len(s.analysis.Recommendations) > 0 {
		content.WriteString("### Next Steps\n")
		for i, rec := range s.analysis.Recommendations {
			content.WriteString(fmt.Sprintf("%d. %s\n", i+1, rec))
		}
	}

	content.WriteString("\n_Type /analyze to re-analyze, or /exit to finish._")

	return &console.Step{
		Type:    console.StepInput,
		Title:   "Complete",
		Content: content.String(),
	}
}

// handleSlashCommand processes slash commands.
func (s *Session) handleSlashCommand(cmd string) error {
	cmd = strings.ToLower(strings.TrimSpace(cmd))

	switch cmd {
	case "/help":
		s.aiResponse = slashCommandHelp()
		s.state = StateConversing

	case "/analyze":
		s.state = StateAnalyzing
		s.history = []model.Message{} // Reset conversation

	case "/explain":
		s.aiResponse = s.generateExplanation()
		s.state = StateConversing

	case "/exit":
		s.result = &SessionResult{
			Graph:    s.graph,
			Analysis: s.analysis,
			Executed: false,
		}
		s.state = StateComplete

	default:
		s.aiResponse = fmt.Sprintf("Unknown command: %s\n\nType /help for available commands.", cmd)
		if s.state != StateConversing {
			s.state = StateConversing
		}
	}

	return nil
}

// slashCommandHelp returns help text.
func slashCommandHelp() string {
	return `## Available Commands

- **/analyze** - Re-run analysis on the directory
- **/explain** - Get AI explanation of the current analysis
- **/help** - Show this help message
- **/exit** - Exit the migration session

Otherwise, just describe what you'd like to do in plain language.`
}

// generateExplanation creates an AI explanation of the analysis.
func (s *Session) generateExplanation() string {
	if s.analysis == nil {
		return "No analysis available. Run /analyze first."
	}

	if s.opts.Provider == nil {
		return "AI provider not available for explanation."
	}

	var buf bytes.Buffer
	ctx := context.Background()
	if err := FormatMigrationExplain(ctx, &buf, s.analysis, s.opts.Provider); err != nil {
		return fmt.Sprintf("Error generating explanation: %v", err)
	}
	return buf.String()
}

// toExecutables converts nodes to executables.
func toExecutables(nodes []*execution.Node) []execution.Executable {
	executables := make([]execution.Executable, len(nodes))
	for i, n := range nodes {
		executables[i] = n
	}
	return executables
}
