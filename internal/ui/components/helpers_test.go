package components

import tea "github.com/charmbracelet/bubbletea"

// pressKey is a test helper that constructs a tea.KeyMsg for a given key string.
func pressKey(key string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key), Alt: false}
}
