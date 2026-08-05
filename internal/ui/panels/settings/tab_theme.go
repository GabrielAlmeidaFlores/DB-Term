package settings

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gabrielfloresousion/db-term/internal/config"
	"github.com/gabrielfloresousion/db-term/internal/ui/styles"
)

type themeEntry struct {
	id          string
	displayName string
	description string
}

var themeEntries = []themeEntry{
	{"catppuccin-mocha", "Catppuccin Mocha", "Dark · purple-blue"},
	{"catppuccin-latte", "Catppuccin Latte", "Light · soft tones"},
	{"dracula", "Dracula", "Dark · classic purple"},
	{"tokyo-night", "Tokyo Night", "Dark · midnight blue"},
	{"gruvbox", "Gruvbox Dark", "Dark · warm earthtones"},
	{"custom", "Custom", "Defined in config.toml"},
}

type tabTheme struct {
	cursor    int    // index in themeEntries
	activeID  string // confirmed (saved) theme
	previewID string // currently highlighted (not yet saved)
	confirmed bool   // toggled when user presses Enter
	styles    styles.Styles
}

func newTabTheme(activeID string, s styles.Styles) tabTheme {
	cursor := 0
	for i, e := range themeEntries {
		if e.id == activeID {
			cursor = i
			break
		}
	}
	return tabTheme{
		cursor:    cursor,
		activeID:  activeID,
		previewID: activeID,
		styles:    s,
	}
}

func (t tabTheme) update(msg tea.KeyMsg) tabTheme {
	t.confirmed = false
	switch msg.String() {
	case "up", "k":
		if t.cursor > 0 {
			t.cursor--
			t.previewID = themeEntries[t.cursor].id
		}
	case "down", "j":
		if t.cursor < len(themeEntries)-1 {
			t.cursor++
			t.previewID = themeEntries[t.cursor].id
		}
	case "enter":
		t.activeID = themeEntries[t.cursor].id
		t.previewID = t.activeID
		t.confirmed = true
	case "esc":
		// Revert to saved active theme.
		for i, e := range themeEntries {
			if e.id == t.activeID {
				t.cursor = i
				break
			}
		}
		t.previewID = t.activeID
	}
	return t
}

func (t tabTheme) view(w, h int) string {
	header := t.styles.Subtext.Render(
		fmt.Sprintf("  Active: %s", t.activeID),
	)
	sep := t.styles.Muted.Render("  " + strings.Repeat("─", w-4))

	var rows []string
	rows = append(rows, header, sep)

	for i, entry := range themeEntries {
		swatch := t.buildSwatch(entry.id)
		line := fmt.Sprintf("  %-22s  %s  %s",
			entry.displayName,
			swatch,
			entry.description,
		)
		if i == t.cursor {
			rows = append(rows, t.styles.TreeSelected.Render(line))
		} else {
			rows = append(rows, t.styles.Text.Render(line))
		}
	}

	rows = append(rows, "")
	rows = append(rows, t.styles.Muted.Render(
		"  Navigate to preview  ·  Enter to apply  ·  Esc to revert",
	))

	return strings.Join(rows, "\n")
}

// buildSwatch renders four colored squares using the canonical theme colors
// from styles.Resolve — single source of truth, no duplicate color table.
func (t tabTheme) buildSwatch(themeID string) string {
	theme := styles.Resolve(themeID, config.ThemeConfig{})
	sq := "██"
	return lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Primary)).Render(sq) +
		lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Success)).Render(sq) +
		lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Warning)).Render(sq) +
		lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Error)).Render(sq)
}
