// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package console

import (
	"fmt"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// Console manages interactive terminal sessions.
type Console struct {
	input  io.Reader
	output io.Writer
	styles *Styles
}

// New creates a new Console with default settings.
func New() *Console {
	return &Console{
		input:  os.Stdin,
		output: os.Stdout,
		styles: DefaultStyles(),
	}
}

// WithInput sets the input reader.
func (c *Console) WithInput(r io.Reader) *Console {
	c.input = r
	return c
}

// WithOutput sets the output writer.
func (c *Console) WithOutput(w io.Writer) *Console {
	c.output = w
	return c
}

// WithStyles sets custom styles.
func (c *Console) WithStyles(s *Styles) *Console {
	c.styles = s
	return c
}

// Run executes an interactive session.
// Returns the session's result after completion, or an error.
func (c *Console) Run(session Session) (any, error) {
	model := NewModel(session)
	model.SetStyles(c.styles)

	p := tea.NewProgram(model,
		tea.WithInput(c.input),
		tea.WithOutput(c.output),
		tea.WithAltScreen(),
	)

	finalModel, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("console: %w", err)
	}

	m, ok := finalModel.(*Model)
	if !ok {
		return nil, fmt.Errorf("console: unexpected model type %T", finalModel)
	}
	if m.err != nil {
		return nil, m.err
	}

	if session.Error() != nil {
		return nil, session.Error()
	}

	return session.Result(), nil
}

// RunInline executes a session without alternate screen.
// Useful for simpler interactions that don't need full-screen mode.
func (c *Console) RunInline(session Session) (any, error) {
	model := NewModel(session)
	model.SetStyles(c.styles)

	p := tea.NewProgram(model,
		tea.WithInput(c.input),
		tea.WithOutput(c.output),
	)

	finalModel, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("console: %w", err)
	}

	m, ok := finalModel.(*Model)
	if !ok {
		return nil, fmt.Errorf("console: unexpected model type %T", finalModel)
	}
	if m.err != nil {
		return nil, m.err
	}

	if session.Error() != nil {
		return nil, session.Error()
	}

	return session.Result(), nil
}

// Styles returns the console's styles.
func (c *Console) Styles() *Styles {
	return c.styles
}

// Print outputs styled text without a session.
func (c *Console) Print(text string) {
	_, _ = fmt.Fprint(c.output, text) //nolint:errcheck
}

// PrintStyled outputs text with the given style.
func (c *Console) PrintStyled(text string, style func(string) string) {
	_, _ = fmt.Fprint(c.output, style(text)) //nolint:errcheck
}

// Success prints a success message.
func (c *Console) Success(msg string) {
	_, _ = fmt.Fprintln(c.output, c.styles.Success.Render("✓ "+msg)) //nolint:errcheck
}

// Warning prints a warning message.
func (c *Console) Warning(msg string) {
	_, _ = fmt.Fprintln(c.output, c.styles.Warning.Render("⚠ "+msg)) //nolint:errcheck
}

// Error prints an error message.
func (c *Console) Error(msg string) {
	_, _ = fmt.Fprintln(c.output, c.styles.Error.Render("✗ "+msg)) //nolint:errcheck
}

// Info prints an info message.
func (c *Console) Info(msg string) {
	_, _ = fmt.Fprintln(c.output, c.styles.Highlighted.Render("ℹ "+msg)) //nolint:errcheck
}
