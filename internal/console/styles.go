// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package console

import "github.com/charmbracelet/lipgloss"

// Theme holds the color scheme for the console UI.
type Theme struct {
	Primary   lipgloss.Color
	Secondary lipgloss.Color
	Success   lipgloss.Color
	Warning   lipgloss.Color
	Error     lipgloss.Color
	Muted     lipgloss.Color
	Text      lipgloss.Color
}

// DefaultTheme returns the default color scheme.
func DefaultTheme() Theme {
	return Theme{
		Primary:   "39",  // Blue
		Secondary: "99",  // Purple
		Success:   "42",  // Green
		Warning:   "214", // Orange
		Error:     "196", // Red
		Muted:     "245", // Gray
		Text:      "252", // Light gray
	}
}

// Styles holds the lipgloss styles for console rendering.
type Styles struct {
	theme Theme

	// Layout styles
	Container lipgloss.Style
	Header    lipgloss.Style
	Content   lipgloss.Style
	Footer    lipgloss.Style

	// Text styles
	Title       lipgloss.Style
	Subtitle    lipgloss.Style
	Body        lipgloss.Style
	Muted       lipgloss.Style
	Highlighted lipgloss.Style

	// Status styles
	Success lipgloss.Style
	Warning lipgloss.Style
	Error   lipgloss.Style

	// Interactive styles
	Prompt         lipgloss.Style
	SelectedOption lipgloss.Style
	Option         lipgloss.Style
	Input          lipgloss.Style

	// Progress styles
	ProgressBar     lipgloss.Style
	ProgressFilled  lipgloss.Style
	ProgressEmpty   lipgloss.Style
	ProgressPercent lipgloss.Style
}

// NewStyles creates styles with the given theme.
func NewStyles(theme Theme) *Styles {
	s := &Styles{theme: theme}

	// Layout
	s.Container = lipgloss.NewStyle().
		Padding(1, 2)

	s.Header = lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		BorderForeground(theme.Muted).
		MarginBottom(1).
		Padding(0, 1)

	s.Content = lipgloss.NewStyle().
		Padding(1, 0)

	s.Footer = lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderTop(true).
		BorderForeground(theme.Muted).
		MarginTop(1).
		Padding(0, 1).
		Foreground(theme.Muted)

	// Text
	s.Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(theme.Primary)

	s.Subtitle = lipgloss.NewStyle().
		Foreground(theme.Secondary)

	s.Body = lipgloss.NewStyle().
		Foreground(theme.Text)

	s.Muted = lipgloss.NewStyle().
		Foreground(theme.Muted)

	s.Highlighted = lipgloss.NewStyle().
		Bold(true).
		Foreground(theme.Primary)

	// Status
	s.Success = lipgloss.NewStyle().
		Foreground(theme.Success)

	s.Warning = lipgloss.NewStyle().
		Foreground(theme.Warning)

	s.Error = lipgloss.NewStyle().
		Foreground(theme.Error)

	// Interactive
	s.Prompt = lipgloss.NewStyle().
		Foreground(theme.Primary).
		Bold(true)

	s.SelectedOption = lipgloss.NewStyle().
		Foreground(theme.Primary).
		Bold(true).
		PaddingLeft(2)

	s.Option = lipgloss.NewStyle().
		Foreground(theme.Text).
		PaddingLeft(2)

	s.Input = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(theme.Primary).
		Padding(0, 1)

	// Progress
	s.ProgressBar = lipgloss.NewStyle().
		Width(40)

	s.ProgressFilled = lipgloss.NewStyle().
		Foreground(theme.Success)

	s.ProgressEmpty = lipgloss.NewStyle().
		Foreground(theme.Muted)

	s.ProgressPercent = lipgloss.NewStyle().
		Foreground(theme.Text).
		MarginLeft(1)

	return s
}

// DefaultStyles returns styles with the default theme.
func DefaultStyles() *Styles {
	return NewStyles(DefaultTheme())
}
