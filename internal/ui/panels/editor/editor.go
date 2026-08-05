// Package editor implements the SQL editor panel.
package editor

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gabrielfloresousion/db-term/internal/config"
	"github.com/gabrielfloresousion/db-term/internal/ui/styles"
)

// ExecAction signals that the user wants to run the current SQL.
type ExecAction struct {
	ConnName string
	SQL      string
}

// Model is the Bubble Tea model for the SQL editor panel.
type Model struct {
	textarea     textarea.Model
	activeConn   string
	activeSchema string
	width        int
	height       int
	focused      bool
	styles       styles.Styles
	keybinds     config.Keybinds

	viEnabled    bool
	mode         editorMode
	pendingKey   string
	yankBuffer   string
	scrollOffset int

	lastExec *ExecAction
}

// New returns an initialised editor Model.
func New(s styles.Styles, kb config.Keybinds, settings config.Settings) Model {
	ta := textarea.New()
	ta.Placeholder = "-- Write your SQL here\n-- " + kb.RunQuery + " to run"
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.SetValue("")

	return Model{
		textarea:  ta,
		styles:    s,
		keybinds:  kb,
		viEnabled: settings.ViMode,
		mode:      modeInsert,
	}
}

// Init satisfies tea.Model.
func (m Model) Init() tea.Cmd {
	return textarea.Blink
}

// SetStyles updates the lipgloss styles (called on theme change).
func (m *Model) SetStyles(s styles.Styles) { m.styles = s }

// SetSettings applies runtime settings changes (called when the user saves config).
func (m *Model) SetSettings(s config.Settings) {
	m.viEnabled = s.ViMode
	if !m.viEnabled {
		m.mode = modeInsert
		m.pendingKey = ""
	}
}

// SetSize sets the panel dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.textarea.SetWidth(w - 4)
	m.textarea.SetHeight(h - 4)
	if m.textarea.Width() < 1 {
		m.textarea.SetWidth(1)
	}
	if m.textarea.Height() < 1 {
		m.textarea.SetHeight(1)
	}
}

// Focus gives keyboard focus to the editor.
func (m *Model) Focus() {
	m.focused = true
	m.textarea.Focus()
}

// Blur removes keyboard focus.
func (m *Model) Blur() {
	m.focused = false
	m.textarea.Blur()
}

// SetActiveConn updates the active connection shown in the panel header.
func (m *Model) SetActiveConn(connName, schemaName string) {
	m.activeConn = connName
	m.activeSchema = schemaName
}

// TakeExec returns and clears the pending execution action.
func (m *Model) TakeExec() *ExecAction {
	a := m.lastExec
	m.lastExec = nil
	return a
}

// Value returns the current SQL text.
func (m Model) Value() string { return m.textarea.Value() }

// SetSQL replaces the editor content with the given SQL string.
// It is used by external callers (e.g. app.go) to inject generated SQL
// such as UPDATE statements from the edit-cell feature.
func (m *Model) SetSQL(sql string) { m.textarea.SetValue(sql) }

// AppendSQL appends sql to the existing editor content separated by a blank
// line. If the editor is empty, it behaves like SetSQL. This preserves the
// query history so the user can review every statement that was auto-generated.
func (m *Model) AppendSQL(sql string) {
	current := strings.TrimSpace(m.textarea.Value())
	if current == "" {
		m.textarea.SetValue(sql)
	} else {
		m.textarea.SetValue(current + "\n\n" + sql)
	}
}

// ActiveConn returns the name of the currently active connection.
func (m Model) ActiveConn() string { return m.activeConn }

func (m Model) inInsertMode() bool {
	return !m.viEnabled || m.mode == modeInsert
}

func (m Model) syncScroll() Model {
	vpH := m.textarea.Height()
	if vpH < 1 {
		vpH = 1
	}
	cur := m.textarea.Line()
	if cur < m.scrollOffset {
		m.scrollOffset = cur
	}
	if cur >= m.scrollOffset+vpH {
		m.scrollOffset = cur - vpH + 1
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
	return m
}

// Update handles keyboard and window events.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.focused {
		return m, nil
	}

	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.viEnabled && msg.Type == tea.KeyEsc {
			m.mode = modeNormal
			m.pendingKey = ""
		} else if m.inInsertMode() {
			if msg.String() == m.keybinds.RunQuery {
				sql := strings.TrimSpace(m.textarea.Value())
				if sql != "" {
					m.lastExec = &ExecAction{
						ConnName: m.activeConn,
						SQL:      sql,
					}
				}
			} else {
				m.textarea, cmd = m.textarea.Update(msg)
			}
		} else {
			m, cmd = handleNormalMode(m, msg)
		}
	default:
		m.textarea, cmd = m.textarea.Update(msg)
	}

	m = m.syncScroll()
	return m, cmd
}

func (m Model) renderHighlighted() string {
	sql := m.textarea.Value()
	if sql == "" {
		return m.styles.Muted.Render("-- Write your SQL here")
	}

	lines := strings.Split(sql, "\n")

	vpH := m.textarea.Height()
	if vpH < 1 {
		vpH = 1
	}

	startLine := m.scrollOffset
	if startLine < 0 {
		startLine = 0
	}
	if startLine >= len(lines) {
		startLine = len(lines) - 1
	}
	endLine := startLine + vpH
	if endLine > len(lines) {
		endLine = len(lines)
	}

	curLine := -1
	curCol := 0
	if m.focused {
		curLine = m.textarea.Line()
		curCol = m.textarea.LineInfo().CharOffset
	}

	var result []string
	for i := startLine; i < endLine; i++ {
		plain := lines[i]
		if i == curLine {
			runes := []rune(plain)
			if curCol >= len(runes) {
				result = append(result, HighlightSQL(plain, m.styles)+m.styles.Cursor.Render(" "))
			} else {
				before := string(runes[:curCol])
				at := string(runes[curCol : curCol+1])
				after := string(runes[curCol+1:])
				result = append(result, HighlightSQL(before, m.styles)+m.styles.Cursor.Render(at)+HighlightSQL(after, m.styles))
			}
		} else {
			result = append(result, HighlightSQL(plain, m.styles))
		}
	}

	for len(result) < vpH {
		result = append(result, "")
	}

	return strings.Join(result, "\n")
}

// View renders the editor panel with a rounded border.
func (m Model) View() string {
	if m.width < 4 || m.height < 3 {
		return ""
	}

	connLabel := "no connection selected"
	if m.activeConn != "" {
		connLabel = m.activeConn
		if m.activeSchema != "" {
			connLabel += " " + styles.IconSeparator + " " + m.activeSchema
		}
	}

	headerLeft := m.styles.Title.Render(styles.IconEditor + "  Editor")
	headerMid := m.styles.Muted.Render(" — " + connLabel)
	runHint := m.styles.Muted.Render("[" + m.keybinds.RunQuery + "] Run")

	contentW := m.width - 4
	if contentW < 1 {
		contentW = 1
	}

	var modeIndicator string
	if m.viEnabled {
		if m.mode == modeNormal {
			modeIndicator = "  " + m.styles.Title.Render("[N]")
		} else {
			modeIndicator = "  " + m.styles.Muted.Render("[I]")
		}
	}

	headerLine := lipgloss.NewStyle().Width(contentW).Render(
		headerLeft + headerMid + modeIndicator + "  " + runHint,
	)

	var body string
	if m.activeConn == "" {
		body = m.styles.Muted.Render(
			"\n  Select a table in the sidebar to set the active connection.",
		)
	} else {
		body = m.renderHighlighted()
	}

	inner := lipgloss.JoinVertical(lipgloss.Left, headerLine, body)

	borderStyle := m.styles.PanelUnfocused
	if m.focused {
		borderStyle = m.styles.PanelFocused
	}

	return borderStyle.
		Width(m.width - 2).
		Height(m.height - 2).
		Render(inner)
}
