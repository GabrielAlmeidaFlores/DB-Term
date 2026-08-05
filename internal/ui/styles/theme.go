package styles

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/gabrielfloresousion/db-term/internal/config"
)

// Theme holds every color used in the UI, including SQL syntax highlight colors.
// Panels receive a resolved Theme at render time — no hardcoded hex values elsewhere.
type Theme struct {
	Name string

	Bg      string // main background
	Surface string // panel / card background
	Border  string // inactive panel border
	Focus   string // focused panel border + primary accent

	Text    string // primary text
	Subtext string // secondary text (hints, column types)
	Muted   string // disabled text, placeholders

	Primary string // titles, selected items
	Success string // connected state, true values
	Warning string // connecting state, caution
	Error   string // error state, false values

	SynKeyword  string // SELECT FROM WHERE JOIN ORDER GROUP BY …
	SynDML      string // INSERT UPDATE DELETE CREATE DROP ALTER …
	SynFunction string // COUNT SUM AVG MIN MAX COALESCE NULLIF …
	SynString   string // 'string literals'
	SynNumber   string // 42, 3.14, NULL, TRUE, FALSE
	SynComment  string // -- single line  /* block */
	SynOperator string // = > < <> AND OR NOT IN IS LIKE BETWEEN
}

var builtinThemes = map[string]Theme{
	"catppuccin-mocha": {
		Name: "Catppuccin Mocha",
		Bg:   "#1e1e2e", Surface: "#313244", Border: "#45475a", Focus: "#89b4fa",
		Text: "#cdd6f4", Subtext: "#a6adc8", Muted: "#6c7086",
		Primary: "#89b4fa", Success: "#a6e3a1", Warning: "#f9e2af", Error: "#f38ba8",
		SynKeyword: "#cba6f7", SynDML: "#f38ba8", SynFunction: "#94e2d5",
		SynString: "#fab387", SynNumber: "#f9e2af", SynComment: "#6c7086", SynOperator: "#89b4fa",
	},
	"catppuccin-latte": {
		Name: "Catppuccin Latte",
		Bg:   "#eff1f5", Surface: "#e6e9ef", Border: "#bcc0cc", Focus: "#1e66f5",
		Text: "#4c4f69", Subtext: "#5c5f77", Muted: "#9ca0b0",
		Primary: "#1e66f5", Success: "#40a02b", Warning: "#df8e1d", Error: "#d20f39",
		SynKeyword: "#8839ef", SynDML: "#d20f39", SynFunction: "#179299",
		SynString: "#fe640b", SynNumber: "#df8e1d", SynComment: "#9ca0b0", SynOperator: "#1e66f5",
	},
	"dracula": {
		Name: "Dracula",
		Bg:   "#282a36", Surface: "#383a59", Border: "#44475a", Focus: "#bd93f9",
		Text: "#f8f8f2", Subtext: "#6272a4", Muted: "#6272a4",
		Primary: "#bd93f9", Success: "#50fa7b", Warning: "#f1fa8c", Error: "#ff5555",
		SynKeyword: "#bd93f9", SynDML: "#ff5555", SynFunction: "#8be9fd",
		SynString: "#f1fa8c", SynNumber: "#bd93f9", SynComment: "#6272a4", SynOperator: "#ff79c6",
	},
	"tokyo-night": {
		Name: "Tokyo Night",
		Bg:   "#1a1b26", Surface: "#24283b", Border: "#414868", Focus: "#7aa2f7",
		Text: "#c0caf5", Subtext: "#9aa5ce", Muted: "#565f89",
		Primary: "#7aa2f7", Success: "#9ece6a", Warning: "#e0af68", Error: "#f7768e",
		SynKeyword: "#bb9af7", SynDML: "#f7768e", SynFunction: "#2ac3de",
		SynString: "#9ece6a", SynNumber: "#ff9e64", SynComment: "#565f89", SynOperator: "#89ddff",
	},
	"gruvbox": {
		Name: "Gruvbox Dark",
		Bg:   "#282828", Surface: "#3c3836", Border: "#504945", Focus: "#83a598",
		Text: "#ebdbb2", Subtext: "#d5c4a1", Muted: "#928374",
		Primary: "#83a598", Success: "#b8bb26", Warning: "#fabd2f", Error: "#fb4934",
		SynKeyword: "#d3869b", SynDML: "#fb4934", SynFunction: "#8ec07c",
		SynString: "#b8bb26", SynNumber: "#d79921", SynComment: "#928374", SynOperator: "#83a598",
	},
}

// Resolve returns the active Theme.
//
//   - themeName is the ID string from config.Settings.Theme.
//   - custom is the ThemeConfig block from config.Theme.Custom (used only
//     when themeName == "custom").
//
// Falls back to catppuccin-mocha if themeName is unrecognised.
func Resolve(themeName string, custom config.ThemeConfig) Theme {
	if themeName == "custom" {
		return FromConfig(custom)
	}
	if t, ok := builtinThemes[themeName]; ok {
		return t
	}
	return builtinThemes["catppuccin-mocha"]
}

// FromConfig converts a config.ThemeConfig (from TOML) into a Theme.
// Empty hex fields in the ThemeConfig fall back to the catppuccin-mocha value.
func FromConfig(tc config.ThemeConfig) Theme {
	fallback := builtinThemes["catppuccin-mocha"]
	pick := func(v, fb string) string {
		if v != "" {
			return v
		}
		return fb
	}
	return Theme{
		Name:        "custom",
		Bg:          pick(tc.Bg, fallback.Bg),
		Surface:     pick(tc.Surface, fallback.Surface),
		Border:      pick(tc.Border, fallback.Border),
		Focus:       pick(tc.Focus, fallback.Focus),
		Text:        pick(tc.Text, fallback.Text),
		Subtext:     pick(tc.Subtext, fallback.Subtext),
		Muted:       pick(tc.Muted, fallback.Muted),
		Primary:     pick(tc.Primary, fallback.Primary),
		Success:     pick(tc.Success, fallback.Success),
		Warning:     pick(tc.Warning, fallback.Warning),
		Error:       pick(tc.Error, fallback.Error),
		SynKeyword:  pick(tc.SynKeyword, fallback.SynKeyword),
		SynDML:      pick(tc.SynDML, fallback.SynDML),
		SynFunction: pick(tc.SynFunction, fallback.SynFunction),
		SynString:   pick(tc.SynString, fallback.SynString),
		SynNumber:   pick(tc.SynNumber, fallback.SynNumber),
		SynComment:  pick(tc.SynComment, fallback.SynComment),
		SynOperator: pick(tc.SynOperator, fallback.SynOperator),
	}
}

// Names returns all available theme IDs in display order.
func Names() []string {
	return []string{
		"catppuccin-mocha",
		"catppuccin-latte",
		"dracula",
		"tokyo-night",
		"gruvbox",
		"custom",
	}
}

// Styles holds every lipgloss.Style used in the application, derived from a Theme.
// Panels call ToStyles(theme) once at startup and whenever the theme changes.
// No panel calls lipgloss.NewStyle() directly.
type Styles struct {
	PanelFocused   lipgloss.Style
	PanelUnfocused lipgloss.Style
	PanelError     lipgloss.Style

	Title   lipgloss.Style
	Text    lipgloss.Style
	Subtext lipgloss.Style
	Muted   lipgloss.Style
	Success lipgloss.Style
	Warning lipgloss.Style
	Error   lipgloss.Style

	TableHeader   lipgloss.Style
	TableRow      lipgloss.Style
	TableSelected lipgloss.Style
	TablePK       lipgloss.Style // PK column header: Primary color + bold

	TreeSelected lipgloss.Style
	TreeConn     lipgloss.Style
	TreeSchema   lipgloss.Style
	TreeTable    lipgloss.Style
	TreeView     lipgloss.Style

	StatusBar   lipgloss.Style
	StatusConn  lipgloss.Style // active connection name (Primary color)
	StatusMuted lipgloss.Style

	SynKeyword  lipgloss.Style
	SynDML      lipgloss.Style
	SynFunction lipgloss.Style
	SynString   lipgloss.Style
	SynNumber   lipgloss.Style
	SynComment  lipgloss.Style
	SynOperator lipgloss.Style
	Cursor      lipgloss.Style
}

// ToStyles builds every lipgloss.Style from the given Theme.
// Call this once at startup and again on ThemeChangedMsg.
func ToStyles(t Theme) Styles {
	c := func(hex string) lipgloss.Color { return lipgloss.Color(hex) }

	panelBase := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(c(t.Border)).
		Padding(0, 1) // 1 column of breathing room on each side inside the border

	return Styles{
		PanelFocused: panelBase.
			BorderForeground(c(t.Focus)),
		PanelUnfocused: panelBase,
		PanelError: panelBase.
			BorderForeground(c(t.Error)),

		Title:   lipgloss.NewStyle().Bold(true).Foreground(c(t.Primary)),
		Text:    lipgloss.NewStyle().Foreground(c(t.Text)),
		Subtext: lipgloss.NewStyle().Foreground(c(t.Subtext)),
		Muted:   lipgloss.NewStyle().Foreground(c(t.Muted)),
		Success: lipgloss.NewStyle().Foreground(c(t.Success)),
		Warning: lipgloss.NewStyle().Foreground(c(t.Warning)),
		Error:   lipgloss.NewStyle().Foreground(c(t.Error)),

		// TableHeader has no bottom border — the results grid draws its own
		// separator line so each header cell must stay on a single line.
		TableHeader: lipgloss.NewStyle().
			Bold(true).
			Foreground(c(t.Primary)),
		TableRow: lipgloss.NewStyle().
			Foreground(c(t.Text)),
		TableSelected: lipgloss.NewStyle().
			Bold(true).
			Foreground(c(t.Text)).
			Background(c(t.Surface)),
		TablePK: lipgloss.NewStyle().
			Bold(true).
			Foreground(c(t.Warning)),

		TreeSelected: lipgloss.NewStyle().
			Bold(true).
			Foreground(c(t.Text)).
			Background(c(t.Surface)),
		TreeConn: lipgloss.NewStyle().
			Bold(true).
			Foreground(c(t.Primary)),
		TreeSchema: lipgloss.NewStyle().
			Foreground(c(t.Subtext)),
		TreeTable: lipgloss.NewStyle().
			Foreground(c(t.Text)),
		TreeView: lipgloss.NewStyle().
			Foreground(c(t.Muted)).
			Italic(true),

		StatusBar: lipgloss.NewStyle().
			Background(c(t.Surface)).
			Foreground(c(t.Subtext)),
		StatusConn: lipgloss.NewStyle().
			Background(c(t.Surface)).
			Foreground(c(t.Primary)).
			Bold(true),
		StatusMuted: lipgloss.NewStyle().
			Background(c(t.Surface)).
			Foreground(c(t.Muted)),

		SynKeyword:  lipgloss.NewStyle().Bold(true).Foreground(c(t.SynKeyword)),
		SynDML:      lipgloss.NewStyle().Foreground(c(t.SynDML)),
		SynFunction: lipgloss.NewStyle().Foreground(c(t.SynFunction)),
		SynString:   lipgloss.NewStyle().Foreground(c(t.SynString)),
		SynNumber:   lipgloss.NewStyle().Foreground(c(t.SynNumber)),
		SynComment:  lipgloss.NewStyle().Italic(true).Foreground(c(t.SynComment)),
		SynOperator: lipgloss.NewStyle().Foreground(c(t.SynOperator)),
		Cursor:      lipgloss.NewStyle().Reverse(true),
	}
}

// PanelStyle returns a panel border style sized to w×h.
// focused controls whether the Focus or Border color is used.
// errored overrides both with the Error color (takes priority).
func PanelStyle(t Theme, w, h int, focused, errored bool) lipgloss.Style {
	color := t.Border
	if focused {
		color = t.Focus
	}
	if errored {
		color = t.Error
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(color)).
		Width(w).
		Height(h)
}
