// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package console

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
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

	// Markdown renderer, in glamour's style for the terminal's background
	renderer *glamour.TermRenderer
	style    string
	wrap     int

	// altScreen is whether the session runs on the alternate screen: [Console.Run] sets it, and
	// [Console.RunInline] does not.
	altScreen bool

	// State
	quitting bool
	err      error
}

// darkStyle and lightStyle are glamour's style sheets for a dark and a light background. Dark is glamour's own
// default, and the console's until the terminal answers [tea.RequestBackgroundColor].
const (
	darkStyle  = "dark"
	lightStyle = "light"
)

// newRenderer builds the markdown renderer for a style and a wrap width.
//
// Parameters:
//   - `style`: glamour's style sheet, [darkStyle] or [lightStyle].
//   - `width`: the column text wraps at.
//
// Returns:
//   - `*glamour.TermRenderer`: the renderer, or nil when it cannot be built, in which case steps render as plain
//     text.
func newRenderer(style string, width int) *glamour.TermRenderer {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStylePath(style),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil
	}
	return renderer
}

// NewModel creates a new console model for the given session.
func NewModel(session Session) *Model {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.CharLimit = 256

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	return &Model{
		session:   session,
		styles:    DefaultStyles(),
		textInput: ti,
		spinner:   sp,
		renderer:  newRenderer(darkStyle, 80),
		style:     darkStyle,
		wrap:      80,
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

	m.renderStep()
	return tea.Batch(
		tea.RequestBackgroundColor,
		m.initInputForStep(),
	)
}

// Update handles messages.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case tea.BackgroundColorMsg:
		m.setStyle(msg.IsDark())
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.renderStep()
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	// Update input components. A session that has ended has no step, and messages still arrive after it.
	var cmd tea.Cmd
	if m.step != nil && m.step.Type == StepInput {
		m.textInput, cmd = m.textInput.Update(msg)
	}
	return m, cmd
}

// setStyle renders in glamour's dark or light style, by the terminal's background, and renders the current step
// again when the style changes.
//
// Parameters:
//   - `dark`: whether the terminal's background is dark.
func (m *Model) setStyle(dark bool) {
	style := lightStyle
	if dark {
		style = darkStyle
	}
	if style == m.style {
		return
	}
	m.style = style
	m.renderer = newRenderer(style, m.wrap)
	m.renderStep()
}

// handleKey processes keyboard input.
func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
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
	case StepProgress:
		return m, nil
	case StepComplete, StepError:
		if msg.Code == tea.KeyEnter {
			m.quitting = true
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m *Model) handleInfoKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.Code == tea.KeyEnter || msg.Code == tea.KeySpace {
		return m.advance("")
	}
	return m, nil
}

func (m *Model) handleConfirmKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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

func (m *Model) handleSelectKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.Code {
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
	default:
		// Ignore unhandled key types
	}
	return m, nil
}

func (m *Model) handleInputKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.Code == tea.KeyEnter {
		value := m.textInput.Value()
		if value == "" && m.step.Default != "" {
			value = m.step.Default
		}
		return m.advance(value)
	}

	// Every other key is typing, and the text input takes it.
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
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
	m.renderStep()
	return m, m.initInputForStep()
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
	case StepInfo, StepConfirm, StepSelect, StepComplete, StepError:
		// No initialization needed.
	}
	return nil
}

// renderStep renders the current step's markdown content into [Model.rendered].
//
// It renders in the update that calls it, not in a command: Bubble Tea runs commands on goroutines of their own,
// and a command reading the model races the next update writing it.
func (m *Model) renderStep() {
	switch {
	case m.step == nil || m.step.Content == "":
		m.rendered = ""
	case m.renderer == nil:
		m.rendered = m.step.Content
	default:
		rendered, err := m.renderer.Render(m.step.Content)
		if err != nil {
			rendered = m.step.Content
		}
		m.rendered = rendered
	}
}

// View renders the model, on the alternate screen when [Model.altScreen] says so.
func (m *Model) View() tea.View {
	view := tea.NewView(m.content())
	view.AltScreen = m.altScreen
	return view
}

// content renders the model's text.
func (m *Model) content() string {
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

	case StepInfo:
		// Info steps render only the title/description — no interaction area.
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
		help = "ProviderType your response, Enter to submit"
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
		if r := newRenderer(m.style, width-4); r != nil {
			m.renderer = r
			m.wrap = width - 4
		}
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
