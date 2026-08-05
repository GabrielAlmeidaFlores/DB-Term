// Package results implements the query results table panel.
package results

import "strings"

const defaultPageSize = 500

// maxCellWidth is the maximum number of runes displayed per cell.
// Values longer than this are truncated with "…" in the display, but the
// full value is still preserved in AllRows for clipboard copy.
const maxCellWidth = 80

// gridOverhead is the number of non-data rows in the results panel:
// top border(1) + panel title(1) + column names(1) + separator(1) + footer(1) + bottom border(1).
const gridOverhead = 6

// PagedResult holds all query rows and tracks the current page.
type PagedResult struct {
	AllRows  [][]string
	PageSize int
	Page     int // 0-indexed
}

// NewPagedResult creates a PagedResult with the given rows and default page size.
func NewPagedResult(rows [][]string) PagedResult {
	return PagedResult{
		AllRows:  rows,
		PageSize: defaultPageSize,
		Page:     0,
	}
}

// CurrentRows returns the rows visible on the current page.
func (p *PagedResult) CurrentRows() [][]string {
	if len(p.AllRows) == 0 {
		return nil
	}
	start := p.Page * p.PageSize
	end := start + p.PageSize
	if end > len(p.AllRows) {
		end = len(p.AllRows)
	}
	if start >= len(p.AllRows) {
		return nil
	}
	return p.AllRows[start:end]
}

// TotalPages returns the number of pages needed to display all rows.
func (p *PagedResult) TotalPages() int {
	if p.PageSize <= 0 || len(p.AllRows) == 0 {
		return 1
	}
	total := len(p.AllRows) / p.PageSize
	if len(p.AllRows)%p.PageSize != 0 {
		total++
	}
	return total
}

// NextPage advances to the next page. No-op on the last page.
func (p *PagedResult) NextPage() {
	if p.Page < p.TotalPages()-1 {
		p.Page++
	}
}

// PrevPage moves to the previous page. No-op on the first page.
func (p *PagedResult) PrevPage() {
	if p.Page > 0 {
		p.Page--
	}
}

// TruncateCell shortens s to at most maxCellWidth runes, appending "…" if needed.
// The full value is preserved separately for clipboard operations.
func TruncateCell(s string) string {
	runes := []rune(s)
	if len(runes) <= maxCellWidth {
		return s
	}
	return string(runes[:maxCellWidth-1]) + "…"
}

// FormatNull replaces the string "NULL" with a styled placeholder.
// Actual styling is applied in results.go using the active theme.
func IsNull(s string) bool { return strings.EqualFold(s, "null") }

// IsBoolTrue reports whether s looks like a true boolean value.
func IsBoolTrue(s string) bool {
	lower := strings.ToLower(s)
	return lower == "true" || lower == "1" || lower == "t" || lower == "yes"
}

// IsBoolFalse reports whether s looks like a false boolean value.
func IsBoolFalse(s string) bool {
	lower := strings.ToLower(s)
	return lower == "false" || lower == "0" || lower == "f" || lower == "no"
}

// IsNegative reports whether s looks like a negative number.
func IsNegative(s string) bool {
	return len(s) > 0 && s[0] == '-'
}
