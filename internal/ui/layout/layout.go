// Package layout composes the db-term panels into the final screen view.
package layout

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	// DefaultSidebarWidth is the initial sidebar column width in characters.
	DefaultSidebarWidth = 28
	// DefaultEditorRatio is the initial percentage of the right column given to the editor.
	DefaultEditorRatio = 40
	// StatusBarHeight is the fixed height of the status bar row.
	StatusBarHeight = 1

	// SidebarStep is the number of columns added or removed per sidebar resize keypress.
	SidebarStep = 3
	// EditorRatioStep is the percentage-point change per editor-split resize keypress.
	EditorRatioStep = 5

	// SidebarMinW is the minimum sidebar width in columns.
	// 20 columns is the smallest that can display the "Connections" title
	// (icon ≈ 2 + 2 spaces + 11 chars = 15 visible cols) plus 4 cols of
	// border and padding without content overflow.
	SidebarMinW = 20
	// SidebarMaxW is the maximum sidebar width in columns.
	SidebarMaxW = 60
	// EditorMinRatio is the minimum editor height as a percentage of the right column.
	EditorMinRatio = 15
	// EditorMaxRatio is the maximum editor height as a percentage of the right column.
	EditorMaxRatio = 85

	// PanelMinW is the absolute minimum panel width (border + 1 content + border).
	PanelMinW = 4
	// PanelMinH is the absolute minimum panel height (border + 1 content + border).
	PanelMinH = 3
	// TopReserved is the number of rows reserved at the top of the screen.
	// This ensures panel top-borders are never at absolute row 0, which is
	// clipped in tmux and some terminal emulators.
	TopReserved = 1
)

// Dimensions holds the computed size for every panel given the current state.
type Dimensions struct {
	Width  int
	Height int

	SidebarW int
	SidebarH int

	RightW   int
	EditorH  int
	ResultsH int

	StatusW int
}

// Compute calculates all panel dimensions.
//   - sidebarVisible: false collapses the sidebar to zero width
//   - sidebarW: current sidebar width (use DefaultSidebarWidth when unset)
//   - editorRatio: percentage (0–100) of the right column for the editor panel
//
// The function guarantees that EditorH + ResultsH == availH so the right
// column never overflows the terminal height, even when the terminal is
// smaller than the minimum preferred sizes.
//
// One row is reserved at the top of the screen so that panel top-borders are
// never placed at absolute row 0 of the terminal. In tmux and some terminal
// emulators the very first row can be clipped or consumed, making borders
// at row 0 invisible. The reserved row shifts all panel content down by one.
func Compute(totalW, totalH int, sidebarVisible bool, sidebarW, editorRatio int) Dimensions {
	d := Dimensions{Width: totalW, Height: totalH}

	d.StatusW = clamp(totalW, 1, totalW)

	// Reserve 1 row at the top + 1 row for the status bar at the bottom.
	availH := totalH - StatusBarHeight - TopReserved
	if availH < 2 {
		availH = 2
	}

	if sidebarVisible {
		maxSidebar := totalW - PanelMinW
		if maxSidebar < 0 {
			maxSidebar = 0
		}
		d.SidebarW = clamp(sidebarW, SidebarMinW, Clamp(SidebarMaxW, SidebarMinW, maxSidebar))
	}
	d.SidebarH = availH

	d.RightW = totalW - d.SidebarW
	if d.RightW < PanelMinW {
		d.RightW = PanelMinW
	}

	// Split availH between editor and results. When availH is too small to
	// meet the preferred minimums, split evenly rather than overflowing.
	ratio := clamp(editorRatio, EditorMinRatio, EditorMaxRatio)
	d.EditorH = availH * ratio / 100
	d.ResultsH = availH - d.EditorH

	// Apply soft minimums only when there is enough space for both.
	const softMin = 3
	if availH >= softMin*2 {
		if d.EditorH < softMin {
			d.EditorH = softMin
			d.ResultsH = availH - d.EditorH
		}
		if d.ResultsH < softMin {
			d.ResultsH = softMin
			d.EditorH = availH - d.ResultsH
		}
	} else {
		// Terminal too short — split evenly, floor for editor.
		d.EditorH = availH / 2
		d.ResultsH = availH - d.EditorH
	}

	// Hard floor: never render at zero height.
	if d.EditorH < 1 {
		d.EditorH = 1
	}
	if d.ResultsH < 1 {
		d.ResultsH = 1
	}

	return d
}

// Render joins the rendered panel strings into the final screen string.
// Each panel string is expected to already include its own border.
// overlay is an optional centred string (modal / connform / settings) rendered on top.
func Render(
	sidebarView, editorView, resultsView, statusView string,
	d Dimensions,
	overlay string,
) string {
	rightCol := lipgloss.JoinVertical(lipgloss.Left, editorView, resultsView)

	var mainArea string
	if d.SidebarW > 0 {
		mainArea = lipgloss.JoinHorizontal(lipgloss.Top, sidebarView, rightCol)
	} else {
		mainArea = rightCol
	}

	// The reserved top row ensures panel borders are never placed at absolute
	// row 0, which is invisible in tmux panes and some terminal emulators.
	reserved := strings.Repeat(" ", d.Width)
	screen := lipgloss.JoinVertical(lipgloss.Left, reserved, mainArea, statusView)

	if overlay != "" {
		return overlayOnScreen(screen, overlay, d.Width, d.Height)
	}
	return screen
}

// overlayOnScreen places an overlay string on top of screen.
// In Bubble Tea, overlays replace the entire screen content; the overlay
// component positions itself with padding to appear centred.
func overlayOnScreen(_, overlay string, _, _ int) string {
	return overlay
}

// Clamp returns v clamped to [min, max].
func Clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func clamp(v, min, max int) int { return Clamp(v, min, max) }
