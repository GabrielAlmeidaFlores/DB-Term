package sidebar

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gabrielfloresousion/db-term/internal/config"
	"github.com/gabrielfloresousion/db-term/internal/types"
	"github.com/gabrielfloresousion/db-term/internal/ui/styles"
)

// ActionKind identifies what action the sidebar wants the parent to take.
type ActionKind int

const (
	ActionNone           ActionKind = iota
	ActionOpenTable                 // user pressed Enter on a table/view node
	ActionNewConn                   // user pressed the new-connection key
	ActionDeleteConn                // user pressed the delete key on a connection
	ActionExpandDatabase            // user expanded a database node (lazy-load schemas)
	ActionConnect                   // user pressed Enter on a disconnected connection
	ActionEditConn                  // user pressed 'e' on a connection node
	ActionDuplicateConn
	ActionCreateDatabase // user pressed 'n' on a connected connection node
	ActionDropDatabase   // user pressed 'd' on a database node
)

// Action carries the intent from the sidebar to the root model.
type Action struct {
	Kind       ActionKind
	ConnName   string
	DBName     string
	SchemaName string
	TableName  string
}

// Model is the Bubble Tea model for the sidebar panel.
type Model struct {
	roots      []*TreeNode
	flat       []*TreeNode // current visible flat list
	cursor     int         // absolute index of the focused node within flat
	scrollTop  int         // index of the first visible row (viewport top)
	filterMode bool
	filter     textinput.Model
	width      int
	height     int
	focused    bool
	styles     styles.Styles
	keybinds   config.Keybinds

	lastAction Action // consumed by the parent via TakeAction()
}

// New returns an initialised sidebar Model.
func New(s styles.Styles, kb config.Keybinds) Model {
	fi := textinput.New()
	fi.Placeholder = "filter…"
	fi.CharLimit = 64

	return Model{
		styles:   s,
		keybinds: kb,
		filter:   fi,
	}
}

// Init satisfies tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// SetStyles updates the lipgloss styles (called on theme change).
func (m *Model) SetStyles(s styles.Styles) { m.styles = s }

// SetSize sets the panel dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Focus gives keyboard focus to this panel.
func (m *Model) Focus() { m.focused = true }

// Blur removes keyboard focus from this panel.
func (m *Model) Blur() { m.focused = false }

// HasConnections reports whether the sidebar has any connection nodes.
func (m *Model) HasConnections() bool { return len(m.roots) > 0 }

// CurrentFocusedNode returns the TreeNode currently under the cursor, or nil.
func (m *Model) CurrentFocusedNode() *TreeNode { return m.currentNode() }

// TakeAction returns and clears the last pending action.
func (m *Model) TakeAction() Action {
	a := m.lastAction
	m.lastAction = Action{}
	return a
}

// SetConnState updates the state of a connection node (e.g. on connect/disconnect).
func (m *Model) SetConnState(connName string, state types.ConnState) {
	for _, root := range m.roots {
		if root.ConnName == connName && root.Kind == NodeConnection {
			root.State = state
		}
	}
	m.rebuildFlat()
}

// SetDatabases populates the connection node with its list of databases
// after the initial connection succeeds. Each database starts as a collapsed
// node; schemas are loaded lazily when expanded.
func (m *Model) SetDatabases(connName, driver string, state types.ConnState, databases []string) {
	tree := BuildConnectionNode(connName, driver, state, databases)

	for i, r := range m.roots {
		if r.ConnName == connName {
			m.roots[i] = tree
			m.rebuildFlat()
			return
		}
	}
	m.roots = append(m.roots, tree)
	m.rebuildFlat()
}

// SetSchemas populates the schemas for a specific database node.
// connName identifies the parent connection; dbName identifies the database.
// When dbName is empty, schemas are attached directly to the connection node
// (used when the connection targets a specific database).
func (m *Model) SetSchemas(connName, driver, dbName string, state types.ConnState, schemas []types.Schema) {
	for _, root := range m.roots {
		if root.ConnName != connName {
			continue
		}
		if dbName == "" {
			tree := BuildTree(connName, driver, state, schemas)
			tree.Expanded = true
			for i, r := range m.roots {
				if r.ConnName == connName {
					m.roots[i] = tree
					break
				}
			}
			m.rebuildFlat()
			return
		}
		for _, child := range root.Children {
			if child.Kind == NodeDatabase && child.DBName == dbName {
				AttachSchemas(child, schemas)
				m.rebuildFlat()
				return
			}
		}
	}
	m.rebuildFlat()
}

// SetDatabaseLoading marks a database node as loading (shows spinner-like icon).
func (m *Model) SetDatabaseLoading(connName, dbName string, loading bool) {
	for _, root := range m.roots {
		if root.ConnName != connName {
			continue
		}
		for _, child := range root.Children {
			if child.Kind == NodeDatabase && child.DBName == dbName {
				child.Loading = loading
			}
		}
	}
	m.rebuildFlat()
}

// AddConnection adds a disconnected connection node (before schema loads).
// driver is used to select the engine-specific icon.
func (m *Model) AddConnection(connName, driver string) {
	for _, r := range m.roots {
		if r.ConnName == connName {
			return // already exists
		}
	}
	m.roots = append(m.roots, &TreeNode{
		Kind:     NodeConnection,
		Label:    connName,
		ConnName: connName,
		Driver:   driver,
		State:    types.StateDisconnected,
	})
	m.rebuildFlat()
}

// RemoveConnection removes a connection and its entire subtree.
func (m *Model) RemoveConnection(connName string) {
	filtered := m.roots[:0]
	for _, r := range m.roots {
		if r.ConnName != connName {
			filtered = append(filtered, r)
		}
	}
	m.roots = filtered
	// rebuildFlat will clamp the cursor correctly after rebuilding.
	// Do not clamp against the stale m.flat here.
	m.rebuildFlat()
}

func (m *Model) rebuildFlat() {
	if m.filterMode {
		m.flat = FilterNodes(m.roots, m.filter.Value())
		// In filter mode, always start from the top of results so the user
		// sees all matches — not just the last one (which is where the cursor
		// would land after clamping from a larger, pre-filter flat list).
		m.cursor = 0
		m.scrollTop = 0
		return
	}
	var all []*TreeNode
	for _, r := range m.roots {
		all = append(all, FlatList(r)...)
	}
	m.flat = all
	if m.cursor >= len(m.flat) {
		m.cursor = max(0, len(m.flat)-1)
	}
	m.clampScrollTop()
}

// Update handles keyboard and window events.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if m.filterMode {
		return m.updateFilter(msg)
	}
	return m.updateNormal(msg)
}

// IsFiltering reports whether keyboard input is currently editing the sidebar filter.
func (m Model) IsFiltering() bool {
	return m.filterMode
}

func (m Model) updateNormal(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if !m.focused {
			return m, nil
		}
		node := m.currentNode()
		switch msg.String() {
		case "up", "k":
			m.moveCursor(-1)
		case "down", "j":
			m.moveCursor(+1)
		case "right", "enter":
			m.activate()
		case "left":
			m.collapse()
		case m.keybinds.FilterSidebar:
			m.filterMode = true
			m.filter.SetValue("")
			m.filter.Focus()
			m.rebuildFlat()

		case "e":
			// 'e' is context-sensitive: edit connection or (future) edit cell.
			if node != nil && node.Kind == NodeConnection {
				m.lastAction = Action{Kind: ActionEditConn, ConnName: node.ConnName}
			}

		case "y":
			if node != nil && node.Kind == NodeConnection {
				m.lastAction = Action{Kind: ActionDuplicateConn, ConnName: node.ConnName}
			}

		case m.keybinds.NewConnection:
			// 'n' creates a new connection when on the connection list, but
			// creates a new database when focused on a connected connection node.
			if node != nil && node.Kind == NodeConnection &&
				(node.State == types.StateConnected || len(node.Children) > 0) {
				m.lastAction = Action{Kind: ActionCreateDatabase, ConnName: node.ConnName}
			} else {
				m.lastAction = Action{Kind: ActionNewConn}
			}

		case m.keybinds.DeleteConnection:
			if node == nil {
				break
			}
			switch node.Kind {
			case NodeConnection:
				m.lastAction = Action{Kind: ActionDeleteConn, ConnName: node.ConnName}
			case NodeDatabase:
				m.lastAction = Action{Kind: ActionDropDatabase,
					ConnName: node.ConnName, DBName: node.DBName}
			}
		}
	}
	return m, nil
}

// moveCursor moves the cursor by delta rows and scrolls the viewport only
// when the cursor would leave the visible window.
func (m *Model) moveCursor(delta int) {
	next := m.cursor + delta
	if next < 0 {
		next = 0
	}
	if next >= len(m.flat) {
		next = len(m.flat) - 1
	}
	if next < 0 {
		return
	}
	m.cursor = next
	m.clampScrollTop()
}

// listHeight returns the number of rows visible in the sidebar list area.
func (m *Model) listHeight() int {
	h := m.height - 4
	if h < 1 {
		h = 1
	}
	return h
}

// clampScrollTop adjusts scrollTop so the cursor is always within the visible
// window, scrolling by the minimum amount necessary. The cursor moves first;
// the viewport follows only when the cursor would go out of view.
func (m *Model) clampScrollTop() {
	lh := m.listHeight()
	if m.cursor < m.scrollTop {
		m.scrollTop = m.cursor
	}
	if m.cursor >= m.scrollTop+lh {
		m.scrollTop = m.cursor - lh + 1
	}
	if m.scrollTop < 0 {
		m.scrollTop = 0
	}
}

func (m Model) updateFilter(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.filterMode = false
			m.filter.Blur()
			m.filter.SetValue("")
			m.rebuildFlat()
			return m, nil
		case "enter":
			m.filterMode = false
			m.filter.Blur()
			m.filter.SetValue("")
			m.activate()
			m.rebuildFlat()
			return m, nil
		case "up", "k":
			m.moveCursor(-1)
			return m, nil
		case "down", "j":
			m.moveCursor(+1)
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)
	m.rebuildFlat()
	return m, cmd
}

func (m *Model) activate() {
	node := m.currentNode()
	if node == nil {
		return
	}
	if node.IsLeaf() {
		m.lastAction = Action{
			Kind:       ActionOpenTable,
			ConnName:   node.ConnName,
			DBName:     node.DBName,
			SchemaName: node.SchemaName,
			TableName:  node.Label,
		}
		return
	}
	// Disconnected or errored connection node: trigger reconnect instead of expand.
	if node.Kind == NodeConnection &&
		(node.State == types.StateDisconnected || node.State == types.StateError) {
		m.lastAction = Action{Kind: ActionConnect, ConnName: node.ConnName}
		return
	}
	// Database nodes trigger lazy schema loading on first expand.
	if node.Kind == NodeDatabase && !node.Expanded && len(node.Children) == 0 {
		node.Loading = true
		m.lastAction = Action{
			Kind:     ActionExpandDatabase,
			ConnName: node.ConnName,
			DBName:   node.DBName,
		}
		m.rebuildFlat()
		return
	}
	node.Expanded = !node.Expanded
	m.rebuildFlat()
}

func (m *Model) collapse() {
	node := m.currentNode()
	if node == nil {
		return
	}
	if node.Expanded {
		node.Expanded = false
		m.rebuildFlat()
	}
}

func (m *Model) currentNode() *TreeNode {
	if len(m.flat) == 0 || m.cursor >= len(m.flat) {
		return nil
	}
	return m.flat[m.cursor]
}

// View renders the sidebar panel.
func (m Model) View() string {
	if m.width < 4 || m.height < 3 {
		return ""
	}

	inner := m.renderInner()

	borderStyle := m.styles.PanelUnfocused
	if m.focused {
		borderStyle = m.styles.PanelFocused
	}

	contentW := m.width - 2
	contentH := m.height - 2
	if contentW < 1 {
		contentW = 1
	}
	if contentH < 1 {
		contentH = 1
	}

	return borderStyle.
		Width(contentW).
		Height(contentH).
		Render(inner)
}

func (m Model) renderInner() string {
	contentW := m.width - 4
	if contentW < 1 {
		contentW = 1
	}

	titleText := plainTruncate(styles.IconConnections+"  Connections", contentW)
	title := m.styles.Title.Render(titleText)

	listH := m.height - 4
	if listH < 1 {
		listH = 1
	}

	var rows []string

	start := m.scrollTop
	end := start + listH
	if end > len(m.flat) {
		end = len(m.flat)
	}

	if len(m.roots) == 0 {
		line1 := plainTruncate("  No connections yet.", contentW)
		line2 := plainTruncate("  Press ["+m.keybinds.NewConnection+"] to add one.", contentW)
		hint := m.styles.Muted.Render("\n" + line1 + "\n" + line2)
		return lipgloss.JoinVertical(lipgloss.Left, title, hint)
	}

	// In filter mode, only nodes in m.flat are shown. FlatItemsFiltered
	// recomputes tree connectors (├─/└─/│) relative to visible siblings so
	// the tree UI is preserved even when many siblings are hidden.
	// In normal mode, FlatItems walks the full expanded tree.
	var allItems []FlatItem
	if m.filterMode {
		visible := make(map[*TreeNode]bool, len(m.flat))
		for _, n := range m.flat {
			visible[n] = true
		}
		allItems = FlatItemsFiltered(m.roots, visible)
	} else {
		allItems = FlatItems(m.roots)
	}

	if m.filterMode && m.filter.Value() != "" && len(m.flat) == 0 {
		noLine1 := plainTruncate("  No matches. Open a database first", contentW)
		noLine2 := plainTruncate("  to load its tables.", contentW)
		noResult := m.styles.Muted.Render("\n" + noLine1 + "\n" + noLine2)
		hint := m.styles.Muted.Render(styles.IconFilter+" ") + m.filter.View()
		return lipgloss.JoinVertical(lipgloss.Left, title, noResult, hint)
	}

	for i := start; i < end; i++ {
		var item FlatItem
		if i < len(allItems) {
			item = allItems[i]
		} else {
			item = FlatItem{Node: m.flat[i], Prefix: ""}
		}
		row := m.renderItem(item, i == m.cursor, contentW)
		rows = append(rows, row)
	}

	for len(rows) < listH {
		rows = append(rows, "")
	}

	var hint string
	if m.filterMode {
		hint = m.styles.Muted.Render(styles.IconFilter+" ") + m.filter.View()
	}

	rows2 := []string{title, strings.Join(rows, "\n")}
	if hint != "" {
		rows2 = append(rows2, hint)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, rows2...)
	return content
}

// renderItem renders a single tree entry using the FlatItem's pre-computed
// renderItem renders a single tree entry using the FlatItem's pre-computed
// connector prefix to express hierarchy visually.
//
// All truncation is applied to plain (non-ANSI) strings before styling is
// added. Truncating an ANSI-styled string with []rune counts escape sequences
// as runes, cutting in the middle of colour codes and corrupting output.
func (m Model) renderItem(item FlatItem, selected bool, maxW int) string {
	node := item.Node

	iconStr := node.Icon() + " "
	prefixCols := lipgloss.Width(item.Prefix)
	iconCols := lipgloss.Width(iconStr)

	if node.Kind == NodeConnection {
		stateIcon := node.StateIcon()
		fixedCols := prefixCols + iconCols + lipgloss.Width(stateIcon) + 1
		avail := maxW - fixedCols
		if avail < 0 {
			avail = 0
		}
		// Truncate label in plain text before styling.
		labelRunes := []rune(node.Label)
		if len(labelRunes) > avail {
			labelRunes = labelRunes[:avail]
		}
		plainLabel := fmt.Sprintf("%-*s", avail, string(labelRunes))
		stateStr := m.colorState(node.State, stateIcon)

		if selected {
			if node.State == types.StateDisconnected || node.State == types.StateError {
				hint := m.styles.Muted.Render(" [↵]")
				return m.styles.TreeSelected.Render(iconStr+plainLabel) + " " + stateStr + hint
			}
			return m.styles.TreeSelected.Render(iconStr+plainLabel) + " " + stateStr
		}
		prefix := m.styles.Muted.Render(item.Prefix)
		icon := m.styles.Muted.Render(iconStr)
		return prefix + icon + m.styles.TreeConn.Render(plainLabel) + " " + stateStr
	}

	// Truncate the label in plain text so the total visible width fits maxW.
	labelAvail := maxW - prefixCols - iconCols
	if labelAvail < 0 {
		labelAvail = 0
	}
	plainLabel := plainTruncate(node.Label, labelAvail)

	if selected {
		return m.styles.TreeSelected.Render(item.Prefix + iconStr + plainLabel)
	}

	prefix := m.styles.Muted.Render(item.Prefix)
	icon := m.styles.Muted.Render(iconStr)

	var styledLabel string
	switch node.Kind {
	case NodeDatabase:
		if node.Loading {
			styledLabel = m.styles.Warning.Render(plainLabel + " " + styles.IconConnecting)
		} else {
			styledLabel = m.styles.TreeConn.Render(plainLabel)
		}
	case NodeSchema:
		styledLabel = m.styles.TreeSchema.Render(plainLabel)
	case NodeTableGroup, NodeViewGroup:
		styledLabel = m.styles.Muted.Render(plainLabel)
	case NodeTable:
		styledLabel = m.styles.TreeTable.Render(plainLabel)
	case NodeView:
		styledLabel = m.styles.TreeView.Render(plainLabel)
	default:
		styledLabel = m.styles.Muted.Render(plainLabel)
	}

	return prefix + icon + styledLabel
}

func (m Model) colorState(state types.ConnState, icon string) string {
	switch state {
	case types.StateConnected:
		return m.styles.Success.Render(icon)
	case types.StateConnecting:
		return m.styles.Warning.Render(icon)
	case types.StateError:
		return m.styles.Error.Render(icon)
	default:
		return m.styles.Muted.Render(icon)
	}
}

// truncate shortens a plain (non-ANSI) string to maxW visible columns.
// It must only be called on strings that contain no ANSI escape sequences;
// use plainTruncate for node labels and item.Prefix values.
func truncate(s string, maxW int) string {
	return plainTruncate(s, maxW)
}

// plainTruncate shortens s to at most maxW runes, appending "…" when cut.
// This function is safe for plain text only — do not pass ANSI-styled strings.
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
