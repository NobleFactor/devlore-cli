// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package console

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

// Model is the Bubble Tea model for interactive sessions.
type Model struct {
	session Session
	styles  *Styles
	width   int
	height  int

	// Current step rendering
	step     *Step
	rendered string

	// Input components
	textInput     textinput.Model
	spinner       spinner.Model
	selectedIndex int

	// Markdown renderer
	renderer *glamour.TermRenderer

	// State
	quitting bool
	err      error
}

// NewModel creates a new console model for the given session.
func NewModel(session Session) *Model {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.CharLimit = 256

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	renderer, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(80),
	)

	return &Model{
		session:   session,
		styles:    DefaultStyles(),
		textInput: ti,
		spinner:   sp,
		renderer:  renderer,
		width:     80,
		height:    24,
	}
}

// Init initializes the model.
func (m *Model) Init() tea.Cmd {
	// Start the session and get the first step
	m.step = m.session.Next()
	if m.step == nil {
		m.quitting = true
		return tea.Quit
	}

	return tea.Batch(
		m.renderStep(),
		m.initInputForStep(),
	)
}

// Update handles messages.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, m.renderStep()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case stepRenderedMsg:
		m.rendered = string(msg)
		return m, nil
	}

	// Update input components
	var cmd tea.Cmd
	switch m.step.Type {
	case StepInput:
		m.textInput, cmd = m.textInput.Update(msg)
	}
	return m, cmd
}

// handleKey processes keyboard input.
func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		m.quitting = true
		return m, tea.Quit
	}

	if m.step == nil {
		return m, nil
	}

	switch m.step.Type {
	case StepInfo:
		return m.handleInfoKey(msg)
	case StepConfirm:
		return m.handleConfirmKey(msg)
	case StepSelect:
		return m.handleSelectKey(msg)
	case StepInput:
		return m.handleInputKey(msg)
	case StepComplete, StepError:
		if msg.Type == tea.KeyEnter {
			m.quitting = true
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m *Model) handleInfoKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEnter || msg.Type == tea.KeySpace {
		return m.advance("")
	}
	return m, nil
}

func (m *Model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		return m.advance("yes")
	case "n", "N":
		return m.advance("no")
	case "enter":
		// Use default
		if m.step.Default != "" {
			return m.advance(m.step.Default)
		}
	}
	return m, nil
}

func (m *Model) handleSelectKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		if m.selectedIndex > 0 {
			m.selectedIndex--
		}
	case tea.KeyDown:
		if m.selectedIndex < len(m.step.Options)-1 {
			m.selectedIndex++
		}
	case tea.KeyEnter:
		if m.selectedIndex < len(m.step.Options) {
			return m.advance(m.step.Options[m.selectedIndex].Value)
		}
	}
	return m, nil
}

func (m *Model) handleInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEnter {
		value := m.textInput.Value()
		if value == "" && m.step.Default != "" {
			value = m.step.Default
		}
		return m.advance(value)
	}
	return m, nil
}

// advance processes a response and moves to the next step.
func (m *Model) advance(response string) (tea.Model, tea.Cmd) {
	if err := m.session.Respond(response); err != nil {
		m.err = err
		m.quitting = true
		return m, tea.Quit
	}

	m.step = m.session.Next()
	if m.step == nil || m.session.Complete() {
		m.quitting = true
		return m, tea.Quit
	}

	m.selectedIndex = 0
	return m, tea.Batch(
		m.renderStep(),
		m.initInputForStep(),
	)
}

// initInputForStep prepares input components for the current step.
func (m *Model) initInputForStep() tea.Cmd {
	if m.step == nil {
		return nil
	}

	switch m.step.Type {
	case StepInput:
		m.textInput.Reset()
		m.textInput.Placeholder = m.step.Default
		m.textInput.Focus()
		return textinput.Blink
	case StepProgress:
		return m.spinner.Tick
	}
	return nil
}

// stepRenderedMsg carries rendered markdown content.
type stepRenderedMsg string

// renderStep renders the current step's markdown content.
func (m *Model) renderStep() tea.Cmd {
	return func() tea.Msg {
		if m.step == nil || m.step.Content == "" {
			return stepRenderedMsg("")
		}
		rendered, err := m.renderer.Render(m.step.Content)
		if err != nil {
			return stepRenderedMsg(m.step.Content)
		}
		return stepRenderedMsg(rendered)
	}
}

// View renders the model.
func (m *Model) View() string {
	if m.quitting && m.step == nil {
		return ""
	}

	var b strings.Builder

	// Header
	if m.step != nil && m.step.Title != "" {
		b.WriteString(m.styles.Header.Render(m.styles.Title.Render(m.step.Title)))
		b.WriteString("\n")
	}

	// Content
	if m.rendered != "" {
		b.WriteString(m.rendered)
	}

	// Step-specific rendering
	if m.step != nil {
		b.WriteString(m.renderStepInteraction())
	}

	// Footer with help
	b.WriteString(m.renderFooter())

	return m.styles.Container.Render(b.String())
}

// renderStepInteraction renders the interactive portion of the current step.
func (m *Model) renderStepInteraction() string {
	if m.step == nil {
		return ""
	}

	var b strings.Builder

	switch m.step.Type {
	case StepConfirm:
		prompt := "[y/n]"
		if m.step.Default != "" {
			if m.step.Default == "yes" {
				prompt = "[Y/n]"
			} else {
				prompt = "[y/N]"
			}
		}
		b.WriteString(m.styles.Prompt.Render(prompt))
		b.WriteString(" ")

	case StepSelect:
		b.WriteString("\n")
		for i, opt := range m.step.Options {
			cursor := "  "
			style := m.styles.Option
			if i == m.selectedIndex {
				cursor = "▸ "
				style = m.styles.SelectedOption
			}
			b.WriteString(cursor)
			b.WriteString(style.Render(opt.Label))
			if opt.Description != "" {
				b.WriteString(m.styles.Muted.Render(" - " + opt.Description))
			}
			b.WriteString("\n")
		}

	case StepInput:
		b.WriteString("\n")
		b.WriteString(m.textInput.View())
		b.WriteString("\n")

	case StepProgress:
		b.WriteString("\n")
		b.WriteString(m.spinner.View())
		b.WriteString(" ")
		b.WriteString(m.renderProgressBar())
		b.WriteString("\n")

	case StepComplete:
		b.WriteString("\n")
		b.WriteString(m.styles.Success.Render("✓ Complete"))
		b.WriteString("\n")

	case StepError:
		b.WriteString("\n")
		if m.step.Error != nil {
			b.WriteString(m.styles.Error.Render("✗ " + m.step.Error.Error()))
		} else {
			b.WriteString(m.styles.Error.Render("✗ An error occurred"))
		}
		b.WriteString("\n")
	}

	return b.String()
}

// renderProgressBar renders the progress bar.
func (m *Model) renderProgressBar() string {
	if m.step == nil {
		return ""
	}

	progress := m.step.Progress
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}

	width := 30
	filled := width * progress / 100
	empty := width - filled

	bar := m.styles.ProgressFilled.Render(strings.Repeat("█", filled)) +
		m.styles.ProgressEmpty.Render(strings.Repeat("░", empty))

	return bar + m.styles.ProgressPercent.Render(fmt.Sprintf(" %d%%", progress))
}

// renderFooter renders the help footer.
func (m *Model) renderFooter() string {
	if m.step == nil {
		return ""
	}

	var help string
	switch m.step.Type {
	case StepInfo:
		help = "Press Enter to continue"
	case StepConfirm:
		help = "Press y/n to respond"
	case StepSelect:
		help = "↑/↓ to navigate, Enter to select"
	case StepInput:
		help = "Type your response, Enter to submit"
	case StepProgress:
		help = "Please wait..."
	case StepComplete, StepError:
		help = "Press Enter to exit"
	}

	return "\n" + m.styles.Footer.Render(help+" • Ctrl+C to cancel")
}

// Styles returns the model's styles for customization.
func (m *Model) Styles() *Styles {
	return m.styles
}

// SetStyles sets custom styles.
func (m *Model) SetStyles(styles *Styles) {
	m.styles = styles
}

// WithWidth sets the terminal width.
func (m *Model) WithWidth(width int) *Model {
	m.width = width
	if m.renderer != nil {
		m.renderer, _ = glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(width-4),
		)
	}
	return m
}

// BorderedBox creates a bordered box with the given title and content.
func (m *Model) BorderedBox(title, content string) string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.styles.theme.Primary).
		Padding(1, 2)

	titleStyle := m.styles.Title.MarginBottom(1)

	return box.Render(titleStyle.Render(title) + "\n" + content)
}
