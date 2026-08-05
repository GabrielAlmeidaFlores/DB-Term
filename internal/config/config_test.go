package config

import (
	"os"
	"path/filepath"
	"testing"
)

// usesTempDir redirects ConfigDir to a temp directory for the duration of the test.
func usesTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// Redirect ConfigDir by setting HOME so os.UserHomeDir picks it up.
	t.Setenv("HOME", dir)
	return dir
}

func TestDefaultConfig_NoEmptyKeybinds(t *testing.T) {
	cfg := DefaultConfig()

	checks := map[string]string{
		"RunQuery":         cfg.Keybinds.RunQuery,
		"CancelQuery":      cfg.Keybinds.CancelQuery,
		"NewConnection":    cfg.Keybinds.NewConnection,
		"DeleteConnection": cfg.Keybinds.DeleteConnection,
		"ToggleSidebar":    cfg.Keybinds.ToggleSidebar,
		"FocusEditor":      cfg.Keybinds.FocusEditor,
		"FocusResults":     cfg.Keybinds.FocusResults,
		"OpenSettings":     cfg.Keybinds.OpenSettings,
		"FilterSidebar":    cfg.Keybinds.FilterSidebar,
		"Quit":             cfg.Keybinds.Quit,
		"Help":             cfg.Keybinds.Help,
		"CopyCell":         cfg.Keybinds.CopyCell,
		"FollowFK":         cfg.Keybinds.FollowFK,
		"NextPage":         cfg.Keybinds.NextPage,
		"PrevPage":         cfg.Keybinds.PrevPage,
	}

	for name, value := range checks {
		if value == "" {
			t.Errorf("DefaultConfig: Keybinds.%s is empty", name)
		}
	}
}

func TestDefaultConfig_ThemeIsSet(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Settings.Theme == "" {
		t.Error("DefaultConfig: Settings.Theme is empty")
	}
}

func TestDefaultConfig_NoConflicts(t *testing.T) {
	cfg := DefaultConfig()
	conflicts := ValidateKeybinds(cfg.Keybinds)
	if len(conflicts) > 0 {
		t.Errorf("DefaultConfig has keybind conflicts: %v", conflicts)
	}
}

func TestValidateKeybinds_DetectsConflict(t *testing.T) {
	kb := DefaultConfig().Keybinds
	// Introduce a conflict: PrevPage same as ToggleSidebar.
	kb.PrevPage = kb.ToggleSidebar

	conflicts := ValidateKeybinds(kb)
	if len(conflicts) == 0 {
		t.Error("ValidateKeybinds: expected conflict, got none")
	}
}

func TestValidateKeybinds_NoConflictOnDefaults(t *testing.T) {
	kb := DefaultConfig().Keybinds
	conflicts := ValidateKeybinds(kb)
	if len(conflicts) != 0 {
		t.Errorf("ValidateKeybinds: unexpected conflicts on defaults: %v", conflicts)
	}
}

func TestLoad_CreatesDefaultConfigOnFirstRun(t *testing.T) {
	usesTempDir(t)

	cfg, conflicts, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(conflicts) > 0 {
		t.Errorf("Load() unexpected conflicts on fresh config: %v", conflicts)
	}
	if cfg.Settings.Theme != "catppuccin-mocha" {
		t.Errorf("Load() theme = %q, want %q", cfg.Settings.Theme, "catppuccin-mocha")
	}
}

func TestLoad_CreatesFileOnDisk(t *testing.T) {
	home := usesTempDir(t)

	_, _, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	path := filepath.Join(home, ".config", "dbterm", "config.toml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("Load() did not create config file at %s", path)
	}
}

func TestSave_RoundTrip(t *testing.T) {
	usesTempDir(t)

	// Write a custom config.
	cfg := DefaultConfig()
	cfg.Settings.Theme = "dracula"
	cfg.Connections = []Connection{
		{Name: "test-pg", Driver: "postgres", Host: "localhost", Port: 5432},
	}

	if err := cfg.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Reload and verify.
	loaded, _, err := Load()
	if err != nil {
		t.Fatalf("Load() after Save() error: %v", err)
	}
	if loaded.Settings.Theme != "dracula" {
		t.Errorf("round-trip theme = %q, want %q", loaded.Settings.Theme, "dracula")
	}
	if len(loaded.Connections) != 1 {
		t.Fatalf("round-trip connections count = %d, want 1", len(loaded.Connections))
	}
	if loaded.Connections[0].Name != "test-pg" {
		t.Errorf("round-trip connection name = %q, want %q", loaded.Connections[0].Name, "test-pg")
	}
}

func TestLoad_ResetConflictingKeybindsToDefaults(t *testing.T) {
	usesTempDir(t)

	// Write a config with a keybind conflict.
	cfg := DefaultConfig()
	cfg.Keybinds.PrevPage = cfg.Keybinds.ToggleSidebar // same as ctrl+b → conflict
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, conflicts, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(conflicts) == 0 {
		t.Error("Load() expected conflicts, got none")
	}
	// Conflicting keys should have been reset to defaults.
	defaults := DefaultConfig()
	if loaded.Keybinds.PrevPage != defaults.Keybinds.PrevPage {
		t.Errorf("Load() PrevPage after conflict reset = %q, want %q",
			loaded.Keybinds.PrevPage, defaults.Keybinds.PrevPage)
	}
}

func TestDefaultConfig_ExternalEditorIsVi(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Settings.ExternalEditor != "vi" {
		t.Errorf("DefaultConfig ExternalEditor = %q, want %q", cfg.Settings.ExternalEditor, "vi")
	}
}

func TestDefaultConfig_ViModeOffByDefault(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Settings.ViMode {
		t.Error("DefaultConfig ViMode should be false (opt-in)")
	}
}
