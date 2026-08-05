// Package components provides reusable UI components for db-term.
package components

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gabrielfloresousion/db-term/internal/ui/styles"
)

// ModalButton is one action button shown at the bottom of the modal.
type ModalButton struct {
	Label    string
	IsDanger bool // renders in Error color when true
}

// ModalAction is returned when the user selects a button.
type ModalAction struct {
	Label string // which button was pressed
}

// Modal is a centered overlay dialog with a title, body text, and action buttons.
// It has no knowledge of app-level messages — callers check the returned action
// via TakeAction() after each Update().
type Modal struct {
	Title   string
	Body    string
	Buttons []ModalButton
	cursor  int
	width   int
	height  int
	styles  styles.Styles

	lastAction  *ModalAction
	dismissed   bool   // true after Esc — caller should hide the modal
	copyPending bool   // true after 'y' — caller should copy Body to clipboard
	statusMsg   string // transient line rendered above the hint bar
}

// NewModal returns an initialised Modal.
func NewModal(title, body string, buttons []ModalButton, s styles.Styles) Modal {
	return Modal{
		Title:   title,
		Body:    body,
		Buttons: buttons,
		styles:  s,
	}
}

// SetStyles updates lipgloss styles (called on theme change).
func (m *Modal) SetStyles(s styles.Styles) { m.styles = s }

// SetSize informs the modal of the terminal dimensions so it can centre itself.
func (m *Modal) SetSize(w, h int) { m.width = w; m.height = h }

// SetStatus sets a transient status message rendered above the hint bar.
func (m *Modal) SetStatus(msg string) { m.statusMsg = msg }

// TakeAction returns and clears the last pending button action.
func (m *Modal) TakeAction() *ModalAction {
	a := m.lastAction
	m.lastAction = nil
	return a
}

// TakeCopyBody returns true and clears the flag if the user pressed 'y'
// to copy the modal body to the clipboard.
func (m *Modal) TakeCopyBody() bool {
	v := m.copyPending
	m.copyPending = false
	return v
}

// Dismissed reports whether the user pressed Esc to close the modal.
// Callers should check this after Update() and hide the modal if true.
func (m *Modal) Dismissed() bool { return m.dismissed }

// ResetDismissed clears the dismissed flag (called after the parent hides the modal).
func (m *Modal) ResetDismissed() { m.dismissed = false }

// Update handles keyboard events.
func (m Modal) Update(msg tea.Msg) (Modal, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "left", "h":
			if m.cursor > 0 {
				m.cursor--
			}
		case "right", "l":
			if m.cursor < len(m.Buttons)-1 {
				m.cursor++
			}
		case "tab":
			if len(m.Buttons) > 0 {
				m.cursor = (m.cursor + 1) % len(m.Buttons)
			}
		case "enter":
			if len(m.Buttons) > 0 {
				m.lastAction = &ModalAction{Label: m.Buttons[m.cursor].Label}
			}
		case "y":
			m.copyPending = true
		case "esc":
			m.dismissed = true
		}
	}
	return m, nil
}

// View renders the modal centred over the terminal.
func (m Modal) View() string {
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

	title := m.styles.Title.Render(" " + m.Title)
	bodyLines := wrapText(m.Body, boxW-4)
	body := m.styles.Text.Render(strings.Join(bodyLines, "\n"))
	buttons := m.renderButtons(boxW)
	divider := m.styles.Muted.Render(strings.Repeat("─", boxW-2))
	hint := m.styles.Muted.Render("[y] Copy  [←→] Select  [Enter] Confirm  [Esc] Close")

	var statusLine string
	if m.statusMsg != "" {
		statusLine = m.styles.Success.Render("  " + m.statusMsg)
	}

	rows := []string{title, divider, "", body, "", buttons, ""}
	if statusLine != "" {
		rows = append(rows, statusLine)
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

	return strings.Repeat("\n", topPad) +
		strings.Repeat(" ", leftPad) +
		strings.ReplaceAll(box, "\n", "\n"+strings.Repeat(" ", leftPad))
}

func (m Modal) renderButtons(boxW int) string {
	if len(m.Buttons) == 0 {
		return ""
	}

	var parts []string
	for i, btn := range m.Buttons {
		label := "[ " + btn.Label + " ]"
		var rendered string
		if i == m.cursor {
			if btn.IsDanger {
				rendered = m.styles.Error.Bold(true).Render(label)
			} else {
				rendered = m.styles.Title.Render(label)
			}
		} else {
			rendered = m.styles.Muted.Render(label)
		}
		parts = append(parts, rendered)
	}

	row := strings.Join(parts, "  ")
	rowW := lipgloss.Width(row)
	pad := boxW - 4 - rowW
	if pad < 0 {
		pad = 0
	}
	return strings.Repeat(" ", pad) + row
}

// wrapText breaks text into lines of at most maxW runes.
func wrapText(text string, maxW int) []string {
	if maxW <= 0 {
		return []string{text}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	var current strings.Builder
	for _, w := range words {
		if current.Len() == 0 {
			current.WriteString(w)
		} else if current.Len()+1+len(w) <= maxW {
			current.WriteByte(' ')
			current.WriteString(w)
		} else {
			lines = append(lines, current.String())
			current.Reset()
			current.WriteString(w)
		}
	}
	if current.Len() > 0 {
		lines = append(lines, current.String())
	}
	return lines
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
