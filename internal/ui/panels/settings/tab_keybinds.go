package settings

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gabrielfloresousion/db-term/internal/config"
	"github.com/gabrielfloresousion/db-term/internal/ui/styles"
)

type tabKeybinds struct {
	kb     config.Keybinds
	cursor int
	styles styles.Styles
}

type keybindRow struct {
	action string
	label  string
	desc   string
	getter func(config.Keybinds) string
}

var keybindRows = []keybindRow{
	{"run_query", "Run query", "Execute the SQL in the editor against the active connection", func(k config.Keybinds) string { return k.RunQuery }},
	{"cancel_query", "Cancel query", "Interrupt a query that is currently running", func(k config.Keybinds) string { return k.CancelQuery }},
	{"new_connection", "New connection", "Open the connection form to add a new database", func(k config.Keybinds) string { return k.NewConnection }},
	{"delete_connection", "Delete connection", "Remove the selected connection and its saved password", func(k config.Keybinds) string { return k.DeleteConnection }},
	{"toggle_sidebar", "Toggle sidebar", "Show or hide the connections sidebar", func(k config.Keybinds) string { return k.ToggleSidebar }},
	{"focus_editor", "Focus editor", "Move keyboard focus to the SQL editor panel", func(k config.Keybinds) string { return k.FocusEditor }},
	{"focus_results", "Focus results", "Move keyboard focus to the query results panel", func(k config.Keybinds) string { return k.FocusResults }},
	{"open_settings", "Open settings", "Toggle this settings panel (works from any panel)", func(k config.Keybinds) string { return k.OpenSettings }},
	{"filter_sidebar", "Filter sidebar", "Enter filter mode to search schemas and tables by name", func(k config.Keybinds) string { return k.FilterSidebar }},
	{"copy_cell", "Copy cell", "Copy the value of the focused results cell to the clipboard", func(k config.Keybinds) string { return k.CopyCell }},
	{"follow_fk", "Follow foreign key", "Open the row referenced by the focused foreign-key cell", func(k config.Keybinds) string { return k.FollowFK }},
	{"next_page", "Next page", "Load the next page of query results (500 rows per page)", func(k config.Keybinds) string { return k.NextPage }},
	{"prev_page", "Previous page", "Load the previous page of query results", func(k config.Keybinds) string { return k.PrevPage }},
	{"open_external_editor", "Open in editor", "Open the SQL buffer in the configured external editor (vi)", func(k config.Keybinds) string { return k.OpenExternalEditor }},
	{"quit", "Quit", "Exit db-term (disconnects all active connections)", func(k config.Keybinds) string { return k.Quit }},
	{"help", "Help", "Show the keyboard shortcut reference overlay", func(k config.Keybinds) string { return k.Help }},
}

func newTabKeybinds(kb config.Keybinds, s styles.Styles) tabKeybinds {
	return tabKeybinds{kb: kb, styles: s}
}

func (t tabKeybinds) update(msg tea.KeyMsg) tabKeybinds {
	switch msg.String() {
	case "up", "k":
		if t.cursor > 0 {
			t.cursor--
		}
	case "down", "j":
		if t.cursor < len(keybindRows)-1 {
			t.cursor++
		}
	}
	return t
}

func (t tabKeybinds) view(w, h int) string {
	const labelW = 20
	const shortcutW = 14

	header := t.styles.Subtext.Render(
		fmt.Sprintf("  %-*s  %-*s  %s", labelW, "Action", shortcutW, "Shortcut", "Description"),
	)
	sep := t.styles.Muted.Render("  " + strings.Repeat("─", w-4))

	var rows []string
	rows = append(rows, header, sep)

	descW := w - labelW - shortcutW - 8
	if descW < 10 {
		descW = 10
	}

	for i, row := range keybindRows {
		shortcut := row.getter(t.kb)
		desc := row.desc
		descRunes := []rune(desc)
		if len(descRunes) > descW {
			desc = string(descRunes[:descW-1]) + "…"
		}
		line := fmt.Sprintf("  %-*s  %-*s  %s", labelW, row.label, shortcutW, shortcut, desc)
		if i == t.cursor {
			rows = append(rows, t.styles.TreeSelected.Render(line))
		} else {
			rows = append(rows, t.styles.Text.Render(line))
		}
	}

	rows = append(rows, "")
	rows = append(rows, t.styles.Muted.Render("  Keybind editing available in a future version."))

	return strings.Join(rows, "\n")
}
