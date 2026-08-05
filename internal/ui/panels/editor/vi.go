package editor

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type editorMode int

const (
	modeInsert editorMode = iota
	modeNormal
)

func handleNormalMode(m Model, msg tea.KeyMsg) (Model, tea.Cmd) {
	var cmds []tea.Cmd
	key := msg.String()

	if m.pendingKey != "" {
		combo := m.pendingKey + key
		m.pendingKey = ""
		var c tea.Cmd
		switch combo {
		case "dd":
			m, c = viDeleteLine(m)
		case "yy":
			m, c = viYankLine(m)
		case "gg":
			m, c = viMoveToStart(m)
		}
		cmds = append(cmds, c)
		return m, tea.Batch(cmds...)
	}

	var c tea.Cmd
	switch key {
	case "i":
		m.mode = modeInsert
	case "a":
		m.mode = modeInsert
		m.textarea, c = m.textarea.Update(tea.KeyMsg{Type: tea.KeyRight})
		cmds = append(cmds, c)
	case "A":
		m.mode = modeInsert
		m.textarea, c = m.textarea.Update(tea.KeyMsg{Type: tea.KeyEnd})
		cmds = append(cmds, c)
	case "I":
		m.mode = modeInsert
		m.textarea, c = m.textarea.Update(tea.KeyMsg{Type: tea.KeyHome})
		cmds = append(cmds, c)
	case "o":
		m.mode = modeInsert
		m.textarea, c = m.textarea.Update(tea.KeyMsg{Type: tea.KeyEnd})
		cmds = append(cmds, c)
		m.textarea, c = m.textarea.Update(tea.KeyMsg{Type: tea.KeyEnter})
		cmds = append(cmds, c)
	case "O":
		m.mode = modeInsert
		m.textarea, c = m.textarea.Update(tea.KeyMsg{Type: tea.KeyHome})
		cmds = append(cmds, c)
		m.textarea, c = m.textarea.Update(tea.KeyMsg{Type: tea.KeyEnter})
		cmds = append(cmds, c)
		m.textarea, c = m.textarea.Update(tea.KeyMsg{Type: tea.KeyUp})
		cmds = append(cmds, c)
	case "esc":
		m.pendingKey = ""
	case "h", "left":
		m.textarea, c = m.textarea.Update(tea.KeyMsg{Type: tea.KeyLeft})
		cmds = append(cmds, c)
	case "j", "down":
		m.textarea, c = m.textarea.Update(tea.KeyMsg{Type: tea.KeyDown})
		cmds = append(cmds, c)
	case "k", "up":
		m.textarea, c = m.textarea.Update(tea.KeyMsg{Type: tea.KeyUp})
		cmds = append(cmds, c)
	case "l", "right":
		m.textarea, c = m.textarea.Update(tea.KeyMsg{Type: tea.KeyRight})
		cmds = append(cmds, c)
	case "0":
		m.textarea, c = m.textarea.Update(tea.KeyMsg{Type: tea.KeyHome})
		cmds = append(cmds, c)
	case "$":
		m.textarea, c = m.textarea.Update(tea.KeyMsg{Type: tea.KeyEnd})
		cmds = append(cmds, c)
	case "G":
		m, c = viMoveToEnd(m)
		cmds = append(cmds, c)
	case "w":
		m, c = viWordForward(m)
		cmds = append(cmds, c)
	case "b":
		m, c = viWordBackward(m)
		cmds = append(cmds, c)
	case "x":
		m.textarea, c = m.textarea.Update(tea.KeyMsg{Type: tea.KeyDelete})
		cmds = append(cmds, c)
	case "p":
		m, c = viPasteAfter(m)
		cmds = append(cmds, c)
	case "P":
		m, c = viPasteBefore(m)
		cmds = append(cmds, c)
	case "u":
		m.textarea, c = m.textarea.Update(tea.KeyMsg{Type: tea.KeyCtrlZ})
		cmds = append(cmds, c)
	case "ctrl+r":
		m.textarea, c = m.textarea.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
		cmds = append(cmds, c)
	case "d", "y", "g":
		m.pendingKey = key
	}

	return m, tea.Batch(cmds...)
}

func viMoveToStart(m Model) (Model, tea.Cmd) {
	var cmds []tea.Cmd
	line := m.textarea.Line()
	for i := 0; i < line; i++ {
		var c tea.Cmd
		m.textarea, c = m.textarea.Update(tea.KeyMsg{Type: tea.KeyUp})
		cmds = append(cmds, c)
	}
	var c tea.Cmd
	m.textarea, c = m.textarea.Update(tea.KeyMsg{Type: tea.KeyHome})
	cmds = append(cmds, c)
	return m, tea.Batch(cmds...)
}

func viMoveToEnd(m Model) (Model, tea.Cmd) {
	var cmds []tea.Cmd
	lines := strings.Split(m.textarea.Value(), "\n")
	cur := m.textarea.Line()
	remaining := len(lines) - 1 - cur
	for i := 0; i < remaining; i++ {
		var c tea.Cmd
		m.textarea, c = m.textarea.Update(tea.KeyMsg{Type: tea.KeyDown})
		cmds = append(cmds, c)
	}
	var c tea.Cmd
	m.textarea, c = m.textarea.Update(tea.KeyMsg{Type: tea.KeyEnd})
	cmds = append(cmds, c)
	return m, tea.Batch(cmds...)
}

func viWordForward(m Model) (Model, tea.Cmd) {
	var cmds []tea.Cmd
	val := m.textarea.Value()
	lines := strings.Split(val, "\n")
	lineNum := m.textarea.Line()
	if lineNum >= len(lines) {
		return m, nil
	}
	info := m.textarea.LineInfo()
	col := info.CharOffset
	line := []rune(lines[lineNum])
	for col < len(line) && !isSpace(line[col]) {
		col++
		var c tea.Cmd
		m.textarea, c = m.textarea.Update(tea.KeyMsg{Type: tea.KeyRight})
		cmds = append(cmds, c)
	}
	for col < len(line) && isSpace(line[col]) {
		col++
		var c tea.Cmd
		m.textarea, c = m.textarea.Update(tea.KeyMsg{Type: tea.KeyRight})
		cmds = append(cmds, c)
	}
	return m, tea.Batch(cmds...)
}

func viWordBackward(m Model) (Model, tea.Cmd) {
	var cmds []tea.Cmd
	val := m.textarea.Value()
	lines := strings.Split(val, "\n")
	lineNum := m.textarea.Line()
	if lineNum >= len(lines) {
		return m, nil
	}
	info := m.textarea.LineInfo()
	col := info.CharOffset
	if col > 0 {
		col--
		var c tea.Cmd
		m.textarea, c = m.textarea.Update(tea.KeyMsg{Type: tea.KeyLeft})
		cmds = append(cmds, c)
	}
	line := []rune(lines[lineNum])
	for col > 0 && isSpace(line[col]) {
		col--
		var c tea.Cmd
		m.textarea, c = m.textarea.Update(tea.KeyMsg{Type: tea.KeyLeft})
		cmds = append(cmds, c)
	}
	for col > 0 && !isSpace(line[col-1]) {
		col--
		var c tea.Cmd
		m.textarea, c = m.textarea.Update(tea.KeyMsg{Type: tea.KeyLeft})
		cmds = append(cmds, c)
	}
	return m, tea.Batch(cmds...)
}

func viDeleteLine(m Model) (Model, tea.Cmd) {
	var cmds []tea.Cmd
	val := m.textarea.Value()
	lines := strings.Split(val, "\n")
	lineNum := m.textarea.Line()
	if lineNum >= len(lines) {
		return m, nil
	}
	m.yankBuffer = lines[lineNum]
	newLines := append(lines[:lineNum:lineNum], lines[lineNum+1:]...)
	m.textarea.SetValue(strings.Join(newLines, "\n"))
	target := lineNum
	if target >= len(newLines) {
		target = len(newLines) - 1
	}
	if target < 0 {
		target = 0
	}
	for i := 0; i < target; i++ {
		var c tea.Cmd
		m.textarea, c = m.textarea.Update(tea.KeyMsg{Type: tea.KeyDown})
		cmds = append(cmds, c)
	}
	return m, tea.Batch(cmds...)
}

func viYankLine(m Model) (Model, tea.Cmd) {
	val := m.textarea.Value()
	lines := strings.Split(val, "\n")
	lineNum := m.textarea.Line()
	if lineNum < len(lines) {
		m.yankBuffer = lines[lineNum]
	}
	return m, nil
}

func viPasteAfter(m Model) (Model, tea.Cmd) {
	if m.yankBuffer == "" {
		return m, nil
	}
	val := m.textarea.Value()
	lines := strings.Split(val, "\n")
	lineNum := m.textarea.Line()
	insertAt := lineNum + 1
	newLines := make([]string, 0, len(lines)+1)
	newLines = append(newLines, lines[:insertAt]...)
	newLines = append(newLines, m.yankBuffer)
	newLines = append(newLines, lines[insertAt:]...)
	m.textarea.SetValue(strings.Join(newLines, "\n"))
	var c tea.Cmd
	m.textarea, c = m.textarea.Update(tea.KeyMsg{Type: tea.KeyDown})
	return m, c
}

func viPasteBefore(m Model) (Model, tea.Cmd) {
	if m.yankBuffer == "" {
		return m, nil
	}
	val := m.textarea.Value()
	lines := strings.Split(val, "\n")
	lineNum := m.textarea.Line()
	newLines := make([]string, 0, len(lines)+1)
	newLines = append(newLines, lines[:lineNum]...)
	newLines = append(newLines, m.yankBuffer)
	newLines = append(newLines, lines[lineNum:]...)
	m.textarea.SetValue(strings.Join(newLines, "\n"))
	return m, nil
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t'
}
