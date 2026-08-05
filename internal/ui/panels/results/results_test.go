package results

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gabrielfloresousion/db-term/internal/config"
	"github.com/gabrielfloresousion/db-term/internal/types"
	"github.com/gabrielfloresousion/db-term/internal/ui/styles"
)

func testModel() Model {
	cfg := config.DefaultConfig()
	s := styles.ToStyles(styles.Resolve(cfg.Settings.Theme, cfg.Theme.Custom))
	return New(s, cfg.Keybinds)
}

func pressKey(key string) tea.Msg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

func TestTruncateCell_ShortStringUnchanged(t *testing.T) {
	s := "hello"
	got := TruncateCell(s)
	if got != s {
		t.Errorf("TruncateCell(%q) = %q, want unchanged", s, got)
	}
}

func TestTruncateCell_LongStringTruncated(t *testing.T) {
	// Build a string longer than maxCellWidth.
	long := make([]byte, maxCellWidth+10)
	for i := range long {
		long[i] = 'a'
	}
	got := TruncateCell(string(long))
	if len([]rune(got)) > maxCellWidth {
		t.Errorf("TruncateCell: result length %d exceeds max %d", len([]rune(got)), maxCellWidth)
	}
	if got[len(got)-3:] != "…" {
		t.Errorf("TruncateCell: truncated string should end with '…', got %q", got[len(got)-3:])
	}
}

func TestTruncateCell_ExactMaxLength(t *testing.T) {
	exact := make([]byte, maxCellWidth)
	for i := range exact {
		exact[i] = 'x'
	}
	got := TruncateCell(string(exact))
	if got != string(exact) {
		t.Errorf("TruncateCell: string of exact max length should be unchanged")
	}
}

func TestIsNull(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"NULL", true},
		{"null", true},
		{"Null", true},
		{"", false},
		{"hello", false},
		{"0", false},
	}
	for _, c := range cases {
		if IsNull(c.s) != c.want {
			t.Errorf("IsNull(%q) = %v, want %v", c.s, !c.want, c.want)
		}
	}
}

func TestIsBoolTrue(t *testing.T) {
	trues := []string{"true", "TRUE", "1", "t", "yes"}
	for _, s := range trues {
		if !IsBoolTrue(s) {
			t.Errorf("IsBoolTrue(%q) = false, want true", s)
		}
	}
	falses := []string{"false", "0", "no", "hello", ""}
	for _, s := range falses {
		if IsBoolTrue(s) {
			t.Errorf("IsBoolTrue(%q) = true, want false", s)
		}
	}
}

func TestIsBoolFalse(t *testing.T) {
	falses := []string{"false", "FALSE", "0", "f", "no"}
	for _, s := range falses {
		if !IsBoolFalse(s) {
			t.Errorf("IsBoolFalse(%q) = false, want true", s)
		}
	}
	trues := []string{"true", "1", "yes", "hello", ""}
	for _, s := range trues {
		if IsBoolFalse(s) {
			t.Errorf("IsBoolFalse(%q) = true, want false", s)
		}
	}
}

func TestIsNegative(t *testing.T) {
	if !IsNegative("-42") {
		t.Error("IsNegative(-42) = false, want true")
	}
	if IsNegative("42") {
		t.Error("IsNegative(42) = true, want false")
	}
	if IsNegative("") {
		t.Error("IsNegative('') = true, want false")
	}
}

func makeRows(n int) [][]string {
	rows := make([][]string, n)
	for i := range rows {
		rows[i] = []string{string(rune('A' + i%26))}
	}
	return rows
}

func TestPagedResult_TotalPages(t *testing.T) {
	cases := []struct {
		rows     int
		pageSize int
		want     int
	}{
		{0, 500, 1},
		{500, 500, 1},
		{501, 500, 2},
		{1000, 500, 2},
		{1001, 500, 3},
	}
	for _, c := range cases {
		p := NewPagedResult(makeRows(c.rows))
		p.PageSize = c.pageSize
		got := p.TotalPages()
		if got != c.want {
			t.Errorf("TotalPages(rows=%d, size=%d) = %d, want %d",
				c.rows, c.pageSize, got, c.want)
		}
	}
}

func TestPagedResult_CurrentRows_FirstPage(t *testing.T) {
	p := NewPagedResult(makeRows(600))
	p.PageSize = 500
	rows := p.CurrentRows()
	if len(rows) != 500 {
		t.Errorf("CurrentRows page 0: got %d rows, want 500", len(rows))
	}
}

func TestPagedResult_CurrentRows_LastPage(t *testing.T) {
	p := NewPagedResult(makeRows(600))
	p.PageSize = 500
	p.NextPage()
	rows := p.CurrentRows()
	if len(rows) != 100 {
		t.Errorf("CurrentRows page 1: got %d rows, want 100", len(rows))
	}
}

func TestPagedResult_NextPage_NoopOnLastPage(t *testing.T) {
	p := NewPagedResult(makeRows(10))
	p.PageSize = 500
	p.NextPage() // already on last page
	if p.Page != 0 {
		t.Errorf("NextPage on last page: Page = %d, want 0", p.Page)
	}
}

func TestPagedResult_PrevPage_NoopOnFirstPage(t *testing.T) {
	p := NewPagedResult(makeRows(1000))
	p.PageSize = 500
	p.PrevPage() // already on first page
	if p.Page != 0 {
		t.Errorf("PrevPage on first page: Page = %d, want 0", p.Page)
	}
}

func TestPagedResult_NextThenPrev(t *testing.T) {
	p := NewPagedResult(makeRows(1001))
	p.PageSize = 500
	p.NextPage()
	if p.Page != 1 {
		t.Errorf("after NextPage: Page = %d, want 1", p.Page)
	}
	p.PrevPage()
	if p.Page != 0 {
		t.Errorf("after PrevPage: Page = %d, want 0", p.Page)
	}
}

func TestPagedResult_EmptyRows(t *testing.T) {
	p := NewPagedResult(nil)
	rows := p.CurrentRows()
	if rows != nil {
		t.Errorf("CurrentRows on empty: expected nil, got %v", rows)
	}
	if p.TotalPages() != 1 {
		t.Errorf("TotalPages on empty: expected 1, got %d", p.TotalPages())
	}
}

func TestEditCell_EKeyProducesAction(t *testing.T) {
	// Regression: pressing 'e' on a focused cell must set lastEditCell and
	// TakeEditCell() must return it with the correct column name and value.
	m := testModel()
	m.SetResult([]string{"id", "name"}, [][]string{{"1", "Alice"}, {"2", "Bob"}}, 0, nil)
	m.SetSize(80, 24)
	m.Focus()

	m, _ = m.Update(pressKey("e"))
	action := m.TakeEditCell()
	if action == nil {
		t.Fatal("TakeEditCell() returned nil after pressing 'e'")
	}
	if action.Column != "id" {
		t.Errorf("Column = %q, want %q", action.Column, "id")
	}
	if action.Value != "1" {
		t.Errorf("Value = %q, want %q", action.Value, "1")
	}
}

func TestEditCell_TakeEditCellClearsAction(t *testing.T) {
	// TakeEditCell() must be idempotent: second call returns nil.
	m := testModel()
	m.SetResult([]string{"col"}, [][]string{{"val"}}, 0, nil)
	m.SetSize(80, 24)
	m.Focus()

	m, _ = m.Update(pressKey("e"))
	_ = m.TakeEditCell()
	if got := m.TakeEditCell(); got != nil {
		t.Errorf("second TakeEditCell() = %v, want nil", got)
	}
}

func TestEditCell_NoActionWhenUnfocused(t *testing.T) {
	// 'e' must not produce an EditCellAction when the panel is not focused.
	m := testModel()
	m.SetResult([]string{"col"}, [][]string{{"val"}}, 0, nil)
	m.SetSize(80, 24)

	m, _ = m.Update(pressKey("e"))
	if got := m.TakeEditCell(); got != nil {
		t.Errorf("got action on unfocused panel, want nil")
	}
}

func TestEditCell_NoActionOnEmptyResults(t *testing.T) {
	// 'e' on an empty result set must not panic and must not set a pending action.
	m := testModel()
	m.SetSize(80, 24)
	m.Focus()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("pressing 'e' on empty results panicked: %v", r)
		}
	}()
	m, _ = m.Update(pressKey("e"))
	if got := m.TakeEditCell(); got != nil {
		t.Errorf("got action on empty results, want nil")
	}
}

func TestColumnAtCursor_ReturnsColumnName(t *testing.T) {
	m := testModel()
	m.SetResult([]string{"id", "name"}, [][]string{{"1", "Alice"}}, 0, nil)
	m.SetSize(80, 24)

	if got := m.ColumnAtCursor(); got != "id" {
		t.Errorf("ColumnAtCursor() = %q, want %q", got, "id")
	}
}

func TestFollowFK_ProducesAndClearsAction(t *testing.T) {
	m := testModel()
	m.SetResult([]string{"customer_id"}, [][]string{{"42"}}, 0, nil)
	m.SetFKInfo(map[string]*types.ForeignKey{
		"customer_id": {Schema: "public", Table: "customers", Column: "id"},
	})
	m.SetSize(80, 24)
	m.Focus()

	m, _ = m.Update(pressKey(m.keybinds.FollowFK))
	action := m.TakeFKNavigate()
	if action == nil {
		t.Fatal("TakeFKNavigate() returned nil after pressing FollowFK")
	}
	if action.Value != "42" || action.FK.Table != "customers" || action.FK.Column != "id" {
		t.Errorf("TakeFKNavigate() = %+v, want customer 42 by id", action)
	}
	if next := m.TakeFKNavigate(); next != nil {
		t.Errorf("second TakeFKNavigate() = %+v, want nil", next)
	}
}

func TestFollowFK_IgnoresNullAndNonForeignKeyCells(t *testing.T) {
	m := testModel()
	m.SetResult([]string{"customer_id", "name"}, [][]string{{"NULL", "Alice"}}, 0, nil)
	m.SetFKInfo(map[string]*types.ForeignKey{
		"customer_id": {Schema: "public", Table: "customers", Column: "id"},
	})
	m.SetSize(80, 24)
	m.Focus()

	m, _ = m.Update(pressKey(m.keybinds.FollowFK))
	if action := m.TakeFKNavigate(); action != nil {
		t.Errorf("FollowFK on NULL = %+v, want nil", action)
	}
	m.cursorCol = 1
	m, _ = m.Update(pressKey(m.keybinds.FollowFK))
	if action := m.TakeFKNavigate(); action != nil {
		t.Errorf("FollowFK on regular cell = %+v, want nil", action)
	}
}

func TestRenderGrid_ForeignKeyCellIncludesIcon(t *testing.T) {
	m := testModel()
	m.SetResult([]string{"customer_id"}, [][]string{{"42"}}, 0, nil)
	m.SetFKInfo(map[string]*types.ForeignKey{
		"customer_id": {Schema: "public", Table: "customers", Column: "id"},
	})
	m.SetSize(80, 24)

	if grid := m.renderGrid(); !strings.Contains(grid, styles.IconFK) {
		t.Errorf("renderGrid() = %q, want foreign-key icon", grid)
	}
}

func TestView_HorizontalScrollDoesNotExceedPanelSize(t *testing.T) {
	// Regression: panel padding was omitted from width calculations, causing wrapped lines and stale grids during horizontal scroll.
	m := testModel()
	columns := []string{"annex_link_key", "original_file_name", "description", "type_id", "invoice_id"}
	rows := [][]string{
		{
			"https://example.com/a/very/long/path/to/a/file.pdf",
			"SOMOS - BRINDES COOPERATIVA LAR.pdf\nImagem do WhatsApp.jpg",
			"e-mail autorizando utilização de bebidas alcoólicas pelo gerente",
			"0ace473e-f36b-1410-83ba-00736d9db73e",
			"b3e9faa6-dce9-ef11-88f8-0022483634d5",
		},
	}
	m.SetResult(columns, rows, 0, nil)
	m.SetSize(80, 20)
	m.Focus()

	for col := range columns {
		m.SetCursor(0, col)
		view := m.View()
		if width := lipgloss.Width(view); width != 80 {
			t.Errorf("column %d rendered width = %d, want 80", col, width)
		}
		if height := lipgloss.Height(view); height != 20 {
			t.Errorf("column %d rendered height = %d, want 20", col, height)
		}
	}
}

func TestUpdate_HorizontalViewportChangeClearsScreen(t *testing.T) {
	// Regression: old table fragments remained visible after the horizontal viewport changed.
	m := testModel()
	m.SetResult(
		[]string{"first_long_column", "second_long_column"},
		[][]string{{"first long value", "second long value"}},
		0,
		nil,
	)
	m.SetSize(24, 10)
	m.Focus()

	var cmd tea.Cmd
	m, cmd = m.Update(pressKey("right"))
	if m.colOffset == 0 {
		t.Fatal("right key did not change the horizontal viewport")
	}
	if cmd == nil {
		t.Fatal("horizontal viewport change did not request a screen clear")
	}
	if msg := cmd(); msg == nil {
		t.Fatal("horizontal viewport command returned nil")
	}
}

func TestSanitizeCell_RemovesTerminalControlCharacters(t *testing.T) {
	input := "first\x1b[2J\bsecond\u2028third"
	got := sanitizeCell(input)
	if strings.ContainsAny(got, "\x1b\b\u2028") {
		t.Errorf("sanitizeCell(%q) = %q, control character remained", input, got)
	}
}
