// Package app contains the root Bubble Tea model for db-term.
package app

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gabrielfloresousion/db-term/internal/clipboard"
	"github.com/gabrielfloresousion/db-term/internal/config"
	"github.com/gabrielfloresousion/db-term/internal/db"
	"github.com/gabrielfloresousion/db-term/internal/types"
	"github.com/gabrielfloresousion/db-term/internal/ui/components"
	"github.com/gabrielfloresousion/db-term/internal/ui/layout"
	"github.com/gabrielfloresousion/db-term/internal/ui/panels/editor"
	"github.com/gabrielfloresousion/db-term/internal/ui/panels/results"
	"github.com/gabrielfloresousion/db-term/internal/ui/panels/settings"
	"github.com/gabrielfloresousion/db-term/internal/ui/panels/sidebar"
	"github.com/gabrielfloresousion/db-term/internal/ui/styles"
)

// connectedMsg is fired when db.Manager.Connect() succeeds.
type connectedMsg struct{ connName string }

// connectErrMsg is fired when db.Manager.Connect() fails.
type connectErrMsg struct {
	connName string
	err      error
}

// databasesLoadedMsg is fired after listing the databases on the instance.
type databasesLoadedMsg struct {
	connName  string
	driver    string
	databases []string
}

// dbOperationMsg is fired when a CREATE/DROP DATABASE command completes.
type dbOperationMsg struct {
	connName string
	driver   string
	err      error
	message  string // success message to show in status bar
}

// schemaLoadedMsg is fired after schema introspection for one database completes.
type schemaLoadedMsg struct {
	connName string
	driver   string
	dbName   string // empty when connection targets a specific database directly
	schemas  []types.Schema
}

// queryResultMsg is fired when db.Manager.RunQuery() completes.
type queryResultMsg struct {
	columns []string
	rows    [][]string
	elapsed time.Duration
	err     error
}

// updateCellResultMsg is fired when an edit-cell UPDATE completes.
// On success, the app re-runs the last SELECT to refresh the results grid.
type updateCellResultMsg struct {
	err error
}

// testConnMsg is fired when the connform test connection completes.
type testConnMsg struct{ err error }

// externalEditorClosedMsg is fired after the external editor process exits.
// tmpFile is the path of the temp file whose content should be loaded back
// into the SQL editor. The caller is responsible for removing the file.
type externalEditorClosedMsg struct {
	tmpFile string
	err     error
}

// keepaliveTickMsg drives the keepalive ping cycle.
type keepaliveTickMsg struct{}

// pingResultMsg is returned by the keepalive ping goroutine.
type pingResultMsg struct {
	connName string
	err      error // non-nil if the connection is dead
}

type fkLoadedMsg struct {
	connName string
	schema   string
	table    string
	fkInfo   map[string]*types.ForeignKey
}

type fkQueryResultMsg struct {
	connName string
	schema   string
	table    string
	sql      string
	columns  []string
	rows     [][]string
	elapsed  time.Duration
	err      error
}

// PanelID identifies which panel has keyboard focus.
type PanelID int

const (
	PanelSidebar PanelID = iota
	PanelEditor
	PanelResults
)

// Model is the root Bubble Tea model. It owns all sub-models and services.
type Model struct {
	sidebar   sidebar.Model
	editor    editor.Model
	results   results.Model
	settings  settings.Model
	statusbar components.StatusBar

	modal        *components.Modal
	connform     *components.ConnForm
	inputModal   *components.InputModal // generic single-line text prompt
	showSettings bool
	showHelp     bool

	// connformTestFired prevents firing the test-connection cmd more than once
	// per stepTesting entry (the connform is a value; the flag lives on Model).
	connformTestFired bool

	// pendingDeleteConn holds the connection name waiting for delete confirmation,
	// bridging the modal show and the modal confirm without a side channel.
	pendingDeleteConn string

	// lastQueryConn and lastQueryTable track the most recently queried table so
	// that edit-cell can generate a correct UPDATE statement.
	lastQueryConn   string
	lastQuerySchema string
	lastQueryTable  string
	lastSelectSQL   string // the last SELECT fired; re-run after a successful UPDATE

	// pendingEditAction holds the EditCellAction while the InputModal is open.
	pendingEditAction *results.EditCellAction

	// pendingCursorRow/Col: row and column to restore after a results refresh.
	// -1 means no restoration pending.
	pendingCursorRow int
	pendingCursorCol int

	activePanel    PanelID
	sidebarVisible bool

	sidebarW    int // sidebar column width; clamped to [SidebarMinW, SidebarMaxW]
	editorRatio int // editor height as % of right column; clamped to [15, 85]

	width  int
	height int

	dbMgr   *db.Manager
	cfg     *config.Config
	secrets config.SecretsStore
	styles  styles.Styles
}

// New returns a fully initialised root Model.
func New(cfg *config.Config, secrets config.SecretsStore, dbMgr *db.Manager) Model {
	s := styles.ToStyles(styles.Resolve(cfg.Settings.Theme, cfg.Theme.Custom))
	kb := cfg.Keybinds

	sb := sidebar.New(s, kb)
	for _, conn := range cfg.Connections {
		sb.AddConnection(conn.Name, conn.Driver)
	}

	m := Model{
		sidebar:          sb,
		editor:           editor.New(s, kb, cfg.Settings),
		results:          results.New(s, kb),
		settings:         settings.New(cfg, s),
		statusbar:        components.NewStatusBar(s, kb),
		activePanel:      PanelSidebar,
		sidebarVisible:   true,
		sidebarW:         layout.DefaultSidebarWidth,
		editorRatio:      layout.DefaultEditorRatio,
		dbMgr:            dbMgr,
		cfg:              cfg,
		secrets:          secrets,
		styles:           s,
		pendingCursorRow: -1,
		pendingCursorCol: -1,
	}
	m.sidebar.Focus()
	m.statusbar.SetHints(m.sidebarHints())
	return m
}

// Init starts background commands, including auto-reconnect for every saved connection.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.results.Init(),
		m.editor.Init(),
		keepaliveTick(),
	}
	for _, conn := range m.cfg.Connections {
		c := conn
		password, err := m.secrets.GetPassword(c.Name)
		if err != nil {
			continue
		}
		m.sidebar.SetConnState(c.Name, types.StateConnecting)
		cmds = append(cmds, m.connectCmd(c, password))
	}
	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Always propagate non-key messages to results so the spinner animates
	// even when another panel has focus.
	if _, isKey := msg.(tea.KeyMsg); !isKey {
		var re results.Model
		var reCmd tea.Cmd
		re, reCmd = m.results.Update(msg)
		m.results = re
		if reCmd != nil {
			cmds = append(cmds, reCmd)
		}
	}

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m = m.resize()

	case tea.KeyMsg:
		m, cmds = m.handleKey(msg, cmds)

	case connectedMsg:
		m.sidebar.SetConnState(msg.connName, types.StateConnected)
		m.statusbar.ShowMessage(msg.connName + " connected")
		snap := m.dbMgr.Get(msg.connName)
		if snap != nil {
			connName := msg.connName
			driver := snap.Config.Driver
			specifiedDB := snap.Config.Database
			dbMgr := m.dbMgr
			cmds = append(cmds, func() tea.Msg {
				if specifiedDB != "" {
					res := dbMgr.RunQuery(connName, introspectSQL(driver))
					if res.Err != nil {
						return schemaLoadedMsg{connName: connName, driver: driver, schemas: nil}
					}
					return schemaLoadedMsg{
						connName: connName,
						driver:   driver,
						schemas:  parseSchemaRows(res.Columns, res.Rows),
					}
				}
				dbs, err := dbMgr.ListDatabases(connName, driver)
				if err != nil {
					return databasesLoadedMsg{connName: connName, driver: driver, databases: nil}
				}
				return databasesLoadedMsg{connName: connName, driver: driver, databases: dbs}
			})
		}

	case connectErrMsg:
		m.sidebar.SetConnState(msg.connName, types.StateError)
		m.modal = modalErr("Connection failed",
			fmt.Sprintf("Could not connect to %q:\n%s", msg.connName, msg.err.Error()),
			m.styles, m.width, m.height)

	case databasesLoadedMsg:
		m.sidebar.SetDatabases(msg.connName, msg.driver, types.StateConnected, msg.databases)

	case schemaLoadedMsg:
		m.sidebar.SetSchemas(msg.connName, msg.driver, msg.dbName, types.StateConnected, msg.schemas)

	case queryResultMsg:
		m.results.SetResult(msg.columns, msg.rows, msg.elapsed, msg.err)
		m.results.SetFKInfo(nil)
		m.statusbar.SetQuerying(false)
		m.statusbar.SetResult(len(msg.rows), msg.elapsed)
		if m.pendingCursorRow >= 0 {
			m.results.SetCursor(m.pendingCursorRow, m.pendingCursorCol)
			m.pendingCursorRow = -1
			m.pendingCursorCol = -1
		}
		if msg.err != nil {
			m.modal = modalErr("Query error", msg.err.Error(), m.styles, m.width, m.height)
		} else if m.lastQueryTable != "" && m.lastQueryConn != "" {
			connName := m.lastQueryConn
			schema := m.lastQuerySchema
			tableSnap := m.lastQueryTable
			snap := m.dbMgr.Get(connName)
			if snap != nil {
				driver := snap.Config.Driver
				dbMgr := m.dbMgr
				cmds = append(cmds, func() tea.Msg {
					fks, err := dbMgr.LoadForeignKeys(connName, driver, schema, tableSnap)
					if err != nil || len(fks) == 0 {
						return fkLoadedMsg{connName: connName, schema: schema, table: tableSnap}
					}
					return fkLoadedMsg{connName: connName, schema: schema, table: tableSnap, fkInfo: fks}
				})
			}
		}

	case fkLoadedMsg:
		if msg.connName == m.lastQueryConn && msg.schema == m.lastQuerySchema && msg.table == m.lastQueryTable {
			m.results.SetFKInfo(msg.fkInfo)
		}

	case fkQueryResultMsg:
		m.results.SetQuerying(false)
		m.statusbar.SetQuerying(false)
		if msg.err != nil {
			m.statusbar.ShowMessage("could not follow foreign key: " + msg.err.Error())
			break
		}
		m.lastQueryConn = msg.connName
		m.lastQuerySchema = msg.schema
		m.lastQueryTable = msg.table
		m.lastSelectSQL = msg.sql
		m.results.SetResult(msg.columns, msg.rows, msg.elapsed, nil)
		m.results.SetFKInfo(nil)
		m.statusbar.SetResult(len(msg.rows), msg.elapsed)
		snap := m.dbMgr.Get(msg.connName)
		if snap != nil {
			driver := snap.Config.Driver
			connName := msg.connName
			schema := msg.schema
			table := msg.table
			dbMgr := m.dbMgr
			cmds = append(cmds, func() tea.Msg {
				fks, err := dbMgr.LoadForeignKeys(connName, driver, schema, table)
				if err != nil || len(fks) == 0 {
					return fkLoadedMsg{connName: connName, schema: schema, table: table}
				}
				return fkLoadedMsg{connName: connName, schema: schema, table: table, fkInfo: fks}
			})
		}

	case updateCellResultMsg:
		if msg.err != nil {
			m.modal = modalErr("Update error", msg.err.Error(), m.styles, m.width, m.height)
		} else {
			m.statusbar.ShowMessage("row updated")
			if m.lastSelectSQL != "" {
				m.results.SetQuerying(true)
				m.statusbar.SetQuerying(true)
				connName := m.lastQueryConn
				sql := m.lastSelectSQL
				dbMgr := m.dbMgr
				cmds = append(cmds, func() tea.Msg {
					res := dbMgr.RunQuery(connName, sql)
					return queryResultMsg{columns: res.Columns, rows: res.Rows, elapsed: res.Elapsed, err: res.Err}
				})
			}
		}

	case testConnMsg:
		if m.connform != nil {
			m.connform.SetTestResult(msg.err)
		}

	case externalEditorClosedMsg:
		defer os.Remove(msg.tmpFile)
		if msg.err != nil {
			m.statusbar.ShowMessage("external editor error: " + msg.err.Error())
		} else {
			content, readErr := os.ReadFile(msg.tmpFile)
			if readErr != nil {
				m.statusbar.ShowMessage("could not read temp file: " + readErr.Error())
			} else {
				m.editor.SetSQL(strings.TrimRight(string(content), "\n"))
				m.statusbar.ShowMessage("editor content updated")
			}
		}

	case dbOperationMsg:
		if msg.err != nil {
			m.modal = modalErr("Database operation failed", msg.err.Error(), m.styles, m.width, m.height)
		} else {
			m.statusbar.ShowMessage(msg.message)
			// Refresh the database list for this connection.
			snap := m.dbMgr.Get(msg.connName)
			if snap != nil {
				connName := msg.connName
				driver := msg.driver
				dbMgr := m.dbMgr
				cmds = append(cmds, func() tea.Msg {
					dbs, err := dbMgr.ListDatabases(connName, driver)
					if err != nil {
						return databasesLoadedMsg{connName: connName, driver: driver, databases: nil}
					}
					return databasesLoadedMsg{connName: connName, driver: driver, databases: dbs}
				})
			}
		}

	case keepaliveTickMsg:
		for _, snap := range m.dbMgr.All() {
			if snap.State == types.StateConnected {
				name := snap.Config.Name
				cmds = append(cmds, func() tea.Msg {
					if err := m.dbMgr.Ping(name); err != nil {
						return pingResultMsg{connName: name, err: err}
					}
					return nil // ping OK — no state change needed
				})
			}
		}
		cmds = append(cmds, keepaliveTick())

	case pingResultMsg:
		if msg.err != nil {
			// Connection is dead — reflect this in the sidebar.
			m.sidebar.SetConnState(msg.connName, types.StateError)
			m.statusbar.ShowMessage(msg.connName + " lost connection")
		}
	}

	m.statusbar.Tick()

	return m, tea.Batch(cmds...)
}

// handleKey routes keyboard events with correct priority order.
func (m Model) handleKey(msg tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	key := msg.String()

	if m.activePanel == PanelSidebar && m.sidebar.IsFiltering() && isPlainKey(key) {
		m, cmds = m.delegateToPanel(msg, cmds)
		return m, cmds
	}

	if key == m.cfg.Keybinds.OpenSettings {
		m.showSettings = !m.showSettings
		return m, cmds
	}

	if m.modal != nil {
		var mo components.Modal
		var moCmd tea.Cmd
		mo, moCmd = m.modal.Update(msg)
		*m.modal = mo
		if moCmd != nil {
			cmds = append(cmds, moCmd)
		}
		if m.modal.TakeCopyBody() {
			if err := clipboard.Copy(m.modal.Body); err != nil {
				m.modal.SetStatus("clipboard unavailable")
			} else {
				m.modal.SetStatus("error message copied")
			}
		}
		if m.modal.Dismissed() {
			m.modal.ResetDismissed()
			m.modal = nil
		}
		if a := m.modal; a != nil {
			if action := a.TakeAction(); action != nil {
				var modalCmds []tea.Cmd
				m, modalCmds = m.handleModalAction(action.Label)
				cmds = append(cmds, modalCmds...)
			}
		}
		return m, cmds
	}

	if m.inputModal != nil {
		var im components.InputModal
		var imCmd tea.Cmd
		im, imCmd = m.inputModal.Update(msg)
		*m.inputModal = im
		if imCmd != nil {
			cmds = append(cmds, imCmd)
		}
		if a := m.inputModal.TakeAction(); a != nil {
			m, cmds = m.handleInputModalAction(a, cmds)
		}
		return m, cmds
	}

	if m.connform != nil {
		var cf components.ConnForm
		var cfCmd tea.Cmd
		cf, cfCmd = m.connform.Update(msg)
		*m.connform = cf
		if cfCmd != nil {
			cmds = append(cmds, cfCmd) // preserves textinput cursor blink cmd
		}

		// If we just entered stepTesting and haven't fired the test yet, do it now.
		m, cmds = m.maybeFireConnTest(cmds)

		if a := m.connform.TakeAction(); a != nil {
			if a.Cancelled {
				m.connform = nil
				m.connformTestFired = false
			} else if a.IsEdit {
				// Update an existing connection in-place.
				for i, c := range m.cfg.Connections {
					if c.Name == a.OriginalName {
						m.cfg.Connections[i] = a.Connection
						break
					}
				}
				if err := m.cfg.Save(); err != nil {
					m.statusbar.ShowMessage("config save error: " + err.Error())
				}
				// Update password only if the user entered a new one.
				if a.Password != "" {
					if err := m.secrets.SetPassword(a.Connection.Name, a.Password); err != nil {
						m.statusbar.ShowMessage("secrets error: " + err.Error())
					} else if err := m.secrets.Save(); err != nil {
						m.statusbar.ShowMessage("secrets save error: " + err.Error())
					}
				} else if a.OriginalName != a.Connection.Name {
					// Name changed: migrate the password entry.
					if pw, err := m.secrets.GetPassword(a.OriginalName); err == nil {
						m.secrets.DeletePassword(a.OriginalName)
						_ = m.secrets.SetPassword(a.Connection.Name, pw)
						_ = m.secrets.Save()
					}
				}
				m.settings.SetConfig(m.cfg)
				m.statusbar.ShowMessage("connection updated")
				m.connform = nil
				m.connformTestFired = false
			} else {
				// Save a new connection + password, showing errors via statusbar.
				m.cfg.Connections = append(m.cfg.Connections, a.Connection)
				if err := m.cfg.Save(); err != nil {
					m.statusbar.ShowMessage("config save error: " + err.Error())
				}
				if err := m.secrets.SetPassword(a.Connection.Name, a.Password); err != nil {
					m.statusbar.ShowMessage("secrets error: " + err.Error())
				} else if err := m.secrets.Save(); err != nil {
					m.statusbar.ShowMessage("secrets save error: " + err.Error())
				}
				m.sidebar.AddConnection(a.Connection.Name, a.Connection.Driver)
				m.settings.SetConfig(m.cfg)
				m.connform = nil
				m.connformTestFired = false
				// Auto-connect — set StateConnecting on the actual model before
				// appending the cmd (connectCmd is a value receiver).
				conn := a.Connection
				pass := a.Password
				m.sidebar.SetConnState(conn.Name, types.StateConnecting)
				cmds = append(cmds, m.connectCmd(conn, pass))
			}
		}
		return m, cmds
	}

	if m.showSettings {
		var se settings.Model
		var seCmd tea.Cmd
		se, seCmd = m.settings.Update(msg)
		m.settings = se
		if seCmd != nil {
			cmds = append(cmds, seCmd)
		}
		if a := m.settings.TakeAction(); a.Kind != settings.ActionNone {
			switch a.Kind {
			case settings.ActionClose:
				m.showSettings = false
			case settings.ActionThemeChange:
				m = m.applyTheme(a.ThemeID)
			}
		}
		return m, cmds
	}

	switch key {
	case m.cfg.Keybinds.Quit, "ctrl+c":
		m.dbMgr.DisconnectAll()
		cmds = append(cmds, tea.Quit)
		return m, cmds

	case m.cfg.Keybinds.OpenExternalEditor:
		cmd := m.openExternalEditorCmd()
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, cmds
	}

	// When the editor is focused, single-character keys (no modifier) must
	// not fire as globals — they belong to the SQL text the user is typing.
	// Only ctrl/alt combinations and special keys remain global.
	if m.activePanel == PanelEditor && isPlainKey(key) {
		m, cmds = m.delegateToPanel(msg, cmds)
		return m, cmds
	}

	switch key {
	case m.cfg.Keybinds.NewConnection:
		cf := components.NewConnForm(m.styles)
		cf.SetSize(m.width, m.height)
		m.connform = &cf
		m.connformTestFired = false
		return m, cmds

	case m.cfg.Keybinds.ToggleSidebar:
		m.sidebarVisible = !m.sidebarVisible
		m = m.resize()
		return m, cmds

	case m.cfg.Keybinds.ResizePanelLeft:
		m.sidebarW = layout.Clamp(m.sidebarW-layout.SidebarStep, layout.SidebarMinW, layout.SidebarMaxW)
		m = m.resize()
		return m, cmds

	case m.cfg.Keybinds.ResizePanelRight:
		m.sidebarW = layout.Clamp(m.sidebarW+layout.SidebarStep, layout.SidebarMinW, layout.SidebarMaxW)
		m = m.resize()
		return m, cmds

	case m.cfg.Keybinds.ResizePanelUp:
		m.editorRatio = layout.Clamp(m.editorRatio-layout.EditorRatioStep, layout.EditorMinRatio, layout.EditorMaxRatio)
		m = m.resize()
		return m, cmds

	case m.cfg.Keybinds.ResizePanelDown:
		m.editorRatio = layout.Clamp(m.editorRatio+layout.EditorRatioStep, layout.EditorMinRatio, layout.EditorMaxRatio)
		m = m.resize()
		return m, cmds

	case m.cfg.Keybinds.FocusEditor:
		m = m.setFocus(PanelEditor)
		return m, cmds

	case m.cfg.Keybinds.FocusResults:
		m = m.setFocus(PanelResults)
		return m, cmds

	case "tab":
		m = m.cyclePanel(+1)
		return m, cmds

	case "shift+tab":
		m = m.cyclePanel(-1)
		return m, cmds

	case "?":
		m.showHelp = !m.showHelp
		return m, cmds
	}

	if key == m.cfg.Keybinds.CancelQuery {
		connName := m.editor.ActiveConn()
		if connName != "" {
			m.dbMgr.CancelQuery(connName)
		}
		m.results.SetQuerying(false)
		m.statusbar.SetQuerying(false)
		return m, cmds
	}

	m, cmds = m.delegateToPanel(msg, cmds)
	return m, cmds
}

// delegateToPanel sends the key to the focused panel and handles its pending action.
func (m Model) delegateToPanel(msg tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	switch m.activePanel {
	case PanelSidebar:
		var sb sidebar.Model
		var cmd tea.Cmd
		sb, cmd = m.sidebar.Update(msg)
		m.sidebar = sb
		cmds = append(cmds, cmd)
		m, cmds = m.handleSidebarAction(cmds)

	case PanelEditor:
		var ed editor.Model
		var cmd tea.Cmd
		ed, cmd = m.editor.Update(msg)
		m.editor = ed
		cmds = append(cmds, cmd)
		if exec := m.editor.TakeExec(); exec != nil {
			if exec.ConnName == "" {
				m.modal = modalErr("No connection",
					"Select a table in the sidebar first.", m.styles, m.width, m.height)
			} else {
				m.results.SetQuerying(true)
				m.statusbar.SetQuerying(true)
				m.lastQueryConn = exec.ConnName
				m.lastQuerySchema = ""
				m.lastQueryTable = ""
				m.lastSelectSQL = exec.SQL
				connName := exec.ConnName
				sql := exec.SQL
				cmds = append(cmds, func() tea.Msg {
					res := m.dbMgr.RunQuery(connName, sql)
					return queryResultMsg{
						columns: res.Columns,
						rows:    res.Rows,
						elapsed: res.Elapsed,
						err:     res.Err,
					}
				})
			}
		}

	case PanelResults:
		var re results.Model
		var cmd tea.Cmd
		re, cmd = m.results.Update(msg)
		m.results = re
		cmds = append(cmds, cmd)
		if cp := m.results.TakeCopy(); cp != nil {
			if err := clipboard.Copy(cp.Value); err != nil {
				m.statusbar.ShowMessage("clipboard unavailable")
			} else {
				m.statusbar.ShowMessage("copied")
			}
		}
		if ec := m.results.TakeEditCell(); ec != nil {
			im := components.NewInputModal("Edit — "+ec.Column, ec.Value, m.styles)
			im.SetValue(ec.Value)
			im.SetSize(m.width, m.height)
			m.inputModal = &im
			m.pendingEditAction = ec
			m.pendingDeleteConn = "editcell"
		}
		if sc := m.results.TakeSearchAction(); sc != nil {
			im := components.NewInputModal("Search — "+sc.Column, "value or %like%", m.styles)
			im.SetSize(m.width, m.height)
			m.inputModal = &im
			m.pendingDeleteConn = "search\x00" + sc.Column
		}
		if sa := m.results.TakeSortAction(); sa != nil {
			if m.lastSelectSQL != "" {
				snap := m.dbMgr.Get(m.lastQueryConn)
				driver := ""
				if snap != nil {
					driver = snap.Config.Driver
				}
				sorted := injectOrderBy(driver, m.lastSelectSQL, sa.Column, sa.Asc)
				m.lastSelectSQL = sorted
				m.results.SetSort(sa.Column, sa.Asc)
				m.pendingCursorRow, m.pendingCursorCol = m.results.Cursor()
				m.results.SetQuerying(true)
				m.statusbar.SetQuerying(true)
				connName := m.lastQueryConn
				dbMgr := m.dbMgr
				cmds = append(cmds, func() tea.Msg {
					res := dbMgr.RunQuery(connName, sorted)
					return queryResultMsg{columns: res.Columns, rows: res.Rows, elapsed: res.Elapsed, err: res.Err}
				})
			}
		}
		if fk := m.results.TakeFKNavigate(); fk != nil {
			connName := m.lastQueryConn
			snap := m.dbMgr.Get(connName)
			if snap == nil {
				m.statusbar.ShowMessage("could not follow foreign key: connection not found")
			} else {
				driver := snap.Config.Driver
				sql := foreignKeySQL(driver, fk.FK, fk.Value)
				m.results.SetQuerying(true)
				m.statusbar.SetQuerying(true)
				dbMgr := m.dbMgr
				schema := fk.FK.Schema
				table := fk.FK.Table
				cmds = append(cmds, func() tea.Msg {
					res := dbMgr.RunQuery(connName, sql)
					return fkQueryResultMsg{
						connName: connName,
						schema:   schema,
						table:    table,
						sql:      sql,
						columns:  res.Columns,
						rows:     res.Rows,
						elapsed:  res.Elapsed,
						err:      res.Err,
					}
				})
			}
		}
	}
	return m, cmds
}

// handleSidebarAction converts sidebar pending actions to app-level operations.
func (m Model) handleSidebarAction(cmds []tea.Cmd) (Model, []tea.Cmd) {
	action := m.sidebar.TakeAction()
	switch action.Kind {
	case sidebar.ActionConnect:
		connName := action.ConnName
		conn := m.connConfigByName(connName)
		if conn != nil {
			password, err := m.secrets.GetPassword(connName)
			if err != nil {
				m.modal = modalErr("No password found",
					fmt.Sprintf("No stored password for %q.\nDelete and re-add the connection.", connName),
					m.styles, m.width, m.height)
			} else {
				m.sidebar.SetConnState(connName, types.StateConnecting)
				cmds = append(cmds, m.connectCmd(*conn, password))
			}
		}

	case sidebar.ActionNewConn:
		cf := components.NewConnForm(m.styles)
		cf.SetSize(m.width, m.height)
		m.connform = &cf
		m.connformTestFired = false

	case sidebar.ActionDeleteConn:
		connName := action.ConnName
		m.pendingDeleteConn = connName
		mo := components.NewModal(
			"Delete Connection",
			fmt.Sprintf("Delete %q? This cannot be undone.", connName),
			[]components.ModalButton{
				{Label: "Cancel"},
				{Label: "Delete", IsDanger: true},
			},
			m.styles,
		)
		mo.SetSize(m.width, m.height)
		m.modal = &mo

	case sidebar.ActionOpenTable:
		connName := action.ConnName
		dbName := action.DBName
		schema := action.SchemaName
		table := action.TableName
		queryConn := connName
		if dbName != "" {
			queryConn = db.SubConnKey(connName, dbName)
		}
		m.lastQueryConn = queryConn
		m.lastQuerySchema = schema
		m.lastQueryTable = table
		m.editor.SetActiveConn(connName, schema)
		m.statusbar.SetConn(connName, schema)
		m = m.setFocus(PanelEditor)
		driver := ""
		if snap := m.dbMgr.Get(queryConn); snap != nil {
			driver = snap.Config.Driver
		}
		sql := previewSQL(driver, schema, table)
		m.lastSelectSQL = sql
		m.results.SetQuerying(true)
		m.statusbar.SetQuerying(true)
		cmds = append(cmds, func() tea.Msg {
			res := m.dbMgr.RunQuery(queryConn, sql)
			return queryResultMsg{
				columns: res.Columns,
				rows:    res.Rows,
				elapsed: res.Elapsed,
				err:     res.Err,
			}
		})

	case sidebar.ActionExpandDatabase:
		connName := action.ConnName
		dbName := action.DBName
		subKey := db.SubConnKey(connName, dbName)
		m.sidebar.SetDatabaseLoading(connName, dbName, true)

		// Open a sub-connection to that specific database if not already open.
		if m.dbMgr.Get(subKey) == nil {
			snap := m.dbMgr.Get(connName)
			if snap != nil {
				subConn := snap.Config
				subConn.Name = subKey // manager uses Name as the map key
				subConn.Database = dbName
				password, _ := m.secrets.GetPassword(connName)
				dbMgr := m.dbMgr
				driver := snap.Config.Driver
				cmds = append(cmds, func() tea.Msg {
					if err := dbMgr.Connect(subConn, password); err != nil {
						return schemaLoadedMsg{connName: connName, driver: driver, dbName: dbName, schemas: nil}
					}
					res := dbMgr.RunQuery(subKey, introspectSQL(driver))
					if res.Err != nil {
						return schemaLoadedMsg{connName: connName, driver: driver, dbName: dbName, schemas: nil}
					}
					return schemaLoadedMsg{
						connName: connName,
						dbName:   dbName,
						schemas:  parseSchemaRows(res.Columns, res.Rows),
					}
				})
			}
		} else {
			// Sub-connection already open — just reload schemas.
			snap := m.dbMgr.Get(connName)
			if snap != nil {
				driver := snap.Config.Driver
				dbMgr := m.dbMgr
				cmds = append(cmds, func() tea.Msg {
					res := dbMgr.RunQuery(subKey, introspectSQL(driver))
					if res.Err != nil {
						return schemaLoadedMsg{connName: connName, driver: driver, dbName: dbName, schemas: nil}
					}
					return schemaLoadedMsg{
						connName: connName,
						driver:   driver,
						dbName:   dbName,
						schemas:  parseSchemaRows(res.Columns, res.Rows),
					}
				})
			}
		}

	case sidebar.ActionEditConn:
		conn := m.connConfigByName(action.ConnName)
		if conn != nil {
			cf := components.NewConnFormEdit(*conn, m.styles)
			cf.SetSize(m.width, m.height)
			m.connform = &cf
			m.connformTestFired = false
		}

	case sidebar.ActionDuplicateConn:
		conn := m.connConfigByName(action.ConnName)
		if conn != nil {
			cf := components.NewConnFormDuplicate(*conn, m.styles)
			cf.SetSize(m.width, m.height)
			m.connform = &cf
			m.connformTestFired = false
		}

	case sidebar.ActionCreateDatabase:
		m.pendingDeleteConn = "create\x00" + action.ConnName
		im := components.NewInputModal(
			styles.IconNew+"  Create Database — "+action.ConnName,
			"new_database_name",
			m.styles,
		)
		im.SetSize(m.width, m.height)
		m.inputModal = &im

	case sidebar.ActionDropDatabase:
		connName := action.ConnName
		dbName := action.DBName
		m.pendingDeleteConn = "drop\x00" + connName + "\x00" + dbName
		mo := components.NewModal(
			"Drop Database",
			fmt.Sprintf("Drop %q on %q?\nThis permanently deletes ALL data.", dbName, connName),
			[]components.ModalButton{
				{Label: "Cancel"},
				{Label: "Drop", IsDanger: true},
			},
			m.styles,
		)
		mo.SetSize(m.width, m.height)
		m.modal = &mo
	}
	return m, cmds
}

// handleModalAction processes a button press in the active modal.
func (m Model) handleModalAction(label string) (Model, []tea.Cmd) {
	var cmds []tea.Cmd
	switch label {
	case "Delete":
		connName := m.pendingDeleteConn
		m.pendingDeleteConn = ""
		if connName != "" {
			if err := m.dbMgr.Disconnect(connName); err != nil {
				m.statusbar.ShowMessage("disconnect error: " + err.Error())
			}
			filtered := m.cfg.Connections[:0]
			for _, c := range m.cfg.Connections {
				if c.Name != connName {
					filtered = append(filtered, c)
				}
			}
			m.cfg.Connections = filtered
			if err := m.cfg.Save(); err != nil {
				m.statusbar.ShowMessage("config save error: " + err.Error())
			}
			m.secrets.DeletePassword(connName)
			if err := m.secrets.Save(); err != nil {
				m.statusbar.ShowMessage("secrets save error: " + err.Error())
			}
			m.sidebar.RemoveConnection(connName)
			m.settings.SetConfig(m.cfg)
		}

	case "Drop":
		parts := strings.SplitN(m.pendingDeleteConn, "\x00", 3)
		m.pendingDeleteConn = ""
		if len(parts) == 3 && parts[0] == "drop" {
			connName, dbName := parts[1], parts[2]
			snap := m.dbMgr.Get(connName)
			if snap != nil {
				driver := snap.Config.Driver
				dbMgr := m.dbMgr
				cmds = append(cmds, func() tea.Msg {
					res := dbMgr.RunQuery(connName, dropDatabaseSQL(driver, dbName))
					msg := fmt.Sprintf("dropped database %q", dbName)
					if res.Err != nil {
						msg = res.Err.Error()
					}
					return dbOperationMsg{connName: connName, driver: driver, err: res.Err, message: msg}
				})
				m.modal = nil
				return m, cmds
			}
		}
	}
	m.modal = nil
	return m, nil
}

// handleInputModalAction processes the result of an InputModal prompt.
func (m Model) handleInputModalAction(a *components.InputModalAction, cmds []tea.Cmd) (Model, []tea.Cmd) {
	m.inputModal = nil
	pending := m.pendingDeleteConn
	m.pendingDeleteConn = ""

	if a.Cancelled {
		return m, cmds
	}

	parts := strings.SplitN(pending, "\x00", 2)
	if len(parts) == 2 && parts[0] == "create" {
		if a.Value == "" {
			return m, cmds
		}
		connName := parts[1]
		dbName := a.Value
		snap := m.dbMgr.Get(connName)
		if snap != nil {
			driver := snap.Config.Driver
			dbMgr := m.dbMgr
			cmds = append(cmds, func() tea.Msg {
				res := dbMgr.RunQuery(connName, createDatabaseSQL(driver, dbName))
				msg := fmt.Sprintf("created database %q", dbName)
				if res.Err != nil {
					msg = res.Err.Error()
				}
				return dbOperationMsg{connName: connName, driver: driver, err: res.Err, message: msg}
			})
		}
		return m, cmds
	}

	// editcell\x00connName\x00tableName\x00row\x00col\x00columns\x00rowvals
	// stored as editcell context; actual columns/vals come from the saved action
	editParts := strings.SplitN(pending, "\x00", 3)
	if len(editParts) >= 1 && editParts[0] == "editcell" && m.pendingEditAction != nil {
		ea := m.pendingEditAction
		m.pendingEditAction = nil
		newVal := a.Value
		connName := m.lastQueryConn
		snap := m.dbMgr.Get(connName)
		driver := ""
		if snap != nil {
			driver = snap.Config.Driver
		}
		sql := buildUpdateSQL(driver, m.lastQueryTable, ea.Column, newVal, ea.Columns, ea.RowVals)
		m.pendingCursorRow = ea.Row
		m.pendingCursorCol = ea.Col
		dbMgr := m.dbMgr
		cmds = append(cmds, func() tea.Msg {
			res := dbMgr.RunQuery(connName, sql)
			return updateCellResultMsg{err: res.Err}
		})
		return m, cmds
	}

	// search\x00columnName
	searchParts := strings.SplitN(pending, "\x00", 2)
	if len(searchParts) == 2 && searchParts[0] == "search" {
		column := searchParts[1]
		connName := m.lastQueryConn
		snap := m.dbMgr.Get(connName)
		driver := ""
		if snap != nil {
			driver = snap.Config.Driver
		}

		var sql string
		if a.Value == "" {
			sql = stripWhere(m.lastSelectSQL)
		} else {
			sql = injectWhere(driver, m.lastSelectSQL, column, a.Value)
		}
		m.lastSelectSQL = sql
		m.pendingCursorRow, m.pendingCursorCol = m.results.Cursor()
		m.results.SetQuerying(true)
		m.statusbar.SetQuerying(true)
		dbMgr := m.dbMgr
		cmds = append(cmds, func() tea.Msg {
			res := dbMgr.RunQuery(connName, sql)
			return queryResultMsg{columns: res.Columns, rows: res.Rows, elapsed: res.Elapsed, err: res.Err}
		})
		return m, cmds
	}

	return m, cmds
}

// createDatabaseSQL returns the SQL to create a database for the given driver.
func createDatabaseSQL(driver, name string) string {
	switch driver {
	case "postgres":
		return fmt.Sprintf("CREATE DATABASE %q", name)
	case "mysql":
		return fmt.Sprintf("CREATE DATABASE `%s`", name)
	case "sqlserver":
		return fmt.Sprintf("CREATE DATABASE [%s]", name)
	default:
		return fmt.Sprintf("CREATE DATABASE %q", name)
	}
}

// dropDatabaseSQL returns the SQL to drop a database for the given driver.
func dropDatabaseSQL(driver, name string) string {
	switch driver {
	case "postgres":
		return fmt.Sprintf("DROP DATABASE %q", name)
	case "mysql":
		return fmt.Sprintf("DROP DATABASE `%s`", name)
	case "sqlserver":
		return fmt.Sprintf("DROP DATABASE [%s]", name)
	default:
		return fmt.Sprintf("DROP DATABASE %q", name)
	}
}

// buildUpdateSQL constructs a complete UPDATE statement with a WHERE clause
// built from all original column values, making it safe to run immediately.
// Single quotes in values are escaped as ” per the SQL standard.
// Values that look like Go's time.Time string (e.g. "2026-05-29 12:17:57 +0000 UTC")
// are normalised to the driver's native timestamp literal format so the implicit
// cast in the WHERE condition matches the stored value.
func buildUpdateSQL(driver, table, editCol, newValue string, columns, rowVals []string) string {
	escape := func(v string) string { return strings.ReplaceAll(v, "'", "''") }

	var quoteIdent func(string) string
	var castToText func(string) string
	var normTime func(time.Time) string

	switch driver {
	case "mysql":
		quoteIdent = func(s string) string { return "`" + s + "`" }
		castToText = func(s string) string { return "CAST(`" + s + "` AS CHAR)" }
		normTime = func(t time.Time) string { return t.UTC().Format("2006-01-02 15:04:05.999999") }
	case "sqlserver":
		quoteIdent = func(s string) string { return "[" + s + "]" }
		castToText = func(s string) string { return "CAST([" + s + "] AS NVARCHAR(MAX))" }
		normTime = func(t time.Time) string { return t.UTC().Format("2006-01-02 15:04:05.9999999") }
	default:
		quoteIdent = func(s string) string { return `"` + s + `"` }
		castToText = func(s string) string { return `CAST("` + s + `" AS TEXT)` }
		normTime = func(t time.Time) string { return t.UTC().Format("2006-01-02 15:04:05.999999+00") }
	}

	whereVal := func(col, raw string) string {
		if raw == "NULL" || raw == "<nil>" || raw == "" {
			return fmt.Sprintf("  %s IS NULL", quoteIdent(col))
		}
		if t, ok := parseGoTime(raw); ok {
			return fmt.Sprintf("  %s = '%s'", quoteIdent(col), normTime(t))
		}
		return fmt.Sprintf("  %s = '%s'", castToText(col), escape(raw))
	}

	setClause := fmt.Sprintf("  %s = '%s'", quoteIdent(editCol), escape(newValue))

	var whereParts []string
	for i, col := range columns {
		val := ""
		if i < len(rowVals) {
			val = rowVals[i]
		}
		whereParts = append(whereParts, whereVal(col, val))
	}

	return fmt.Sprintf("UPDATE %s\nSET\n%s\nWHERE\n%s\n;",
		quoteIdent(table), setClause, strings.Join(whereParts, "\n  AND "))
}

// goTimeLayouts are the formats produced by Go's time.Time.String() and
// time.Time.Format with varying fractional-second precision.
var goTimeLayouts = []string{
	"2006-01-02 15:04:05.999999999 -0700 MST",
	"2006-01-02 15:04:05.999999 -0700 MST",
	"2006-01-02 15:04:05.999 -0700 MST",
	"2006-01-02 15:04:05 -0700 MST",
}

// parseGoTime attempts to parse s as a Go time.Time string.
// Returns the parsed time and true on success.
func parseGoTime(s string) (time.Time, bool) {
	for _, layout := range goTimeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// injectOrderBy strips any existing ORDER BY clause from sql and appends
// a new one for the given column and direction.
// It is driver-aware: PostgreSQL and SQL Server use double-quote identifiers,
// MySQL uses backticks.
// injectOrderBy strips any existing ORDER BY clause from sql and appends
// a new one for the given column and direction.
// LIMIT/OFFSET clauses are preserved and re-appended after ORDER BY so the
// generated SQL is valid (ORDER BY must precede LIMIT in SQL).
func injectOrderBy(driver, sql, column string, asc bool) string {
	dir := "ASC"
	if !asc {
		dir = "DESC"
	}

	var quotedCol string
	switch driver {
	case "mysql":
		quotedCol = "`" + strings.ReplaceAll(column, "`", "``") + "`"
	case "sqlserver":
		quotedCol = "[" + strings.ReplaceAll(column, "]", "]]") + "]"
	default:
		quotedCol = `"` + strings.ReplaceAll(column, `"`, `""`) + `"`
	}

	trimmed := strings.TrimRight(strings.TrimSpace(sql), ";")

	// Extract trailing LIMIT/OFFSET from the original string first, before
	// removing ORDER BY, because ORDER BY may appear between SELECT and LIMIT
	// (e.g. "SELECT … ORDER BY old LIMIT 200") and would take LIMIT with it.
	tail := ""
	if idx := indexCaseInsensitive(trimmed, "LIMIT"); idx >= 0 {
		tail = " " + strings.TrimSpace(trimmed[idx:])
		trimmed = strings.TrimRight(trimmed[:idx], " \t\n")
	}

	// Now remove any existing ORDER BY (which is safe because LIMIT is gone).
	if idx := indexCaseInsensitive(trimmed, "ORDER BY"); idx >= 0 {
		trimmed = strings.TrimRight(trimmed[:idx], " \t\n")
	}

	return trimmed + "\nORDER BY " + quotedCol + " " + dir + tail + ";"
}

// injectWhere adds (or replaces) a single-column predicate in sql.
// When val contains '%' the operator is LIKE, otherwise '='.
// Existing WHERE conditions are replaced so repeated searches on the same
// column don't accumulate. ORDER BY and LIMIT/OFFSET are preserved in the
// correct position.
func injectWhere(driver, sql, column, val string) string {
	escaped := strings.ReplaceAll(val, "'", "''")

	var quoteIdent func(string) string
	switch driver {
	case "mysql":
		quoteIdent = func(s string) string { return "`" + strings.ReplaceAll(s, "`", "``") + "`" }
	case "sqlserver":
		quoteIdent = func(s string) string { return "[" + strings.ReplaceAll(s, "]", "]]") + "]" }
	default:
		quoteIdent = func(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
	}

	op := "="
	if strings.Contains(val, "%") {
		op = "LIKE"
	}
	predicate := fmt.Sprintf("%s %s '%s'", quoteIdent(column), op, escaped)

	trimmed := strings.TrimRight(strings.TrimSpace(sql), ";")

	// Extract LIMIT/OFFSET tail.
	tail := ""
	if idx := indexCaseInsensitive(trimmed, "LIMIT"); idx >= 0 {
		tail = " " + strings.TrimSpace(trimmed[idx:])
		trimmed = strings.TrimRight(trimmed[:idx], " \t\n")
	}

	// Extract ORDER BY.
	orderBy := ""
	if idx := indexCaseInsensitive(trimmed, "ORDER BY"); idx >= 0 {
		orderBy = "\n" + strings.TrimSpace(trimmed[idx:])
		trimmed = strings.TrimRight(trimmed[:idx], " \t\n")
	}

	// Remove existing WHERE so we always replace with the new predicate.
	if idx := indexCaseInsensitive(trimmed, "WHERE"); idx >= 0 {
		trimmed = strings.TrimRight(trimmed[:idx], " \t\n")
	}

	return trimmed + "\nWHERE " + predicate + orderBy + tail + ";"
}

// stripWhere removes the WHERE clause from sql, preserving ORDER BY and
// LIMIT/OFFSET. Used when the user confirms an empty search to clear filters.
func stripWhere(sql string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(sql), ";")

	tail := ""
	if idx := indexCaseInsensitive(trimmed, "LIMIT"); idx >= 0 {
		tail = " " + strings.TrimSpace(trimmed[idx:])
		trimmed = strings.TrimRight(trimmed[:idx], " \t\n")
	}

	orderBy := ""
	if idx := indexCaseInsensitive(trimmed, "ORDER BY"); idx >= 0 {
		orderBy = "\n" + strings.TrimSpace(trimmed[idx:])
		trimmed = strings.TrimRight(trimmed[:idx], " \t\n")
	}

	if idx := indexCaseInsensitive(trimmed, "WHERE"); idx >= 0 {
		trimmed = strings.TrimRight(trimmed[:idx], " \t\n")
	}

	return trimmed + orderBy + tail + ";"
}

// indexCaseInsensitive returns the index of substr in s, case-insensitively,
// or -1 if not found. Only the last occurrence is relevant here, so we scan
// from the end to avoid false matches inside string literals.
func indexCaseInsensitive(s, substr string) int {
	lower := strings.ToLower(s)
	lowerSub := strings.ToLower(substr)
	return strings.LastIndex(lower, lowerSub)
}

// openExternalEditorCmd writes the current editor SQL to a temp file and
// returns a tea.ExecProcess command that opens the configured external editor.
// After the editor exits, externalEditorClosedMsg is fired and the content
// is read back into the SQL editor.
// Priority for the editor binary: Settings.ExternalEditor → $EDITOR → $VISUAL → "vi".
func (m Model) openExternalEditorCmd() tea.Cmd {
	editorBin := m.cfg.Settings.ExternalEditor
	if editorBin == "" {
		editorBin = os.Getenv("EDITOR")
	}
	if editorBin == "" {
		editorBin = os.Getenv("VISUAL")
	}
	if editorBin == "" {
		editorBin = "vi"
	}

	_, err := exec.LookPath(editorBin)
	if err != nil {
		m.statusbar.ShowMessage("editor not found: " + editorBin)
		return nil
	}

	tmp, err := os.CreateTemp("", "dbterm-*.sql")
	if err != nil {
		m.statusbar.ShowMessage("could not create temp file: " + err.Error())
		return nil
	}
	if _, err := tmp.WriteString(m.editor.Value()); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		m.statusbar.ShowMessage("could not write temp file: " + err.Error())
		return nil
	}
	tmp.Close()

	tmpFile := tmp.Name()
	c := exec.Command(editorBin, tmpFile)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return externalEditorClosedMsg{tmpFile: tmpFile, err: err}
	})
}

// maybeFireConnTest fires the test-connection cmd when the connform first
// reaches stepTesting and we haven't fired it yet this session.
func (m Model) maybeFireConnTest(cmds []tea.Cmd) (Model, []tea.Cmd) {
	if m.connform == nil || m.connformTestFired {
		return m, cmds
	}
	if !m.connform.IsTesting() {
		return m, cmds
	}
	m.connformTestFired = true
	conn := m.connform.PendingConn()
	password := m.connform.PendingPassword()
	cmds = append(cmds, func() tea.Msg {
		if err := m.dbMgr.Connect(conn, password); err != nil {
			return testConnMsg{err: err}
		}
		return testConnMsg{err: nil}
	})
	return m, cmds
}

// connectCmd fires an async db connection and returns the result as a tea.Msg.
// The caller is responsible for setting StateConnecting on the sidebar before
// appending this cmd to the batch (connectCmd is a value receiver so any
// sidebar mutation here would be silently discarded).
func (m Model) connectCmd(conn config.Connection, password string) tea.Cmd {
	return func() tea.Msg {
		if err := m.dbMgr.Connect(conn, password); err != nil {
			return connectErrMsg{connName: conn.Name, err: err}
		}
		return connectedMsg{connName: conn.Name}
	}
}

// applyTheme updates all styles when the user changes the theme.
func (m Model) applyTheme(themeID string) Model {
	m.cfg.Settings.Theme = themeID
	if err := m.cfg.Save(); err != nil {
		m.statusbar.ShowMessage("config save error: " + err.Error())
	}
	s := styles.ToStyles(styles.Resolve(themeID, m.cfg.Theme.Custom))
	m.styles = s
	m.sidebar.SetStyles(s)
	m.editor.SetStyles(s)
	m.editor.SetSettings(m.cfg.Settings)
	m.results.SetStyles(s)
	m.settings.SetStyles(s)
	m.statusbar.SetStyles(s)
	if m.modal != nil {
		m.modal.SetStyles(s)
	}
	if m.inputModal != nil {
		m.inputModal.SetStyles(s)
	}
	return m
}

// setFocus moves keyboard focus to the given panel.
func (m Model) setFocus(p PanelID) Model {
	m.sidebar.Blur()
	m.editor.Blur()
	m.results.Blur()
	m.activePanel = p
	switch p {
	case PanelSidebar:
		m.sidebar.Focus()
		m.statusbar.SetHints(m.sidebarHints())
	case PanelEditor:
		m.editor.Focus()
		m.statusbar.SetHints(m.editorHints())
	case PanelResults:
		m.results.Focus()
		m.statusbar.SetHints(m.resultsHints())
	}
	return m
}

// sidebarHints returns the footer hint string for the sidebar panel.
// sidebarHints returns context-aware footer hints based on the focused node type.
func (m Model) sidebarHints() string {
	kb := m.cfg.Keybinds
	resize := "  [" + kb.ResizePanelLeft + "/" + kb.ResizePanelRight + "] Resize"
	node := m.sidebar.CurrentFocusedNode()
	if node == nil {
		return "[" + kb.NewConnection + "] New connection  [Tab] Switch  [" + kb.OpenSettings + "] Settings" + resize
	}
	switch node.Kind {
	case sidebar.NodeConnection:
		if node.State == types.StateConnected || len(node.Children) > 0 {
			return "[Enter] Expand  [e] Edit  [" + kb.NewConnection + "] New DB  [" + kb.DeleteConnection + "] Delete  [Tab] Switch" + resize
		}
		return "[Enter] Connect  [e] Edit  [" + kb.DeleteConnection + "] Delete  [" + kb.NewConnection + "] New conn  [Tab] Switch" + resize
	case sidebar.NodeDatabase:
		return "[Enter] Load  [" + kb.DeleteConnection + "] Drop DB  [Tab] Switch" + resize
	case sidebar.NodeTable, sidebar.NodeView:
		return "[Enter] Query table  [Tab] Switch" + resize
	default:
		return "[↑↓] Move  [Enter] Open  [" + kb.FilterSidebar + "] Filter  [Tab] Switch" + resize
	}
}

// editorHints returns the footer hint string for the editor panel.
func (m Model) editorHints() string {
	kb := m.cfg.Keybinds
	return "[" + kb.RunQuery + "] Run  [" + kb.CancelQuery + "] Cancel  " +
		"[" + kb.OpenExternalEditor + "] Open in " + m.externalEditorName() + "  " +
		"[Tab] Switch  [" + kb.OpenSettings + "] Settings  [?] Help  " +
		"[" + kb.ResizePanelUp + "/" + kb.ResizePanelDown + "] Resize editor"
}

// externalEditorName returns the display name of the configured external editor.
func (m Model) externalEditorName() string {
	if m.cfg.Settings.ExternalEditor != "" {
		return m.cfg.Settings.ExternalEditor
	}
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	if e := os.Getenv("VISUAL"); e != "" {
		return e
	}
	return "vi"
}

// resultsHints returns the footer hint string for the results panel.
func (m Model) resultsHints() string {
	kb := m.cfg.Keybinds
	return "[↑↓←→] Navigate  [e] Edit  [s] Sort  [" + kb.CopyCell + "] Copy  " +
		"[" + kb.FollowFK + "] Follow FK  " +
		"[" + kb.NextPage + "] Next  [" + kb.PrevPage + "] Prev  " +
		"[" + kb.ResizePanelUp + "/" + kb.ResizePanelDown + "] Resize  [Tab] Switch"
}

// panelCount is the total number of focusable panels.
// Used by cyclePanel to avoid the magic literal % 3.
const panelCount = 3

// tablePreviewLimit is the maximum row count for SELECT * table preview queries.
const tablePreviewLimit = 200

// previewSQL builds a driver-appropriate SELECT * preview query for the given
// schema and table, using the correct row-limit syntax and identifier quoting
// for each database engine.
func previewSQL(driver, schema, table string) string {
	switch driver {
	case "sqlserver":
		s := strings.ReplaceAll(schema, "]", "]]")
		t := strings.ReplaceAll(table, "]", "]]")
		return fmt.Sprintf("SELECT TOP %d * FROM [%s].[%s];", tablePreviewLimit, s, t)
	case "mysql":
		s := strings.ReplaceAll(schema, "`", "``")
		t := strings.ReplaceAll(table, "`", "``")
		return fmt.Sprintf("SELECT * FROM `%s`.`%s` LIMIT %d;", s, t, tablePreviewLimit)
	default:
		s := strings.ReplaceAll(schema, `"`, `""`)
		t := strings.ReplaceAll(table, `"`, `""`)
		return fmt.Sprintf(`SELECT * FROM "%s"."%s" LIMIT %d;`, s, t, tablePreviewLimit)
	}
}

func foreignKeySQL(driver string, fk types.ForeignKey, value string) string {
	return injectWhere(driver, previewSQL(driver, fk.Schema, fk.Table), fk.Column, value)
}

// cyclePanel moves focus forward (+1) or backward (-1) through the panels.
func (m Model) cyclePanel(dir int) Model {
	next := PanelID((int(m.activePanel) + dir + panelCount) % panelCount)
	return m.setFocus(next)
}

// resize recalculates and propagates panel dimensions after a window resize.
func (m Model) resize() Model {
	if m.width == 0 || m.height == 0 {
		return m
	}
	d := layout.Compute(m.width, m.height, m.sidebarVisible, m.sidebarW, m.editorRatio)

	m.sidebar.SetSize(d.SidebarW, d.SidebarH)
	m.editor.SetSize(d.RightW, d.EditorH)
	m.results.SetSize(d.RightW, d.ResultsH)
	m.statusbar.SetWidth(d.StatusW)
	m.settings.SetSize(m.width, m.height)

	if m.modal != nil {
		m.modal.SetSize(m.width, m.height)
	}
	if m.connform != nil {
		m.connform.SetSize(m.width, m.height)
	}
	if m.inputModal != nil {
		m.inputModal.SetSize(m.width, m.height)
	}
	return m
}

// View renders the full TUI.
func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	if m.width < 80 || m.height < 24 {
		return m.styles.Muted.Render(
			"\n\n  Please resize your terminal to at least 80×24.\n" +
				fmt.Sprintf("  Current size: %d×%d\n", m.width, m.height),
		)
	}

	if m.modal != nil {
		return m.modal.View()
	}
	if m.connform != nil {
		return m.connform.View()
	}
	if m.inputModal != nil {
		return m.inputModal.View()
	}
	if m.showSettings {
		return m.settings.View()
	}
	if m.showHelp {
		return m.viewHelp()
	}

	d := layout.Compute(m.width, m.height, m.sidebarVisible, m.sidebarW, m.editorRatio)
	return layout.Render(
		m.sidebar.View(),
		m.editor.View(),
		m.results.View(),
		m.statusbar.View(),
		d,
		"",
	)
}

// viewHelp renders a two-column shortcut reference as a centred overlay.
func (m Model) viewHelp() string {
	type row struct{ action, key string }
	shortcuts := []row{
		{"Run query", m.cfg.Keybinds.RunQuery},
		{"Cancel query", m.cfg.Keybinds.CancelQuery},
		{"New connection", m.cfg.Keybinds.NewConnection},
		{"Delete connection", m.cfg.Keybinds.DeleteConnection},
		{"Edit connection", "e"},
		{"Duplicate connection", "y"},
		{"Toggle sidebar", m.cfg.Keybinds.ToggleSidebar},
		{"Focus editor", m.cfg.Keybinds.FocusEditor},
		{"Focus results", m.cfg.Keybinds.FocusResults},
		{"Open settings", m.cfg.Keybinds.OpenSettings},
		{"Filter sidebar", m.cfg.Keybinds.FilterSidebar},
		{"Copy cell", m.cfg.Keybinds.CopyCell},
		{"Follow foreign key", m.cfg.Keybinds.FollowFK},
		{"Next page", m.cfg.Keybinds.NextPage},
		{"Prev page", m.cfg.Keybinds.PrevPage},
		{"Sidebar narrower", m.cfg.Keybinds.ResizePanelLeft},
		{"Sidebar wider", m.cfg.Keybinds.ResizePanelRight},
		{"Editor shorter", m.cfg.Keybinds.ResizePanelUp},
		{"Editor taller", m.cfg.Keybinds.ResizePanelDown},
		{"Help", "?"},
		{"Quit", m.cfg.Keybinds.Quit},
	}

	const sepLen = 44
	title := m.styles.Title.Render(styles.IconHelp + "  Keyboard Shortcuts")
	sep := m.styles.Muted.Render(strings.Repeat("─", sepLen))

	lines := []string{title, sep, ""}
	for _, s := range shortcuts {
		action := m.styles.Subtext.Render(fmt.Sprintf("  %-22s", s.action))
		key := m.styles.Title.Render(s.key)
		lines = append(lines, action+key)
	}
	lines = append(lines, "", m.styles.Muted.Render("  Press [?] to close"))

	inner := strings.Join(lines, "\n")

	const boxW = 52
	box := m.styles.PanelFocused.
		Width(boxW-2).
		Padding(0, 1).
		Render(inner)

	topPad := (m.height - lipgloss.Height(box)) / 2
	if topPad < 0 {
		topPad = 0
	}
	leftPad := (m.width - lipgloss.Width(box)) / 2
	if leftPad < 0 {
		leftPad = 0
	}

	pad := strings.Repeat(" ", leftPad)
	return strings.Repeat("\n", topPad) +
		pad + strings.ReplaceAll(box, "\n", "\n"+pad)
}

// connConfigByName returns the config.Connection for a given name, or nil.
func (m Model) connConfigByName(name string) *config.Connection {
	for i := range m.cfg.Connections {
		if m.cfg.Connections[i].Name == name {
			return &m.cfg.Connections[i]
		}
	}
	return nil
}

// isPlainKey reports whether key is a single printable character with no
// modifier prefix. These keys must not fire as global shortcuts when the
// editor panel is focused because the user is typing SQL.
func isPlainKey(key string) bool {
	// Multi-char strings starting with "ctrl+", "alt+", "shift+", "f1"… are
	// modified or special keys — safe to fire as globals from any panel.
	if len(key) != 1 {
		return false
	}
	r := rune(key[0])
	return r >= 0x20 && r <= 0x7e // printable ASCII
}

func keepaliveTick() tea.Cmd {
	return tea.Tick(db.PingInterval, func(_ time.Time) tea.Msg {
		return keepaliveTickMsg{}
	})
}

func modalErr(title, body string, s styles.Styles, w, h int) *components.Modal {
	mo := components.NewModal(title, body,
		[]components.ModalButton{{Label: "OK"}},
		s,
	)
	mo.SetSize(w, h)
	return &mo
}

// introspectSQL returns a driver-specific query that returns
// table_schema, table_name, table_type rows from the information schema.
// We reuse db.RunQuery (which takes plain SQL) for introspection to avoid
// importing the gorm.DB directly from the app layer.
func introspectSQL(driver string) string {
	switch driver {
	case "postgres":
		return `SELECT table_schema, table_name, table_type
			FROM information_schema.tables
			WHERE table_schema NOT IN ('pg_catalog','information_schema')
			ORDER BY table_schema, table_name`
	case "mysql":
		return `SELECT table_schema, table_name, table_type
			FROM information_schema.tables
			WHERE table_schema = DATABASE()
			ORDER BY table_name`
	case "sqlserver":
		return `SELECT s.name AS table_schema, t.name AS table_name, 'BASE TABLE' AS table_type
			FROM sys.tables t JOIN sys.schemas s ON t.schema_id = s.schema_id
			ORDER BY s.name, t.name`
	}
	return "SELECT 1 WHERE 1=0" // no-op for unknown drivers
}

// parseSchemaRows converts the flat RunQuery output into []types.Schema.
func parseSchemaRows(cols []string, rows [][]string) []types.Schema {
	schemaIdx, nameIdx, typeIdx := 0, 1, 2
	for i, c := range cols {
		switch strings.ToLower(c) {
		case "table_schema":
			schemaIdx = i
		case "table_name":
			nameIdx = i
		case "table_type":
			typeIdx = i
		}
	}

	index := map[string]int{}
	var schemas []types.Schema
	for _, row := range rows {
		if len(row) <= typeIdx {
			continue
		}
		schName := row[schemaIdx]
		tblName := row[nameIdx]
		tblType := row[typeIdx]

		pos, exists := index[schName]
		if !exists {
			pos = len(schemas)
			index[schName] = pos
			schemas = append(schemas, types.Schema{Name: schName})
		}
		schemas[pos].Tables = append(schemas[pos].Tables, types.Table{
			Name:   tblName,
			Schema: schName,
			IsView: tblType == "VIEW",
		})
	}
	return schemas
}
