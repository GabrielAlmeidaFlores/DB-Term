// Package editor implements the SQL editor panel.
package editor

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gabrielfloresousion/db-term/internal/config"
	"github.com/gabrielfloresousion/db-term/internal/ui/styles"
)

func testStyles() styles.Styles {
	return styles.ToStyles(styles.Resolve("catppuccin-mocha", config.ThemeConfig{}))
}

func TestTokenizeSQL_RoundTrip(t *testing.T) {
	// Regression: joining all tokens must reconstruct the original input exactly.
	inputs := []string{
		"SELECT * FROM users WHERE id = 1",
		"-- comment\nSELECT 'hello''world' FROM t",
		"/* block\n   comment */\nDELETE FROM t",
		"SELECT COUNT(*), SUM(amount) FROM orders GROUP BY status",
		"SELECT id, name FROM t WHERE created_at >= '2024-01-01'",
		"",
	}
	for _, sql := range inputs {
		tokens := tokenizeSQL(sql)
		var sb strings.Builder
		for _, tok := range tokens {
			sb.WriteString(tok.text)
		}
		if got := sb.String(); got != sql {
			t.Errorf("RoundTrip(%q):\n  got  %q\n  want %q", sql, got, sql)
		}
	}
}

func TestTokenizeSQL_KeywordsClassified(t *testing.T) {
	tokens := tokenizeSQL("SELECT id FROM users WHERE id = 1")
	kinds := map[string]tokenKind{}
	for _, tok := range tokens {
		kinds[tok.text] = tok.kind
	}
	cases := []struct {
		word string
		want tokenKind
	}{
		{"SELECT", tkKeyword},
		{"FROM", tkKeyword},
		{"WHERE", tkKeyword},
		{"=", tkOperator},
		{"1", tkNumber},
		{"id", tkPlain},
		{"users", tkPlain},
	}
	for _, tc := range cases {
		if got := kinds[tc.word]; got != tc.want {
			t.Errorf("token %q: kind=%d, want %d", tc.word, got, tc.want)
		}
	}
}

func TestTokenizeSQL_DMLClassified(t *testing.T) {
	tokens := tokenizeSQL("INSERT INTO t VALUES (1)")
	kinds := map[string]tokenKind{}
	for _, tok := range tokens {
		kinds[tok.text] = tok.kind
	}
	if kinds["INSERT"] != tkDML {
		t.Errorf("INSERT should be tkDML, got %d", kinds["INSERT"])
	}
	if kinds["INTO"] != tkKeyword {
		t.Errorf("INTO should be tkKeyword, got %d", kinds["INTO"])
	}
}

func TestTokenizeSQL_StringLiteral(t *testing.T) {
	tokens := tokenizeSQL("WHERE name = 'O''Brien'")
	for _, tok := range tokens {
		if tok.text == "'O''Brien'" && tok.kind != tkString {
			t.Errorf("string literal classified as %d, want tkString", tok.kind)
		}
	}
}

func TestTokenizeSQL_LineComment(t *testing.T) {
	tokens := tokenizeSQL("-- this is a comment\nSELECT 1")
	if tokens[0].kind != tkComment {
		t.Errorf("first token should be tkComment, got %d", tokens[0].kind)
	}
	if tokens[0].text != "-- this is a comment" {
		t.Errorf("comment text = %q, want %q", tokens[0].text, "-- this is a comment")
	}
}

func TestTokenizeSQL_BlockComment(t *testing.T) {
	tokens := tokenizeSQL("/* block */SELECT 1")
	if tokens[0].kind != tkComment {
		t.Errorf("first token should be tkComment, got %d", tokens[0].kind)
	}
	if tokens[0].text != "/* block */" {
		t.Errorf("block comment text = %q, want %q", tokens[0].text, "/* block */")
	}
}

func TestTokenizeSQL_FunctionClassified(t *testing.T) {
	tokens := tokenizeSQL("SELECT COUNT(*), MAX(price) FROM t")
	kinds := map[string]tokenKind{}
	for _, tok := range tokens {
		kinds[tok.text] = tok.kind
	}
	if kinds["COUNT"] != tkFunction {
		t.Errorf("COUNT should be tkFunction, got %d", kinds["COUNT"])
	}
	if kinds["MAX"] != tkFunction {
		t.Errorf("MAX should be tkFunction, got %d", kinds["MAX"])
	}
}

func TestTokenizeSQL_NullTrueFalse(t *testing.T) {
	tokens := tokenizeSQL("WHERE x IS NULL OR y = TRUE OR z = FALSE")
	kinds := map[string]tokenKind{}
	for _, tok := range tokens {
		kinds[tok.text] = tok.kind
	}
	for _, word := range []string{"NULL", "TRUE", "FALSE"} {
		if kinds[word] != tkOperator {
			t.Errorf("%s should be tkOperator, got %d", word, kinds[word])
		}
	}
}

func TestHighlightSQL_EmptyInput(t *testing.T) {
	s := testStyles()
	if got := HighlightSQL("", s); got != "" {
		t.Errorf("HighlightSQL(\"\") = %q, want empty", got)
	}
}

func TestHighlightSQL_ContainsOriginalWords(t *testing.T) {
	// The styled output should still contain the original word text (ANSI may wrap it).
	s := testStyles()
	sql := "SELECT id FROM users"
	out := HighlightSQL(sql, s)
	for _, word := range []string{"SELECT", "id", "FROM", "users"} {
		if !strings.Contains(out, word) {
			t.Errorf("HighlightSQL output missing word %q", word)
		}
	}
}

func newViEditor() Model {
	s := testStyles()
	kb := config.DefaultConfig().Keybinds
	settings := config.Settings{ViMode: true}
	m := New(s, kb, settings)
	m.focused = true
	m.activeConn = "test"
	m.textarea.Focus()
	m.mode = modeNormal
	return m
}

func viKey(key string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key), Alt: false}
}

func TestViMode_InsertModeOnI(t *testing.T) {
	// Regression: pressing 'i' in Normal mode must switch to Insert mode.
	m := newViEditor()
	if m.mode != modeNormal {
		t.Fatal("expected Normal mode initially")
	}
	m, _ = m.Update(viKey("i"))
	if m.mode != modeInsert {
		t.Errorf("mode = %v after 'i', want modeInsert", m.mode)
	}
}

func TestViMode_EscReturnsToNormal(t *testing.T) {
	// Regression: ESC must return to Normal mode from Insert.
	m := newViEditor()
	m.mode = modeInsert
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeNormal {
		t.Errorf("mode = %v after ESC, want modeNormal", m.mode)
	}
}

func TestViMode_DeleteLine(t *testing.T) {
	// Regression: dd must remove the current line and store it in yankBuffer.
	// SetValue places the cursor at the end, so navigate to line 0 via gg first.
	m := newViEditor()
	m.textarea.SetValue("line1\nline2\nline3")
	m, _ = m.Update(viKey("g"))
	m, _ = m.Update(viKey("g"))
	m, _ = m.Update(viKey("d"))
	m, _ = m.Update(viKey("d"))
	lines := strings.Split(m.textarea.Value(), "\n")
	if len(lines) != 2 {
		t.Errorf("after dd: got %d lines, want 2; value=%q", len(lines), m.textarea.Value())
	}
	if m.yankBuffer != "line1" {
		t.Errorf("yankBuffer = %q, want %q", m.yankBuffer, "line1")
	}
}

func TestViMode_YankAndPaste(t *testing.T) {
	// Regression: yy must store the current line; p must insert it below.
	// SetValue places cursor at end — navigate to line 0 via gg first.
	m := newViEditor()
	m.textarea.SetValue("alpha\nbeta")
	m, _ = m.Update(viKey("g"))
	m, _ = m.Update(viKey("g"))
	m, _ = m.Update(viKey("y"))
	m, _ = m.Update(viKey("y"))
	if m.yankBuffer != "alpha" {
		t.Fatalf("yankBuffer = %q after yy, want %q", m.yankBuffer, "alpha")
	}
	m, _ = m.Update(viKey("p"))
	lines := strings.Split(m.textarea.Value(), "\n")
	if len(lines) != 3 {
		t.Errorf("after p: got %d lines, want 3; value=%q", len(lines), m.textarea.Value())
	}
}

func TestViMode_PendingKeyCleared(t *testing.T) {
	// Regression: an unrecognised second key after 'd' must clear pendingKey.
	m := newViEditor()
	m.textarea.SetValue("a\nb")
	m, _ = m.Update(viKey("d"))
	if m.pendingKey != "d" {
		t.Fatalf("pendingKey = %q after 'd', want %q", m.pendingKey, "d")
	}
	m, _ = m.Update(viKey("k"))
	if m.pendingKey != "" {
		t.Errorf("pendingKey = %q after unrecognised combo, want empty", m.pendingKey)
	}
}

func TestViMode_DisabledWhenViModeOff(t *testing.T) {
	// When vi_mode = false, all keys must pass through to the textarea as normal.
	s := testStyles()
	kb := config.DefaultConfig().Keybinds
	m := New(s, kb, config.Settings{ViMode: false})
	m.focused = true
	m.activeConn = "test"
	m.textarea.Focus()

	if !m.inInsertMode() {
		t.Error("inInsertMode() should be true when vi_mode is off")
	}
}
