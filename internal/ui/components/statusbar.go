package components

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/gabrielfloresousion/db-term/internal/config"
	"github.com/gabrielfloresousion/db-term/internal/ui/styles"
)

// StatusBar is the single-row bar always shown at the bottom of the screen.
type StatusBar struct {
	activeConn   string
	activeSchema string
	rowCount     int
	totalRows    int
	currentPage  int
	totalPages   int
	elapsed      time.Duration
	querying     bool
	// panelHints holds the context-sensitive shortcuts for the focused panel.
	// Updated via SetHints whenever focus changes.
	panelHints string
	message    string // transient feedback (e.g. "copied", "clipboard unavailable")
	messageTTL int    // decrements each tick; message cleared at 0
	width      int
	styles     styles.Styles
	keybinds   config.Keybinds
}

// NewStatusBar returns an initialised StatusBar.
func NewStatusBar(s styles.Styles, kb config.Keybinds) StatusBar {
	return StatusBar{styles: s, keybinds: kb}
}

// SetStyles updates lipgloss styles (called on theme change).
func (sb *StatusBar) SetStyles(s styles.Styles) { sb.styles = s }

// SetWidth updates the bar width.
func (sb *StatusBar) SetWidth(w int) { sb.width = w }

// SetConn updates the active connection and schema breadcrumb.
func (sb *StatusBar) SetConn(connName, schemaName string) {
	sb.activeConn = connName
	sb.activeSchema = schemaName
}

// SetResult updates the row count, elapsed time, and pagination info.
func (sb *StatusBar) SetResult(rowCount int, elapsed time.Duration) {
	sb.rowCount = rowCount
	sb.totalRows = rowCount
	sb.elapsed = elapsed
	sb.querying = false
}

// SetPage updates the current page and total pages (for paginated results).
func (sb *StatusBar) SetPage(current, total int) {
	sb.currentPage = current
	sb.totalPages = total
}

// SetQuerying switches the bar into the "query running" state.
func (sb *StatusBar) SetQuerying(q bool) { sb.querying = q }

// SetHints replaces the panel-specific shortcut hints shown on the right side.
// Called whenever the focused panel changes.
func (sb *StatusBar) SetHints(hints string) { sb.panelHints = hints }

// ShowMessage sets a transient feedback message visible for ~3 render ticks.
// Calling ShowMessage("") immediately clears any existing message.
func (sb *StatusBar) ShowMessage(msg string) {
	sb.message = msg
	if msg == "" {
		sb.messageTTL = 0
		return
	}
	sb.messageTTL = 30 // roughly 3 seconds at ~10fps
}

// Tick decrements the message TTL. Call on each tea.Tick or frame update.
func (sb *StatusBar) Tick() {
	if sb.messageTTL > 0 {
		sb.messageTTL--
		if sb.messageTTL == 0 {
			sb.message = ""
		}
	}
}

// Message returns the current transient message (may be empty).
func (sb *StatusBar) Message() string { return sb.message }

// View renders the status bar as a single line.
func (sb StatusBar) View() string {
	if sb.width == 0 {
		return ""
	}

	var left, right string

	if sb.message != "" {
		left = sb.styles.Success.Render("  " + sb.message)
	} else if sb.activeConn == "" {
		left = sb.styles.Muted.Render("  No connection   Press [" + sb.keybinds.NewConnection + "] to add one")
	} else {
		connStr := sb.styles.StatusConn.Render(" " + styles.IconDatabase + " " + sb.activeConn)
		if sb.activeSchema != "" {
			connStr += sb.styles.StatusMuted.Render(
				" " + styles.IconSeparator + " " + sb.activeSchema,
			)
		}
		left = connStr
	}

	var rightParts []string
	if sb.querying {
		rightParts = append(rightParts,
			sb.styles.Warning.Render(styles.IconConnecting+" Executing…"),
			sb.styles.Muted.Render("["+sb.keybinds.CancelQuery+"] Cancel"),
		)
	} else {
		if sb.rowCount > 0 {
			rightParts = append(rightParts,
				sb.styles.StatusMuted.Render(fmt.Sprintf("%d rows", sb.rowCount)),
			)
			if sb.elapsed > 0 {
				rightParts = append(rightParts,
					sb.styles.StatusMuted.Render(
						styles.IconClock+" "+sb.elapsed.Round(time.Millisecond).String(),
					),
				)
			}
		}
		// Show panel-specific hints when available, falling back to global hints.
		if sb.panelHints != "" {
			rightParts = append(rightParts, sb.styles.Muted.Render(sb.panelHints))
		} else {
			global := "[" + sb.keybinds.OpenSettings + "] Settings  [?] Help  [" + sb.keybinds.Quit + "] Quit"
			rightParts = append(rightParts, sb.styles.Muted.Render(global))
		}
	}

	right = strings.Join(rightParts, "  ")

	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	pad := sb.width - leftW - rightW - 2
	if pad < 1 {
		pad = 1
	}

	line := left + strings.Repeat(" ", pad) + right
	return sb.styles.StatusBar.Width(sb.width).Render(line)
}
