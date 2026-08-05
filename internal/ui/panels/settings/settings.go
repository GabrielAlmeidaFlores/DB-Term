// Package settings implements the full-screen settings overlay panel.
package settings

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gabrielfloresousion/db-term/internal/config"
	"github.com/gabrielfloresousion/db-term/internal/ui/styles"
)

// Tab identifies which settings tab is active.
type Tab int

const (
	TabKeybinds    Tab = iota
	TabConnections Tab = iota
	TabTheme       Tab = iota
)

// Action signals what the settings panel wants the parent to do.
type ActionKind int

const (
	ActionNone        ActionKind = iota
	ActionThemeChange            // user confirmed a new theme
	ActionClose                  // user pressed Esc to close settings
)

// Action carries intent from the settings panel to the root model.
type Action struct {
	Kind    ActionKind
	ThemeID string // set when Kind == ActionThemeChange
}

// Model is the root settings overlay model.
type Model struct {
	activeTab   Tab
	keybinds    tabKeybinds
	connections tabConnections
	theme       tabTheme

	cfg    *config.Config
	width  int
	height int
	styles styles.Styles

	lastAction Action
}

// New returns an initialised settings Model.
func New(cfg *config.Config, s styles.Styles) Model {
	return Model{
		cfg:         cfg,
		styles:      s,
		keybinds:    newTabKeybinds(cfg.Keybinds, s),
		connections: newTabConnections(cfg.Connections, s),
		theme:       newTabTheme(cfg.Settings.Theme, s),
	}
}

// Init satisfies tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// SetStyles updates styles and propagates to all tabs.
func (m *Model) SetStyles(s styles.Styles) {
	m.styles = s
	m.keybinds.styles = s
	m.connections.styles = s
	m.theme.styles = s
}

// SetSize updates the panel dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// SetConfig refreshes the config reference (called after a connection is added/removed).
func (m *Model) SetConfig(cfg *config.Config) {
	m.cfg = cfg
	m.connections.conns = cfg.Connections
}

// TakeAction returns and clears the last pending action.
func (m *Model) TakeAction() Action {
	a := m.lastAction
	m.lastAction = Action{}
	return a
}

// Update handles keyboard events.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.lastAction = Action{Kind: ActionClose}
			return m, nil
		case "tab":
			m.activeTab = (m.activeTab + 1) % 3
			return m, nil
		case "shift+tab":
			m.activeTab = (m.activeTab + 2) % 3 // go backwards
			return m, nil
		}

		// Delegate to active tab.
		switch m.activeTab {
		case TabKeybinds:
			m.keybinds = m.keybinds.update(msg)
		case TabConnections:
			m.connections = m.connections.update(msg)
		case TabTheme:
			prev := m.theme.confirmed
			m.theme = m.theme.update(msg)
			if m.theme.confirmed != prev {
				m.lastAction = Action{
					Kind:    ActionThemeChange,
					ThemeID: m.theme.activeID,
				}
			}
		}
	}
	return m, nil
}

// View renders the full-screen settings overlay.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	tabBar := m.renderTabBar()

	var content string
	innerH := m.height - 6
	if innerH < 1 {
		innerH = 1
	}
	switch m.activeTab {
	case TabKeybinds:
		content = m.keybinds.view(m.width-4, innerH)
	case TabConnections:
		content = m.connections.view(m.width-4, innerH)
	case TabTheme:
		content = m.theme.view(m.width-4, innerH)
	}

	hint := m.styles.Muted.Render("  [Tab] Next tab  [Shift+Tab] Prev tab  [Esc] Close")

	inner := lipgloss.JoinVertical(lipgloss.Left, tabBar, content, hint)

	return m.styles.PanelFocused.
		Width(m.width - 2).
		Height(m.height - 2).
		Render(inner)
}

func (m Model) renderTabBar() string {
	tabs := []struct {
		label string
		id    Tab
	}{
		{styles.IconSettings + "  Keybinds", TabKeybinds},
		{"  Connections", TabConnections},
		{styles.IconTheme + "  Theme", TabTheme},
	}

	var parts []string
	for _, t := range tabs {
		label := "[ " + t.label + " ]"
		if t.id == m.activeTab {
			parts = append(parts, m.styles.Title.Render(label))
		} else {
			parts = append(parts, m.styles.Muted.Render(label))
		}
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, parts...) + "\n"
}
