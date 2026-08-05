package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gabrielfloresousion/db-term/internal/config"
	"github.com/gabrielfloresousion/db-term/internal/db"
)

func TestHandleKey_FilterConsumesNewConnectionKey(t *testing.T) {
	// Regression: typing "n" in the sidebar filter opened the new-connection form instead of filtering.
	cfg := config.DefaultConfig()
	m := New(cfg, make(config.SecretsStore), db.NewManager())

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(cfg.Keybinds.FilterSidebar)})
	m = model.(Model)
	model, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(cfg.Keybinds.NewConnection)})
	m = model.(Model)

	if m.connform != nil {
		t.Fatal("typing the new-connection key in the sidebar filter opened the connection form")
	}
	if !m.sidebar.IsFiltering() {
		t.Fatal("sidebar left filter mode after receiving a plain character")
	}
}
