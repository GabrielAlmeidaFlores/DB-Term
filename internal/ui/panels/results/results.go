// Package results implements the query results table panel.
package results

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gabrielfloresousion/db-term/internal/config"
	"github.com/gabrielfloresousion/db-term/internal/types"
	"github.com/gabrielfloresousion/db-term/internal/ui/styles"
	runewidth "github.com/mattn/go-runewidth"
)

// CopyAction signals that the user wants to copy the focused cell value.
type CopyAction struct {
	Value string // the full (non-truncated) cell value
}

// EditCellAction signals that the user pressed 'e' on a cell and wants to
// edit it. The caller is responsible for opening a prompt pre-filled with
// Value and, on confirmation, executing the appropriate UPDATE statement.
type EditCellAction struct {
	Column  string   // column name being edited
	Value   string   // current raw cell value
	Row     int      // zero-based row index within the current page
	Col     int      // zero-based column index
	Columns []string // all column names (for WHERE clause generation)
	RowVals []string // all original cell values in the row (parallel to Columns)
}

// SortAction signals that the user wants to re-run the current query sorted
// by the column under the cursor. Asc is true for ascending, false for descending.
type SortAction struct {
	Column string
	Asc    bool
}

// SearchAction signals that the user pressed '/' to filter results by the
// column under the cursor. The caller opens a prompt and builds the WHERE clause.
type SearchAction struct {
	Column string
}

// FKNavigateAction signals that the user pressed the FollowFK key on a cell
// whose column is a foreign key. The caller builds a query against the
// referenced table filtered to the current cell value.
type FKNavigateAction struct {
	Value string
	FK    types.ForeignKey
}

// Model is the Bubble Tea model for the query results panel.
// It renders a custom scrollable grid with a free-moving cell cursor
// — not backed by bubbles/table, which only supports row-level selection.
type Model struct {
	paged     PagedResult
	columns   []string
	colWidths []int // display width per column: max(header, content) capped at maxColWidth

	cursorRow int // focused row index within the current page
	cursorCol int // focused column index
	rowOffset int // first visible row (vertical scroll)
	colOffset int // first visible column (horizontal scroll)

	elapsed  time.Duration
	rowCount int
	err      error
	querying bool
	spinner  spinner.Model
	width    int
	height   int
	focused  bool
	styles   styles.Styles
	keybinds config.Keybinds

	lastCopy       *CopyAction
	lastEditCell   *EditCellAction
	lastSort       *SortAction
	lastSearch     *SearchAction
	lastFKNavigate *FKNavigateAction

	fkInfo map[string]*types.ForeignKey

	// sortCol and sortAsc track the active sort for header indicator display.
	sortCol string
	sortAsc bool
}

// New returns an initialised results Model.
func New(s styles.Styles, kb config.Keybinds) Model {
	return Model{
		spinner:  newSpinner(s),
		styles:   s,
		keybinds: kb,
	}
}

// newSpinner creates a themed spinner using the active styles.
func newSpinner(s styles.Styles) spinner.Model {
	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	sp.Style = s.Warning
	return sp
}

// Init starts the spinner tick.
func (m Model) Init() tea.Cmd { return m.spinner.Tick }

// SetStyles updates lipgloss styles (called on theme change).
func (m *Model) SetStyles(s styles.Styles) {
	m.styles = s
	m.spinner = newSpinner(s)
}

// SetSize sets the panel dimensions and reflows the visible column range.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.scrollToShowCursor()
}

// Focus gives keyboard focus to the panel.
func (m *Model) Focus() { m.focused = true }

// Blur removes keyboard focus.
func (m *Model) Blur() { m.focused = false }

// SetQuerying switches the panel into/out of the "query running" state.
func (m *Model) SetQuerying(q bool) { m.querying = q }

// SetResult populates the panel with a completed query result.
func (m *Model) SetResult(columns []string, rows [][]string, elapsed time.Duration, err error) {
	m.querying = false
	m.err = err
	m.elapsed = elapsed
	m.columns = columns
	m.rowCount = len(rows)
	m.paged = NewPagedResult(rows)
	m.cursorRow = 0
	m.cursorCol = 0
	m.rowOffset = 0
	m.colOffset = 0
	m.computeColWidths()
}

// NextPage advances to the next page and resets the cursor to the top.
func (m *Model) NextPage() {
	m.paged.NextPage()
	m.cursorRow = 0
	m.rowOffset = 0
	m.computeColWidths()
}

// PrevPage moves to the previous page and resets the cursor to the top.
func (m *Model) PrevPage() {
	m.paged.PrevPage()
	m.cursorRow = 0
	m.rowOffset = 0
	m.computeColWidths()
}

// TakeCopy returns and clears the pending copy action.
func (m *Model) TakeCopy() *CopyAction {
	a := m.lastCopy
	m.lastCopy = nil
	return a
}

// TakeEditCell returns and clears the pending edit-cell action.
func (m *Model) TakeEditCell() *EditCellAction {
	a := m.lastEditCell
	m.lastEditCell = nil
	return a
}

// TakeSortAction returns and clears the pending sort action.
func (m *Model) TakeSortAction() *SortAction {
	a := m.lastSort
	m.lastSort = nil
	return a
}

// TakeSearchAction returns and clears the pending column-search action.
func (m *Model) TakeSearchAction() *SearchAction {
	a := m.lastSearch
	m.lastSearch = nil
	return a
}

// TakeFKNavigate returns and clears the pending FK navigation action.
func (m *Model) TakeFKNavigate() *FKNavigateAction {
	a := m.lastFKNavigate
	m.lastFKNavigate = nil
	return a
}

// SetFKInfo stores foreign-key metadata for the current result set.
// fkInfo maps column name to its FK reference; nil clears all FK info.
func (m *Model) SetFKInfo(fkInfo map[string]*types.ForeignKey) {
	m.fkInfo = fkInfo
}

// SetKeybinds updates the keybind configuration (called after user edits keybinds).
func (m *Model) SetKeybinds(kb config.Keybinds) { m.keybinds = kb }

// SetSort records the active sort column and direction for header display.
// Called by app.go after it applies the ORDER BY to the re-run query.
func (m *Model) SetSort(col string, asc bool) {
	m.sortCol = col
	m.sortAsc = asc
}

// ColumnAtCursor returns the column name at the current cursor position,
// or an empty string if the cursor is out of range.
func (m *Model) ColumnAtCursor() string {
	if m.cursorCol >= 0 && m.cursorCol < len(m.columns) {
		return m.columns[m.cursorCol]
	}
	return ""
}

// SetCursor moves the cursor to the given row/column and scrolls to show it.
// Values are clamped to the valid range so no bounds check is needed by callers.
func (m *Model) SetCursor(row, col int) {
	rows := m.paged.CurrentRows()
	if len(rows) == 0 {
		return
	}
	if row >= len(rows) {
		row = len(rows) - 1
	}
	if row < 0 {
		row = 0
	}
	if col >= len(m.columns) {
		col = len(m.columns) - 1
	}
	if col < 0 {
		col = 0
	}
	m.cursorRow = row
	m.cursorCol = col
	m.scrollToShowCursor()
}

// Cursor returns the current cursor position as (row, col).
func (m *Model) Cursor() (int, int) { return m.cursorRow, m.cursorCol }

// computeColWidths calculates the VISUAL display width for each column from
// the current page data. Visual width (go-runewidth) is used rather than rune
// count because CJK characters, combining characters, and mojibake sequences
// all occupy different numbers of terminal columns than their rune counts.
func (m *Model) computeColWidths() {
	if len(m.columns) == 0 {
		m.colWidths = nil
		return
	}
	m.colWidths = make([]int, len(m.columns))
	for i, col := range m.columns {
		m.colWidths[i] = runewidth.StringWidth(col)
	}
	for _, row := range m.paged.CurrentRows() {
		for i, cell := range row {
			if i >= len(m.colWidths) {
				break
			}
			// Sanitize before measuring so newlines don't skew the width calculation.
			if w := runewidth.StringWidth(sanitizeCell(cell)); w > m.colWidths[i] {
				m.colWidths[i] = w
			}
		}
	}
	for i := range m.colWidths {
		if m.colWidths[i] > maxCellWidth {
			m.colWidths[i] = maxCellWidth
		}
		if m.colWidths[i] < 3 {
			m.colWidths[i] = 3
		}
	}
}

// visibleColRange returns the [start, end) column indices that fit in the
// available panel width starting from colOffset.
// When the panel is very narrow, at least one column is always shown but its
// display width is capped so the cell never exceeds contentW.
func (m *Model) visibleColRange() (start, end int) {
	if len(m.colWidths) == 0 {
		return 0, 0
	}
	start = m.colOffset
	contentW := m.width - 4
	if contentW < 4 {
		contentW = 4
	}
	used := 0
	for end = start; end < len(m.columns); end++ {
		w := m.colWidths[end]
		// Cap the column at contentW-2 so padding fits even in a 1-column view.
		if w > contentW-2 {
			w = contentW - 2
		}
		cellW := w + 3 // content + 2 padding + 1 separator
		if used+cellW > contentW && end > start {
			break
		}
		used += cellW
	}
	if end == start && end < len(m.columns) {
		end = start + 1
	}
	return start, end
}

// effectiveColWidth returns the display width for column ci, capped to the
// available content width so a single column never overflows a narrow panel.
func (m *Model) effectiveColWidth(ci int) int {
	contentW := m.width - 4
	if contentW < 4 {
		contentW = 4
	}
	w := m.colWidths[ci]
	if w > contentW-2 {
		w = contentW - 2
	}
	return w
}

// stretchedColWidths returns per-column display widths for [colStart, colEnd)
// stretched so that the total grid width exactly fills contentW.
// Each column first gets its effectiveColWidth. If there is leftover space,
// it is distributed proportionally (wider columns absorb more extra space)
// so no blank area is left on the right side of the panel.
func (m *Model) stretchedColWidths(colStart, colEnd int) []int {
	n := colEnd - colStart
	if n <= 0 {
		return nil
	}

	contentW := m.width - 4
	if contentW < n {
		contentW = n
	}

	// Base widths capped to the panel.
	widths := make([]int, n)
	total := 0
	for i := range widths {
		widths[i] = m.effectiveColWidth(colStart + i)
		// Each cell = widths[i]+2 padding, plus 1 separator (except last).
		total += widths[i] + 2
		if i < n-1 {
			total++ // separator
		}
	}

	remaining := contentW - total
	if remaining <= 0 {
		return widths
	}

	// Distribute remaining space proportionally among all visible columns.
	// Columns with more natural width receive proportionally more extra space.
	baseSum := 0
	for _, w := range widths {
		baseSum += w
	}
	if baseSum == 0 {
		// All columns have zero width — give extra to the last one.
		widths[n-1] += remaining
		return widths
	}

	given := 0
	for i := range widths {
		if i == n-1 {
			// Last column absorbs any rounding remainder.
			widths[i] += remaining - given
		} else {
			extra := remaining * widths[i] / baseSum
			widths[i] += extra
			given += extra
		}
	}
	return widths
}

// visibleRowCount returns how many data rows fit in the panel height.
func (m *Model) visibleRowCount() int {
	n := m.height - gridOverhead
	if n < 1 {
		n = 1
	}
	return n
}

// scrollToShowCursor adjusts rowOffset and colOffset so the cursor stays
// within the visible area after any cursor movement or resize.
func (m *Model) scrollToShowCursor() {
	rows := m.paged.CurrentRows()
	numRows := len(rows)
	numCols := len(m.columns)
	if numRows == 0 || numCols == 0 {
		return
	}

	// Clamp cursor.
	if m.cursorRow >= numRows {
		m.cursorRow = numRows - 1
	}
	if m.cursorRow < 0 {
		m.cursorRow = 0
	}
	if m.cursorCol >= numCols {
		m.cursorCol = numCols - 1
	}
	if m.cursorCol < 0 {
		m.cursorCol = 0
	}

	visH := m.visibleRowCount()
	if m.cursorRow < m.rowOffset {
		m.rowOffset = m.cursorRow
	}
	if m.cursorRow >= m.rowOffset+visH {
		m.rowOffset = m.cursorRow - visH + 1
	}

	// Keep cursor column inside the visible range [colOffset, colEnd).
	if m.cursorCol < m.colOffset {
		m.colOffset = m.cursorCol
	} else {
		_, colEnd := m.visibleColRange()
		for m.cursorCol >= colEnd && m.colOffset < numCols-1 {
			m.colOffset++
			_, colEnd = m.visibleColRange()
		}
	}
}

// Update handles keyboard events and spinner ticks.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmds []tea.Cmd

	if m.querying {
		var spCmd tea.Cmd
		m.spinner, spCmd = m.spinner.Update(msg)
		cmds = append(cmds, spCmd)
	}

	if !m.focused {
		return m, tea.Batch(cmds...)
	}

	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "up", "k":
			m.cursorRow--
			m.scrollToShowCursor()

		case "down", "j":
			m.cursorRow++
			m.scrollToShowCursor()

		case "left", "h":
			oldOffset := m.colOffset
			m.cursorCol--
			m.scrollToShowCursor()
			if m.colOffset != oldOffset {
				cmds = append(cmds, func() tea.Msg { return tea.ClearScreen() })
			}

		case "right", "l":
			oldOffset := m.colOffset
			m.cursorCol++
			m.scrollToShowCursor()
			if m.colOffset != oldOffset {
				cmds = append(cmds, func() tea.Msg { return tea.ClearScreen() })
			}

		case m.keybinds.CopyCell:
			rows := m.paged.CurrentRows()
			if m.cursorRow < len(rows) && m.cursorCol < len(rows[m.cursorRow]) {
				m.lastCopy = &CopyAction{Value: rows[m.cursorRow][m.cursorCol]}
			}

		case "e":
			rows := m.paged.CurrentRows()
			if m.cursorRow < len(rows) && m.cursorCol < len(m.columns) && m.cursorCol < len(rows[m.cursorRow]) {
				rowCopy := make([]string, len(rows[m.cursorRow]))
				copy(rowCopy, rows[m.cursorRow])
				colsCopy := make([]string, len(m.columns))
				copy(colsCopy, m.columns)
				m.lastEditCell = &EditCellAction{
					Column:  m.columns[m.cursorCol],
					Value:   rows[m.cursorRow][m.cursorCol],
					Row:     m.cursorRow,
					Col:     m.cursorCol,
					Columns: colsCopy,
					RowVals: rowCopy,
				}
			}

		case "s":
			if m.cursorCol >= 0 && m.cursorCol < len(m.columns) {
				col := m.columns[m.cursorCol]
				// Toggle direction when sorting the same column again.
				asc := true
				if m.sortCol == col {
					asc = !m.sortAsc
				}
				m.lastSort = &SortAction{Column: col, Asc: asc}
			}

		case "/":
			if m.cursorCol >= 0 && m.cursorCol < len(m.columns) {
				m.lastSearch = &SearchAction{Column: m.columns[m.cursorCol]}
			}

		case m.keybinds.FollowFK:
			rows := m.paged.CurrentRows()
			if m.cursorRow < len(rows) && m.cursorCol < len(m.columns) && m.cursorCol < len(rows[m.cursorRow]) {
				colName := m.columns[m.cursorCol]
				if fk, ok := m.fkInfo[colName]; ok && fk != nil {
					val := rows[m.cursorRow][m.cursorCol]
					if val != "" && val != "NULL" {
						m.lastFKNavigate = &FKNavigateAction{Value: val, FK: *fk}
					}
				}
			}

		case m.keybinds.NextPage:
			m.NextPage()

		case m.keybinds.PrevPage:
			m.PrevPage()
		}
	}

	return m, tea.Batch(cmds...)
}

// View renders the results panel.
func (m Model) View() string {
	if m.width < 4 || m.height < 3 {
		return ""
	}

	contentW := m.width - 4
	if contentW < 1 {
		contentW = 1
	}

	var body string
	switch {
	case m.querying:
		body = "\n  " + m.spinner.View() + "  Executing query…" +
			"    " + m.styles.Muted.Render("["+m.keybinds.CancelQuery+"] Cancel")
	case m.err != nil:
		body = m.styles.Error.Width(contentW).Render(
			"  " + styles.IconError + " " + m.err.Error(),
		)
	case len(m.columns) == 0:
		body = m.styles.Muted.Render("\n  Run a query to see results here.")
	default:
		body = m.renderGrid()
	}

	header := m.renderHeader()
	footer := m.renderFooter(contentW)

	inner := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)

	borderStyle := m.styles.PanelUnfocused
	if m.focused {
		borderStyle = m.styles.PanelFocused
	}
	if m.err != nil {
		borderStyle = m.styles.PanelError
	}

	return borderStyle.
		Width(m.width - 2).
		Height(m.height - 2).
		Render(inner)
}

func (m Model) renderHeader() string {
	label := styles.IconResults + "  Results"
	if m.querying {
		label = styles.IconResults + "  Results  " + m.spinner.View()
	}
	return m.styles.Title.Render(label)
}

// renderGrid draws the table as a custom grid with a free-moving cell cursor.
func (m Model) renderGrid() string {
	rows := m.paged.CurrentRows()
	if len(rows) == 0 || len(m.columns) == 0 {
		return m.styles.Muted.Render("\n  No rows.")
	}

	colStart, colEnd := m.visibleColRange()
	visH := m.visibleRowCount()
	rowStart := m.rowOffset
	rowEnd := rowStart + visH
	if rowEnd > len(rows) {
		rowEnd = len(rows)
	}

	// Compute per-column display widths, then stretch them to fill contentW
	// so no blank space is left on the right when columns don't fill the panel.
	displayW := m.stretchedColWidths(colStart, colEnd)

	sep := m.styles.Muted.Render("│")

	var sb strings.Builder

	for i := colStart; i < colEnd; i++ {
		w := displayW[i-colStart]
		label := m.columns[i]
		if m.sortCol == label {
			if m.sortAsc {
				label += " ↑"
			} else {
				label += " ↓"
			}
		}
		text := visualTruncate(label, w)
		cell := padRight(" "+text, w+2)
		sb.WriteString(m.styles.TableHeader.Render(cell))
		if i < colEnd-1 {
			sb.WriteString(sep)
		}
	}
	sb.WriteByte('\n')

	for i := colStart; i < colEnd; i++ {
		w := displayW[i-colStart]
		sb.WriteString(m.styles.Muted.Render(strings.Repeat("─", w+2)))
		if i < colEnd-1 {
			sb.WriteString(m.styles.Muted.Render("┼"))
		}
	}
	sb.WriteByte('\n')

	for ri := rowStart; ri < rowEnd; ri++ {
		row := rows[ri]
		for ci := colStart; ci < colEnd; ci++ {
			w := displayW[ci-colStart]
			var raw string
			if ci < len(row) {
				raw = row[ci]
			}

			colName := ""
			if ci < len(m.columns) {
				colName = m.columns[ci]
			}
			isFK := m.fkInfo != nil && m.fkInfo[colName] != nil && raw != "" && raw != "NULL"

			const fkPrefixW = 2
			var cell string
			if isFK {
				contentW := w - fkPrefixW
				if contentW < 1 {
					contentW = 1
				}
				display := styles.IconFK + " " + visualTruncate(sanitizeCell(raw), contentW)
				cell = padRight(display, w+2)
			} else {
				display := visualTruncate(sanitizeCell(raw), w)
				cell = padRight(" "+display, w+2)
			}

			if ri == m.cursorRow && ci == m.cursorCol {
				sb.WriteString(m.styles.TableSelected.Bold(true).Render(cell))
			} else {
				sb.WriteString(m.colorCell(raw, cell))
			}
			if ci < colEnd-1 {
				sb.WriteString(sep)
			}
		}
		if ri < rowEnd-1 {
			sb.WriteByte('\n')
		}
	}

	return sb.String()
}

// colorCell applies value-based coloring to a pre-padded cell string.
func (m Model) colorCell(raw, cell string) string {
	if IsNull(raw) {
		return m.styles.Muted.Italic(true).Render(cell)
	}
	if IsBoolTrue(raw) {
		return m.styles.Success.Render(cell)
	}
	if IsBoolFalse(raw) {
		return m.styles.Error.Render(cell)
	}
	if IsNegative(raw) {
		return m.styles.Warning.Render(cell)
	}
	return cell
}

func (m Model) renderFooter(contentW int) string {
	if m.rowCount == 0 && !m.querying && m.err == nil {
		return ""
	}

	colStart, colEnd := m.visibleColRange()
	numCols := len(m.columns)
	rows := m.paged.CurrentRows()

	var parts []string

	if m.rowCount > 0 {
		curRow := m.cursorRow + 1
		if len(rows) == 0 {
			curRow = 0
		}
		parts = append(parts, fmt.Sprintf("row %d/%d", curRow, m.rowCount))
	}

	if numCols > 0 {
		parts = append(parts, fmt.Sprintf("col %d/%d", m.cursorCol+1, numCols))
	}
	if colStart > 0 {
		parts = append(parts, m.styles.Muted.Render(fmt.Sprintf("← %d", colStart)))
	}
	if colEnd < numCols {
		parts = append(parts, m.styles.Muted.Render(fmt.Sprintf("%d →", numCols-colEnd)))
	}

	if m.elapsed > 0 {
		parts = append(parts, styles.IconClock+" "+m.elapsed.Round(time.Millisecond).String())
	}

	if m.paged.TotalPages() > 1 {
		parts = append(parts,
			fmt.Sprintf("p%d/%d", m.paged.Page+1, m.paged.TotalPages()),
			"["+m.keybinds.NextPage+"]",
			"["+m.keybinds.PrevPage+"]",
		)
	}

	if len(parts) == 0 {
		return ""
	}
	return m.styles.Muted.Width(contentW).Render("  " + strings.Join(parts, "  "))
}

// visualTruncate truncates s so that its terminal display width is at most
// maxW columns, appending "…" when cut. Unlike plainTruncate (rune count),
// this uses go-runewidth to handle wide characters (CJK, combining chars,
// mojibake sequences) that occupy more than one terminal column per rune.
// sanitizeCell replaces control characters (newlines, tabs, carriage returns)
// that would break the single-line grid layout with a visible placeholder.
// Database columns such as JSON, serialized logs, or multi-line text fields
// commonly contain embedded newlines that must be collapsed before display.
func sanitizeCell(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\n', '\r', '\u2028', '\u2029':
			b.WriteRune('↩')
		case '\t':
			b.WriteRune('→')
		default:
			if unicode.IsControl(r) {
				b.WriteRune('�')
			} else {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// visualTruncate truncates s so that its terminal display width is at most
// maxW columns, appending "…" when cut. Unlike plainTruncate (rune count),
// this uses go-runewidth to handle wide characters (CJK, combining chars,
// mojibake sequences) that occupy more than one terminal column per rune.
func visualTruncate(s string, maxW int) string {
	if runewidth.StringWidth(s) <= maxW {
		return s
	}
	if maxW <= 1 {
		return "…"
	}
	used := 0
	var out strings.Builder
	for _, r := range s {
		w := runewidth.RuneWidth(r)
		if used+w > maxW-1 {
			break
		}
		out.WriteRune(r)
		used += w
	}
	out.WriteRune('…')
	return out.String()
}

// padRight pads s with spaces on the right so the total visual width equals
// targetW. Required because fmt.Sprintf uses rune count, not terminal columns.
func padRight(s string, targetW int) string {
	w := runewidth.StringWidth(s)
	if w >= targetW {
		return s
	}
	return s + strings.Repeat(" ", targetW-w)
}

// plainTruncate shortens a plain (non-ANSI) string to maxW runes.
// Use visualTruncate when the string may contain wide Unicode characters.
func plainTruncate(s string, maxW int) string {
	runes := []rune(s)
	if len(runes) <= maxW {
		return s
	}
	if maxW <= 1 {
		return "…"
	}
	return string(runes[:maxW-1]) + "…"
}
