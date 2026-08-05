package settings

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gabrielfloresousion/db-term/internal/config"
	"github.com/gabrielfloresousion/db-term/internal/ui/styles"
)

type tabConnections struct {
	conns  []config.Connection
	cursor int
	styles styles.Styles
}

func newTabConnections(conns []config.Connection, s styles.Styles) tabConnections {
	return tabConnections{conns: conns, styles: s}
}

func (t tabConnections) update(msg tea.KeyMsg) tabConnections {
	switch msg.String() {
	case "up", "k":
		if t.cursor > 0 {
			t.cursor--
		}
	case "down", "j":
		if t.cursor < len(t.conns)-1 {
			t.cursor++
		}
	}
	return t
}

func (t tabConnections) view(w, h int) string {
	header := t.styles.Subtext.Render(
		fmt.Sprintf("  %-20s  %-12s  %-22s", "Name", "Driver", "Host:Port"),
	)
	sep := t.styles.Muted.Render("  " + strings.Repeat("─", w-4))

	var rows []string
	rows = append(rows, header, sep)

	if len(t.conns) == 0 {
		rows = append(rows, t.styles.Muted.Render("  No connections saved.  Press [n] from the main view to add one."))
	}

	for i, conn := range t.conns {
		hostPort := fmt.Sprintf("%s:%d", conn.Host, conn.Port)
		line := fmt.Sprintf("  %-20s  %-12s  %-22s",
			truncStr(conn.Name, 20),
			conn.Driver,
			truncStr(hostPort, 22),
		)
		if i == t.cursor {
			rows = append(rows, t.styles.TreeSelected.Render(line))
		} else {
			rows = append(rows, t.styles.Text.Render(line))
		}
	}

	rows = append(rows, "")
	rows = append(rows, t.styles.Muted.Render("  [n] New   [d] Delete   [Enter] Edit   Manage connections from main view."))

	return strings.Join(rows, "\n")
}

func truncStr(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}
