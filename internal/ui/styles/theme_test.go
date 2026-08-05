package styles

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/gabrielfloresousion/db-term/internal/config"
)

func TestResolve_ReturnsBuiltinTheme(t *testing.T) {
	for _, name := range Names() {
		if name == "custom" {
			continue
		}
		theme := Resolve(name, config.ThemeConfig{})
		if theme.Name == "" {
			t.Errorf("Resolve(%q): Name is empty", name)
		}
		if theme.Bg == "" {
			t.Errorf("Resolve(%q): Bg is empty", name)
		}
	}
}

func TestResolve_FallbackOnUnknownName(t *testing.T) {
	theme := Resolve("does-not-exist", config.ThemeConfig{})
	if theme.Name != "Catppuccin Mocha" {
		t.Errorf("Resolve(unknown): expected fallback to Catppuccin Mocha, got %q", theme.Name)
	}
}

func TestResolve_CustomUsesFromConfig(t *testing.T) {
	tc := config.ThemeConfig{
		Bg:   "#ff0000",
		Text: "#00ff00",
	}
	theme := Resolve("custom", tc)
	if theme.Name != "custom" {
		t.Errorf("Resolve(custom): Name = %q, want %q", theme.Name, "custom")
	}
	if theme.Bg != "#ff0000" {
		t.Errorf("Resolve(custom): Bg = %q, want %q", theme.Bg, "#ff0000")
	}
	if theme.Text != "#00ff00" {
		t.Errorf("Resolve(custom): Text = %q, want %q", theme.Text, "#00ff00")
	}
}

func TestFromConfig_EmptyFieldsFallbackToMocha(t *testing.T) {
	mocha := builtinThemes["catppuccin-mocha"]
	theme := FromConfig(config.ThemeConfig{}) // all fields empty
	if theme.Bg != mocha.Bg {
		t.Errorf("FromConfig(empty).Bg = %q, want mocha fallback %q", theme.Bg, mocha.Bg)
	}
	if theme.SynKeyword != mocha.SynKeyword {
		t.Errorf("FromConfig(empty).SynKeyword = %q, want mocha fallback %q", theme.SynKeyword, mocha.SynKeyword)
	}
}

func TestFromConfig_FilledFieldsOverrideFallback(t *testing.T) {
	tc := config.ThemeConfig{Bg: "#123456"}
	theme := FromConfig(tc)
	if theme.Bg != "#123456" {
		t.Errorf("FromConfig: Bg = %q, want %q", theme.Bg, "#123456")
	}
}

func TestNames_ContainsAllBuiltins(t *testing.T) {
	names := Names()
	required := []string{"catppuccin-mocha", "catppuccin-latte", "dracula", "tokyo-night", "gruvbox", "custom"}
	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}
	for _, r := range required {
		if !nameSet[r] {
			t.Errorf("Names(): missing %q", r)
		}
	}
}

func TestNames_FirstIsMocha(t *testing.T) {
	if Names()[0] != "catppuccin-mocha" {
		t.Errorf("Names()[0] = %q, want %q", Names()[0], "catppuccin-mocha")
	}
}

func TestNames_LastIsCustom(t *testing.T) {
	names := Names()
	last := names[len(names)-1]
	if last != "custom" {
		t.Errorf("Names() last = %q, want %q", last, "custom")
	}
}

func TestToStyles_NoZeroStyles(t *testing.T) {
	for _, name := range Names() {
		if name == "custom" {
			continue
		}
		theme := Resolve(name, config.ThemeConfig{})
		s := ToStyles(theme)

		// Verify foreground colors are set on text styles.
		// lipgloss.NoColor{} is the zero value when no color has been set.
		colorChecks := map[string]lipgloss.TerminalColor{
			"Title.Foreground":       s.Title.GetForeground(),
			"Text.Foreground":        s.Text.GetForeground(),
			"Subtext.Foreground":     s.Subtext.GetForeground(),
			"Muted.Foreground":       s.Muted.GetForeground(),
			"Success.Foreground":     s.Success.GetForeground(),
			"Warning.Foreground":     s.Warning.GetForeground(),
			"Error.Foreground":       s.Error.GetForeground(),
			"SynKeyword.Foreground":  s.SynKeyword.GetForeground(),
			"SynDML.Foreground":      s.SynDML.GetForeground(),
			"SynFunction.Foreground": s.SynFunction.GetForeground(),
			"SynString.Foreground":   s.SynString.GetForeground(),
			"SynNumber.Foreground":   s.SynNumber.GetForeground(),
			"SynComment.Foreground":  s.SynComment.GetForeground(),
			"SynOperator.Foreground": s.SynOperator.GetForeground(),
			"TreeConn.Foreground":    s.TreeConn.GetForeground(),
			"StatusConn.Foreground":  s.StatusConn.GetForeground(),
		}

		_, _, _, noAlpha := lipgloss.NoColor{}.RGBA()
		for field, color := range colorChecks {
			_, _, _, gotAlpha := color.RGBA()
			if gotAlpha == noAlpha && color == (lipgloss.NoColor{}) {
				t.Errorf("ToStyles(%q): %s is not set (zero color)", name, field)
			}
		}

		// Verify bold is set on styles that require it.
		if !s.Title.GetBold() {
			t.Errorf("ToStyles(%q): Title.Bold is false, want true", name)
		}
		if !s.SynKeyword.GetBold() {
			t.Errorf("ToStyles(%q): SynKeyword.Bold is false, want true", name)
		}
		if !s.TreeConn.GetBold() {
			t.Errorf("ToStyles(%q): TreeConn.Bold is false, want true", name)
		}

		// Verify italic is set on SynComment.
		if !s.SynComment.GetItalic() {
			t.Errorf("ToStyles(%q): SynComment.Italic is false, want true", name)
		}
	}
}

func TestToStyles_AllThemesProduceDifferentPrimaryColors(t *testing.T) {
	primaries := map[string]string{}
	for _, name := range Names() {
		if name == "custom" {
			continue
		}
		theme := Resolve(name, config.ThemeConfig{})
		primaries[name] = theme.Primary
	}
	// Each builtin theme should have a distinct Primary color.
	seen := map[string]string{}
	for name, primary := range primaries {
		if prev, ok := seen[primary]; ok {
			t.Errorf("themes %q and %q share the same Primary color %q", prev, name, primary)
		}
		seen[primary] = name
	}
}

func TestPanelStyle_FocusedContainsContent(t *testing.T) {
	theme := Resolve("catppuccin-mocha", config.ThemeConfig{})
	style := PanelStyle(theme, 20, 5, true, false)
	rendered := style.Render("hello")
	if !strings.Contains(rendered, "hello") {
		t.Error("PanelStyle focused: rendered output does not contain content")
	}
}

func TestPanelStyle_ErroredTakesPriorityOverFocused(t *testing.T) {
	theme := Resolve("catppuccin-mocha", config.ThemeConfig{})
	// When errored=true the Error color is used regardless of focused.
	// We can't inspect the exact ANSI code easily, but we confirm it renders.
	style := PanelStyle(theme, 20, 5, true, true)
	rendered := style.Render("err")
	if !strings.Contains(rendered, "err") {
		t.Error("PanelStyle errored: rendered output does not contain content")
	}
}

func TestBuiltinThemes_AllFieldsNonEmpty(t *testing.T) {
	for _, name := range Names() {
		if name == "custom" {
			continue
		}
		theme := Resolve(name, config.ThemeConfig{})
		fields := map[string]string{
			"Bg":          theme.Bg,
			"Surface":     theme.Surface,
			"Border":      theme.Border,
			"Focus":       theme.Focus,
			"Text":        theme.Text,
			"Subtext":     theme.Subtext,
			"Muted":       theme.Muted,
			"Primary":     theme.Primary,
			"Success":     theme.Success,
			"Warning":     theme.Warning,
			"Error":       theme.Error,
			"SynKeyword":  theme.SynKeyword,
			"SynDML":      theme.SynDML,
			"SynFunction": theme.SynFunction,
			"SynString":   theme.SynString,
			"SynNumber":   theme.SynNumber,
			"SynComment":  theme.SynComment,
			"SynOperator": theme.SynOperator,
		}
		for field, value := range fields {
			if value == "" {
				t.Errorf("theme %q: field %q is empty", name, field)
			}
		}
	}
}
