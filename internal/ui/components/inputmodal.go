package components

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gabrielfloresousion/db-term/internal/ui/styles"
)

// InputModalAction is produced when the user confirms or cancels the modal.
type InputModalAction struct {
	Cancelled bool
	Value     string
}

// InputModal is a reusable single-line text-input overlay.
// It is used for simple prompts such as "enter database name" or
// "rename connection". More complex forms use ConnForm.
type InputModal struct {
	title       string
	placeholder string
	input       textinput.Model
	width       int
	height      int
	styles      styles.Styles
	statusMsg   string // transient line rendered above the hint bar

	lastAction *InputModalAction
}

// NewInputModal returns an initialised InputModal.
func NewInputModal(title, placeholder string, s styles.Styles) InputModal {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = 256
	ti.Focus()

	return InputModal{
		title:       title,
		placeholder: placeholder,
		input:       ti,
		styles:      s,
	}
}

// SetStyles updates lipgloss styles (called on theme change).
func (m *InputModal) SetStyles(s styles.Styles) { m.styles = s }

// SetStatus sets a transient status message rendered above the hint bar.
func (m *InputModal) SetStatus(msg string) { m.statusMsg = msg }

// SetSize informs the modal of the terminal dimensions and resizes the
// internal text input to fill the box width.
func (m *InputModal) SetSize(w, h int) {
	m.width = w
	m.height = h
	boxW := w * 4 / 5
	if boxW < 40 {
		boxW = 40
	}
	if w > 0 && boxW > w-8 {
		boxW = w - 8
	}
	// Subtract box padding (2×1) and border (2×1) from the usable input width.
	m.input.Width = boxW - 6
}

// SetValue pre-fills the input with an initial value.
func (m *InputModal) SetValue(v string) { m.input.SetValue(v) }

// TakeAction returns and clears the last pending action.
func (m *InputModal) TakeAction() *InputModalAction {
	a := m.lastAction
	m.lastAction = nil
	return a
}

// Update handles keyboard events.
func (m InputModal) Update(msg tea.Msg) (InputModal, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			v := strings.TrimSpace(m.input.Value())
			m.lastAction = &InputModalAction{Value: v}
			return m, nil
		case "esc":
			m.lastAction = &InputModalAction{Cancelled: true}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// View renders the input modal centred on the terminal.
func (m InputModal) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	boxW := m.width * 4 / 5
	if boxW < 40 {
		boxW = 40
	}
	if m.width > 0 && boxW > m.width-8 {
		boxW = m.width - 8
	}

	title := m.styles.Title.Render(m.title)
	sep := m.styles.Muted.Render(strings.Repeat("─", boxW-4))
	inputLine := m.input.View()
	hint := m.styles.Muted.Render("  [Enter] Confirm   [Esc] Cancel")

	rows := []string{title, sep, "", inputLine, ""}
	if m.statusMsg != "" {
		rows = append(rows, m.styles.Success.Render("  "+m.statusMsg))
	}
	rows = append(rows, hint)

	inner := lipgloss.JoinVertical(lipgloss.Left, rows...)

	box := m.styles.PanelFocused.
		Width(boxW-2).
		Padding(0, 1).
		Render(inner)

	boxH := lipgloss.Height(box)
	topPad := (m.height - boxH) / 2
	if topPad < 0 {
		topPad = 0
	}
	leftPad := (m.width - lipgloss.Width(box)) / 2
	if leftPad < 0 {
		leftPad = 0
	}

	pad := strings.Repeat(" ", leftPad)
	return strings.Repeat("\n", topPad) +
		pad + strings.ReplaceAll(box, "\n", "\n"+pad)
}
