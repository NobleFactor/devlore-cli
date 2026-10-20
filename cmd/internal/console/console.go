// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package console

import (
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
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

// Run executes an interactive session on the alternate screen.
// Returns the session's result after completion, or an error.
func (c *Console) Run(session Session) (any, error) {
	model := c.newModel(session, true)

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

// RunInline executes a session without alternate screen.
// Useful for simpler interactions that don't need full-screen mode.
func (c *Console) RunInline(session Session) (any, error) {
	model := c.newModel(session, false)

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

// newModel returns the model for a session, in the console's styles.
//
// Parameters:
//   - `session`: the session the model runs.
//   - `altScreen`: whether the session runs on the alternate screen.
//
// Returns:
//   - `*Model`: the model.
func (c *Console) newModel(session Session, altScreen bool) *Model {
	model := NewModel(session)
	model.SetStyles(c.styles)
	model.altScreen = altScreen
	return model
}

// Styles returns the console's styles.
func (c *Console) Styles() *Styles {
	return c.styles
}

// colorWriter returns the console's output behind a writer that adapts color to the terminal.
//
// Lip Gloss v2 renders full color and leaves downsampling to the writer. Outside a Bubble Tea program, which
// downsamples itself, the console's own writes go through colorprofile, which detects the terminal and honors
// `NO_COLOR` and `CLICOLOR`.
//
// Returns:
//   - `io.Writer`: the output, downsampled for the terminal it reaches.
func (c *Console) colorWriter() io.Writer {
	return colorprofile.NewWriter(c.output, os.Environ())
}

// Print outputs styled text without a session.
func (c *Console) Print(text string) {
	//nolint:errcheck // diagnose-ignored-error: console write; see docs/architecture/2.8-eventing-infrastructure.md
	_, _ = fmt.Fprint(c.colorWriter(), text)
}

// PrintStyled outputs text with the given style.
func (c *Console) PrintStyled(text string, style func(string) string) {
	//nolint:errcheck // diagnose-ignored-error: console write; see docs/architecture/2.8-eventing-infrastructure.md
	_, _ = fmt.Fprint(c.colorWriter(), style(text))
}

// Success prints a success message.
func (c *Console) Success(msg string) {
	//nolint:errcheck // diagnose-ignored-error: console write; see docs/architecture/2.8-eventing-infrastructure.md
	_, _ = fmt.Fprintln(c.colorWriter(), c.styles.Success.Render("✓ "+msg))
}

// Warning prints a warning message.
func (c *Console) Warning(msg string) {
	//nolint:errcheck // diagnose-ignored-error: console write; see docs/architecture/2.8-eventing-infrastructure.md
	_, _ = fmt.Fprintln(c.colorWriter(), c.styles.Warning.Render("⚠ "+msg))
}

// Error prints an error message.
func (c *Console) Error(msg string) {
	//nolint:errcheck // diagnose-ignored-error: console write; see docs/architecture/2.8-eventing-infrastructure.md
	_, _ = fmt.Fprintln(c.colorWriter(), c.styles.Error.Render("✗ "+msg))
}

// Info prints an info message.
func (c *Console) Info(msg string) {
	//nolint:errcheck // diagnose-ignored-error: console write; see docs/architecture/2.8-eventing-infrastructure.md
	_, _ = fmt.Fprintln(c.colorWriter(), c.styles.Highlighted.Render("ℹ "+msg))
}
