// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package console

import (
	"image/color"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// mockSession implements Session for testing.
type mockSession struct {
	steps    []*Step
	index    int
	response string
	result   any
	err      error
}

func (m *mockSession) Next() *Step {
	if m.index >= len(m.steps) {
		return nil
	}
	step := m.steps[m.index]
	m.index++
	return step
}

func (m *mockSession) Respond(response string) error {
	m.response = response
	return nil
}

func (m *mockSession) Current() *Step {
	if m.index == 0 || m.index > len(m.steps) {
		return nil
	}
	return m.steps[m.index-1]
}

func (m *mockSession) Complete() bool {
	return m.index >= len(m.steps)
}

func (m *mockSession) Result() any {
	return m.result
}

func (m *mockSession) Error() error {
	return m.err
}

func TestStepTypes(t *testing.T) {
	tests := []struct {
		stepType StepType
		name     string
	}{
		{StepInfo, "info"},
		{StepConfirm, "confirm"},
		{StepSelect, "select"},
		{StepInput, "input"},
		{StepProgress, "progress"},
		{StepComplete, "complete"},
		{StepError, "error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := &Step{Type: tt.stepType, Title: tt.name}
			if step.Type != tt.stepType {
				t.Errorf("expected step type %v, got %v", tt.stepType, step.Type)
			}
		})
	}
}

func TestOption(t *testing.T) {
	opt := Option{
		Label:       "Test Option",
		Description: "A test option",
		Value:       "test",
	}

	if opt.Label != "Test Option" {
		t.Errorf("expected label 'Test Option', got %q", opt.Label)
	}
	if opt.Value != "test" {
		t.Errorf("expected value 'test', got %q", opt.Value)
	}
}

func TestDefaultTheme(t *testing.T) {
	theme := DefaultTheme()

	if theme.Primary == nil {
		t.Error("expected primary color to be set")
	}
	if theme.Error == nil {
		t.Error("expected error color to be set")
	}
	if theme.Success == nil {
		t.Error("expected success color to be set")
	}
}

func TestDefaultStyles(t *testing.T) {
	styles := DefaultStyles()

	if styles == nil {
		t.Fatal("expected styles to be non-nil")
	}

	// Verify key styles are initialized
	_ = styles.Title.Render("test")
	_ = styles.Error.Render("test")
	_ = styles.Success.Render("test")
}

func TestNewConsole(t *testing.T) {
	con := New()
	if con == nil {
		t.Fatal("expected console to be non-nil")
	}
	if con.styles == nil {
		t.Error("expected styles to be initialized")
	}
}

func TestConsoleHelpers(t *testing.T) {
	con := New()

	// These should not panic
	con.Success("test success")
	con.Warning("test warning")
	con.Error("test error")
	con.Info("test info")
}

func TestMockSession(t *testing.T) {
	session := &mockSession{
		steps: []*Step{
			{Type: StepInfo, Title: "Step 1"},
			{Type: StepConfirm, Title: "Step 2"},
			{Type: StepComplete, Title: "Done"},
		},
		result: "test result",
	}

	// Initially not complete
	if session.Complete() {
		t.Error("session should not be complete initially")
	}

	// Advance through steps
	step1 := session.Next()
	if step1 == nil {
		t.Fatal("expected non-nil step1")
	}
	if step1.Title != "Step 1" {
		t.Errorf("expected 'Step 1', got %q", step1.Title)
	}

	step2 := session.Next()
	if step2 == nil {
		t.Fatal("expected non-nil step2")
	}
	if step2.Title != "Step 2" {
		t.Errorf("expected 'Step 2', got %q", step2.Title)
	}

	// Respond to step
	if err := session.Respond("yes"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if session.response != "yes" {
		t.Errorf("expected response 'yes', got %q", session.response)
	}

	step3 := session.Next()
	if step3 == nil {
		t.Fatal("expected non-nil step3")
	}
	if step3.Title != "Done" {
		t.Errorf("expected 'Done', got %q", step3.Title)
	}

	// Now complete
	if !session.Complete() {
		t.Error("session should be complete")
	}

	// Result available
	if session.Result() != "test result" {
		t.Errorf("expected 'test result', got %v", session.Result())
	}
}

func TestNewModel(t *testing.T) {
	session := &mockSession{
		steps: []*Step{
			{Type: StepInfo, Title: "Test", Content: "Hello"},
		},
	}

	model := NewModel(session)
	if model == nil {
		t.Fatal("expected model to be non-nil")
	}
	if model.session != session {
		t.Error("expected session to be set")
	}
	if model.styles == nil {
		t.Error("expected styles to be initialized")
	}
}

func TestModelInit(t *testing.T) {
	session := &mockSession{
		steps: []*Step{
			{Type: StepInfo, Title: "Test", Content: "Hello"},
		},
	}

	model := NewModel(session)
	cmd := model.Init()

	// Should have a command (batch of render + input init)
	if cmd == nil {
		t.Error("expected init to return a command")
	}

	// Step should be set
	if model.step == nil {
		t.Error("expected step to be set after Init")
	}
}

func TestModelView(t *testing.T) {
	session := &mockSession{
		steps: []*Step{
			{Type: StepInfo, Title: "Test Title", Content: "Test content"},
		},
	}

	model := NewModel(session)
	_ = model.Init()

	view := model.View()
	if view.Content == "" {
		t.Error("expected view to have content")
	}
}

// TestConsole_AltScreen pins the alternate screen to Run alone: Bubble Tea v2 takes it from the view, not from a
// program option.
func TestConsole_AltScreen(t *testing.T) {
	c := New()
	for _, testCase := range []struct {
		name      string
		altScreen bool
	}{
		{"Run", true},
		{"RunInline", false},
	} {
		session := &mockSession{steps: []*Step{{Type: StepInfo, Title: "T", Content: "C"}}}
		model := c.newModel(session, testCase.altScreen)
		_ = model.Init()
		if got := model.View().AltScreen; got != testCase.altScreen {
			t.Errorf("%s: View().AltScreen = %v, want %v", testCase.name, got, testCase.altScreen)
		}
	}
}

// TestModel_Keys pins each key a step handles to what it does, as Bubble Tea v2 reports it: a KeyPressMsg whose
// Code is the key and whose Text is what it types.
func TestModel_Keys(t *testing.T) {
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	options := []Option{{Label: "A", Value: "a"}, {Label: "B", Value: "b"}}

	for _, testCase := range []struct {
		name  string
		step  *Step
		keys  []tea.KeyPressMsg
		wants string
	}{
		{"info advances on enter", &Step{Type: StepInfo}, []tea.KeyPressMsg{enter}, ""},
		{"info advances on space", &Step{Type: StepInfo}, []tea.KeyPressMsg{{Code: tea.KeySpace, Text: " "}}, ""},
		{"confirm y", &Step{Type: StepConfirm}, []tea.KeyPressMsg{{Code: 'y', Text: "y"}}, "yes"},
		{"confirm N", &Step{Type: StepConfirm}, []tea.KeyPressMsg{{Code: 'n', ShiftedCode: 'N', Text: "N", Mod: tea.ModShift}}, "no"},
		{"confirm enter takes the default", &Step{Type: StepConfirm, Default: "yes"}, []tea.KeyPressMsg{enter}, "yes"},
		{"select down then enter", &Step{Type: StepSelect, Options: options}, []tea.KeyPressMsg{{Code: tea.KeyDown}, enter}, "b"},
		{"select up stops at the first", &Step{Type: StepSelect, Options: options}, []tea.KeyPressMsg{{Code: tea.KeyUp}, enter}, "a"},
		{"input enter takes the default", &Step{Type: StepInput, Default: "d"}, []tea.KeyPressMsg{enter}, "d"},
	} {
		session := &mockSession{response: "unset", steps: []*Step{testCase.step, {Type: StepInfo}}}
		model := NewModel(session)
		_ = model.Init()
		for _, key := range testCase.keys {
			_, _ = model.Update(key)
		}
		if session.response != testCase.wants {
			t.Errorf("%s: responded %q, want %q", testCase.name, session.response, testCase.wants)
		}
	}
}

// TestModel_Cancel pins ctrl+c and esc to quitting, whatever the step.
func TestModel_Cancel(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{
		{Code: 'c', Mod: tea.ModCtrl},
		{Code: tea.KeyEscape},
	} {
		session := &mockSession{steps: []*Step{{Type: StepInput}}}
		model := NewModel(session)
		_ = model.Init()
		if _, cmd := model.Update(key); cmd == nil || !model.quitting {
			t.Errorf("%q: quitting = %v, want true with a command", key.String(), model.quitting)
		}
	}
}

// TestModel_StyleFollowsBackground pins the markdown style to the terminal's answer to RequestBackgroundColor:
// glamour v2 removed the auto style that read it.
func TestModel_StyleFollowsBackground(t *testing.T) {
	session := &mockSession{steps: []*Step{{Type: StepInfo, Content: "# Hello"}}}
	model := NewModel(session)
	_ = model.Init()

	if model.style != darkStyle {
		t.Fatalf("style before the answer = %q, want %q", model.style, darkStyle)
	}
	dark := model.rendered

	_, _ = model.Update(tea.BackgroundColorMsg{Color: color.White})
	if model.style != lightStyle || model.rendered == dark {
		t.Errorf("on a white background: style = %q, step rendered again = %v; want %q, rendered again",
			model.style, model.rendered != dark, lightStyle)
	}

	_, _ = model.Update(tea.BackgroundColorMsg{Color: color.Black})
	if model.style != darkStyle || model.rendered != dark {
		t.Errorf("on a black background: style = %q, rendered as at first = %v; want %q, as at first",
			model.style, model.rendered == dark, darkStyle)
	}

	renderer := model.renderer
	_, _ = model.Update(tea.BackgroundColorMsg{Color: color.Black})
	if model.renderer != renderer {
		t.Error("the same background again built a new renderer")
	}
}

// recordingSession is a session that keeps every response, in order.
type recordingSession struct {
	mockSession
	responses []string
}

func (r *recordingSession) Respond(response string) error {
	r.responses = append(r.responses, response)
	return r.mockSession.Respond(response)
}

// TestConsole_RunsAProgram drives a real Bubble Tea v2 program through [Console.RunInline], from keystrokes as a
// terminal sends them: a confirm answered with y, an input typed and entered, and the session complete. What a step
// renders is [TestModelView]'s: v2 renders by frame, and these keys are read before the first.
func TestConsole_RunsAProgram(t *testing.T) {
	session := &recordingSession{mockSession: mockSession{steps: []*Step{
		{Type: StepConfirm, Title: "Proceed?"},
		{Type: StepInput, Title: "Name"},
		{Type: StepInfo, Title: "Done"},
	}}}
	c := New().WithInput(strings.NewReader("yabc\r")).WithOutput(io.Discard)
	if _, err := c.RunInline(session); err != nil {
		t.Fatalf("RunInline: %v", err)
	}

	if got := strings.Join(session.responses, ","); got != "yes,abc" {
		t.Errorf("responses = %q, want %q", got, "yes,abc")
	}
}

// TestConsole_CtrlCCancels pins ctrl+c, as the byte a terminal sends, to ending the program with no response.
func TestConsole_CtrlCCancels(t *testing.T) {
	session := &recordingSession{mockSession: mockSession{steps: []*Step{{Type: StepInput, Title: "Name"}}}}

	c := New().WithInput(strings.NewReader("\x03")).WithOutput(io.Discard)
	if _, err := c.RunInline(session); err != nil {
		t.Fatalf("RunInline: %v", err)
	}
	if len(session.responses) != 0 {
		t.Errorf("responses = %q, want none", session.responses)
	}
}

// TestModel_Resizes pins a window resize to the model's width and height. It is pinned at Update, not through a
// program: a Bubble Tea v2 program sends its own first size, 0x0 without a terminal, at a moment a test cannot
// order against.
func TestModel_Resizes(t *testing.T) {
	session := &mockSession{steps: []*Step{{Type: StepInfo, Title: "Wait", Content: "Hello"}}}
	model := NewModel(session)
	_ = model.Init()

	_, _ = model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if model.width != 120 || model.height != 40 {
		t.Errorf("after the resize: %dx%d, want 120x40", model.width, model.height)
	}
	if model.rendered == "" {
		t.Error("the step was not rendered after the resize")
	}
}

// TestConsole_NoColorAnyValue pins NO_COLOR to no-color.org's rule: any value that is not empty turns color off
// and keeps the text. colorprofile alone would honor only a value strconv.ParseBool reads as true.
//
// CLICOLOR_FORCE keeps color on a writer that is not a terminal, so the color has something to lose; the first case
// shows it is there without NO_COLOR. Which color codes appear is the terminal's: 256-color where TERM says so, the
// 16 colors where TERM is unset, as on CI's runners.
func TestConsole_NoColorAnyValue(t *testing.T) {
	t.Setenv("CLICOLOR_FORCE", "1")
	for _, testCase := range []struct {
		value string
		color bool
	}{
		{"", true},
		{"1", false},
		{"true", false},
		{"yes", false},
		{"anything", false},
	} {
		t.Setenv("NO_COLOR", testCase.value)
		var output strings.Builder
		New().WithOutput(&output).Success("done")
		if got := hasColor(output.String()); got != testCase.color {
			t.Errorf("NO_COLOR=%q: colored = %v, want %v: %q", testCase.value, got, testCase.color, output.String())
		}
		if !strings.Contains(output.String(), "done") {
			t.Errorf("NO_COLOR=%q: the message lost its text: %q", testCase.value, output.String())
		}
	}
}

// sgrSequence matches one SGR escape sequence and captures its parameters.
var sgrSequence = regexp.MustCompile(`\x1b\[([0-9;]*)m`)

// hasColor reports whether any SGR sequence in s sets a color: a foreground or background in the 8, bright 8, 256 or
// true colors (30-38, 40-48, 90-97, 100-107). Bold, italic and the other attributes are not color.
//
// Parameters:
//   - `s`: rendered output.
//
// Returns:
//   - `bool`: whether s sets any color.
func hasColor(s string) bool {
	for _, match := range sgrSequence.FindAllStringSubmatch(s, -1) {
		for _, parameter := range strings.Split(match[1], ";") {
			code, err := strconv.Atoi(parameter)
			if err != nil {
				continue
			}
			if code >= 30 && code <= 38 || code >= 40 && code <= 48 || code >= 90 && code <= 97 || code >= 100 && code <= 107 {
				return true
			}
		}
	}
	return false
}
