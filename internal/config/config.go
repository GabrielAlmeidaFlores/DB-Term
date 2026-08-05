// Package config handles reading and writing the application configuration.
// It has no internal imports — only stdlib and third-party libraries.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// Connection holds the parameters for a single database connection.
// Passwords are never stored here — they live in the secrets store.
type Connection struct {
	Name        string `toml:"name"`
	Driver      string `toml:"driver"` // "postgres" | "mysql" | "sqlserver"
	Host        string `toml:"host"`
	Port        int    `toml:"port"`
	User        string `toml:"user"`
	Database    string `toml:"database"`
	SSLMode     string `toml:"ssl_mode"`     // postgres: "disable" | "require" | "verify-full"
	SSLCert     string `toml:"ssl_cert"`     // path to client cert (optional)
	SSLKey      string `toml:"ssl_key"`      // path to client key (optional)
	SSLRootCert string `toml:"ssl_rootcert"` // path to CA cert (optional)
}

// Settings holds general application settings.
type Settings struct {
	Theme          string `toml:"theme"`
	ExternalEditor string `toml:"external_editor"` // e.g. "vi", "nano". Empty = use $EDITOR env var.
	ViMode         bool   `toml:"vi_mode"`         // enable vi-style modal editing in the SQL editor
}

// ThemeSection wraps the [theme] TOML table.
// Custom colors live under [theme.custom].
type ThemeSection struct {
	Custom ThemeConfig `toml:"custom"`
}

// ThemeConfig holds all hex color values for a custom theme.
// Only active when Settings.Theme == "custom".
// The config package does NOT import the styles package — no conversion here.
type ThemeConfig struct {
	Bg          string `toml:"bg"`
	Surface     string `toml:"surface"`
	Border      string `toml:"border"`
	Focus       string `toml:"focus"`
	Text        string `toml:"text"`
	Subtext     string `toml:"subtext"`
	Muted       string `toml:"muted"`
	Primary     string `toml:"primary"`
	Success     string `toml:"success"`
	Warning     string `toml:"warning"`
	Error       string `toml:"error"`
	SynKeyword  string `toml:"keyword"`
	SynDML      string `toml:"dml_word"`
	SynFunction string `toml:"function"`
	SynString   string `toml:"str"`
	SynNumber   string `toml:"number"`
	SynComment  string `toml:"comment"`
	SynOperator string `toml:"operator"`
}

// Keybinds holds all configurable key bindings with their defaults.
type Keybinds struct {
	RunQuery         string `toml:"run_query"`
	CancelQuery      string `toml:"cancel_query"`
	NewConnection    string `toml:"new_connection"`
	DeleteConnection string `toml:"delete_connection"`
	ToggleSidebar    string `toml:"toggle_sidebar"`
	FocusEditor      string `toml:"focus_editor"`
	FocusResults     string `toml:"focus_results"`
	OpenSettings     string `toml:"open_settings"`
	FilterSidebar    string `toml:"filter_sidebar"`
	Quit             string `toml:"quit"`
	Help             string `toml:"help"`
	CopyCell         string `toml:"copy_cell"`
	NextPage         string `toml:"next_page"`
	PrevPage         string `toml:"prev_page"`
	// Panel resize — press to grow/shrink each panel dimension (like tmux)
	ResizePanelLeft  string `toml:"resize_panel_left"`
	ResizePanelRight string `toml:"resize_panel_right"`
	ResizePanelUp    string `toml:"resize_panel_up"`
	ResizePanelDown  string `toml:"resize_panel_down"`
	// OpenExternalEditor opens the editor content in the system editor (nano, vi…).
	OpenExternalEditor string `toml:"open_external_editor"`
	// FollowFK navigates from a foreign-key cell to the referenced row in its table.
	FollowFK string `toml:"follow_fk"`
}

// Config is the root configuration structure, mapped 1:1 to config.toml.
type Config struct {
	Settings    Settings     `toml:"settings"`
	Keybinds    Keybinds     `toml:"keybinds"`
	Theme       ThemeSection `toml:"theme"`
	Connections []Connection `toml:"connections"`
}

// DefaultConfig returns a Config with all fields set to their default values.
func DefaultConfig() *Config {
	return &Config{
		Settings: Settings{
			Theme:          "catppuccin-mocha",
			ExternalEditor: "vi",
		},
		Keybinds: Keybinds{
			RunQuery:           "ctrl+g",
			CancelQuery:        "ctrl+x",
			NewConnection:      "n",
			DeleteConnection:   "d",
			ToggleSidebar:      "ctrl+b",
			FocusEditor:        "ctrl+e",
			FocusResults:       "ctrl+r",
			OpenSettings:       "ctrl+p",
			FilterSidebar:      "/",
			Quit:               "q",
			Help:               "?",
			CopyCell:           "y",
			NextPage:           "ctrl+f",
			PrevPage:           "ctrl+u",
			ResizePanelLeft:    "ctrl+shift+left",
			ResizePanelRight:   "ctrl+shift+right",
			ResizePanelUp:      "ctrl+shift+up",
			ResizePanelDown:    "ctrl+shift+down",
			OpenExternalEditor: "ctrl+o",
			FollowFK:           "F",
		},
	}
}

// ConfigDir returns the resolved path to the dbterm config directory.
// On all platforms this is ~/.config/dbterm/.
func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".config", "dbterm"), nil
}

// configPath returns the full path to config.toml.
func configPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// Load reads ~/.config/dbterm/config.toml.
// If the file does not exist, it creates the directory and writes the default config.
// If any keybind conflicts are detected, the conflicting keys are reset to defaults
// and the list of conflicts is returned as a non-fatal warning alongside the config.
func Load() (*Config, []string, error) {
	path, err := configPath()
	if err != nil {
		return nil, nil, fmt.Errorf("config: resolving path: %w", err)
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		cfg := DefaultConfig()
		if saveErr := cfg.Save(); saveErr != nil {
			return nil, nil, fmt.Errorf("config: writing default config: %w", saveErr)
		}
		return cfg, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("config: reading file: %w", err)
	}

	cfg := DefaultConfig()
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, nil, fmt.Errorf("config: parsing TOML: %w", err)
	}

	conflicts := ValidateKeybinds(cfg.Keybinds)
	if len(conflicts) > 0 {
		defaults := DefaultConfig()
		cfg.Keybinds = defaults.Keybinds
	}

	return cfg, conflicts, nil
}

// Save marshals the Config to TOML and writes it to ~/.config/dbterm/config.toml.
// The directory is created if it does not exist.
// Comments written by the user are NOT preserved (go-toml/v2 does not support
// document-level comment round-trip in marshal mode; use the toml.Encoder for
// structured output).
func (c *Config) Save() error {
	dir, err := ConfigDir()
	if err != nil {
		return fmt.Errorf("config: resolving dir: %w", err)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("config: creating directory %q: %w", dir, err)
	}

	data, err := toml.Marshal(c)
	if err != nil {
		return fmt.Errorf("config: marshaling TOML: %w", err)
	}

	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("config: writing file %q: %w", path, err)
	}

	return nil
}

// ValidateKeybinds checks for duplicate key assignments across all actions.
// Returns a slice of human-readable conflict descriptions (empty if no conflicts).
func ValidateKeybinds(kb Keybinds) []string {
	seen := map[string]string{} // key string → first action name that claimed it
	var conflicts []string

	check := func(key, action string) {
		if key == "" {
			return
		}
		if prev, ok := seen[key]; ok {
			conflicts = append(conflicts,
				fmt.Sprintf("%q is bound to both %q and %q", key, prev, action))
			return
		}
		seen[key] = action
	}

	check(kb.RunQuery, "run_query")
	check(kb.CancelQuery, "cancel_query")
	check(kb.NewConnection, "new_connection")
	check(kb.DeleteConnection, "delete_connection")
	check(kb.ToggleSidebar, "toggle_sidebar")
	check(kb.FocusEditor, "focus_editor")
	check(kb.FocusResults, "focus_results")
	check(kb.OpenSettings, "open_settings")
	check(kb.FilterSidebar, "filter_sidebar")
	check(kb.Quit, "quit")
	check(kb.Help, "help")
	check(kb.CopyCell, "copy_cell")
	check(kb.NextPage, "next_page")
	check(kb.PrevPage, "prev_page")
	check(kb.ResizePanelLeft, "resize_panel_left")
	check(kb.ResizePanelRight, "resize_panel_right")
	check(kb.ResizePanelUp, "resize_panel_up")
	check(kb.ResizePanelDown, "resize_panel_down")
	check(kb.OpenExternalEditor, "open_external_editor")
	check(kb.FollowFK, "follow_fk")

	return conflicts
}
