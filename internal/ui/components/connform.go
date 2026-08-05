package components

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gabrielfloresousion/db-term/internal/config"
	"github.com/gabrielfloresousion/db-term/internal/ui/styles"
)

// connFormStep identifies which step the connection form is on.
type connFormStep int

const (
	stepDriverPicker connFormStep = iota
	stepActivation
	stepFields
	stepTesting
)

// ConnFormAction is produced when the user completes or cancels the form.
type ConnFormAction struct {
	Cancelled    bool
	IsEdit       bool   // true when editing an existing connection (vs creating)
	OriginalName string // original connection name before editing
	Connection   config.Connection
	Password     string
}

// fieldIndex names the index of each text input in the fields step.
const (
	fieldName     = 0
	fieldHost     = 1
	fieldPort     = 2
	fieldUser     = 3
	fieldDatabase = 4
	fieldPassword = 5
	fieldSSLMode  = 6
	fieldCount    = 7
)

// ConnForm is the multi-step connection creation / edit form.
//
// validationError holds an inline error message shown at the bottom of
// the fields step when required fields are empty. It is cleared on the
// next keystroke so the user gets immediate feedback without a modal.
// Steps:
//
//	0 — driver picker  (postgres / mysql / sqlserver)
//	1 — activation     (shown once per driver type per session)
//	2 — fields         (name, host, port, user, db, password, sslmode)
//	3 — testing        (spinner while connecting, then success/error)
//
// The parent receives the result via TakeAction() after Update().
type ConnForm struct {
	step            connFormStep
	driverCursor    int // index into drivers slice (step 0)
	inputs          []textinput.Model
	focusIdx        int
	testingErr      error
	testingDone     bool
	activatedFor    map[string]bool // drivers shown activation modal this session
	validationError string          // inline error shown when required fields are empty

	// isEdit indicates this form is editing an existing connection. originalName
	// preserves the pre-edit name so the caller can update the correct config entry.
	isEdit       bool
	originalName string

	styles styles.Styles
	width  int
	height int

	lastAction *ConnFormAction
}

var drivers = []struct {
	id    string
	label string
	pkg   string
	port  int
	icon  string
}{
	{"postgres", "PostgreSQL", "gorm.io/driver/postgres (pgx/v5)", 5432, styles.IconPostgres},
	{"mysql", "MySQL", "gorm.io/driver/mysql", 3306, styles.IconMySQL},
	{"sqlserver", "SQL Server", "gorm.io/driver/sqlserver", 1433, styles.IconSQLServer},
}

// NewConnForm returns an initialised ConnForm for creating a new connection.
func NewConnForm(s styles.Styles) ConnForm {
	inputs := make([]textinput.Model, fieldCount)
	for i := range inputs {
		inputs[i] = textinput.New()
		inputs[i].CharLimit = 128
	}
	inputs[fieldName].Placeholder = "my-connection"
	inputs[fieldHost].Placeholder = "localhost"
	inputs[fieldPort].Placeholder = "5432"
	inputs[fieldUser].Placeholder = "admin"
	inputs[fieldDatabase].Placeholder = "optional — leave empty to browse all"
	inputs[fieldPassword].Placeholder = "••••••••"
	inputs[fieldPassword].EchoMode = textinput.EchoPassword
	inputs[fieldSSLMode].Placeholder = "disable"
	inputs[fieldName].Focus()

	return ConnForm{
		inputs:       inputs,
		activatedFor: map[string]bool{},
		styles:       s,
	}
}

// NewConnFormDuplicate returns a ConnForm pre-filled with the data of an existing
// connection so the user can save it as a new, independent copy. It behaves like
// NewConnFormEdit but treats the result as a brand-new connection (isEdit = false),
// so no original record is overwritten on confirmation.
// The Name field is pre-set to "<original> copy" to avoid a name collision.
func NewConnFormDuplicate(conn config.Connection, s styles.Styles) ConnForm {
	f := NewConnForm(s)

	for i, d := range drivers {
		if d.id == conn.Driver {
			f.driverCursor = i
			break
		}
	}
	f.activatedFor[conn.Driver] = true

	f.inputs[fieldName].SetValue(conn.Name + " copy")
	f.inputs[fieldHost].SetValue(conn.Host)
	f.inputs[fieldPort].SetValue(strconv.Itoa(conn.Port))
	f.inputs[fieldUser].SetValue(conn.User)
	f.inputs[fieldDatabase].SetValue(conn.Database)
	f.inputs[fieldSSLMode].SetValue(conn.SSLMode)

	f.step = stepFields
	f.focusIdx = fieldName
	f.inputs[fieldName].Focus()
	return f
}

// NewConnFormEdit returns a ConnForm pre-filled with the data of an existing
// connection for editing. It skips directly to the fields step so the user
// can update values without re-selecting the driver type.
func NewConnFormEdit(conn config.Connection, s styles.Styles) ConnForm {
	f := NewConnForm(s)
	f.isEdit = true
	f.originalName = conn.Name

	// Locate the driver index.
	for i, d := range drivers {
		if d.id == conn.Driver {
			f.driverCursor = i
			break
		}
	}
	f.activatedFor[conn.Driver] = true

	// Pre-fill all field values.
	f.inputs[fieldName].SetValue(conn.Name)
	f.inputs[fieldHost].SetValue(conn.Host)
	f.inputs[fieldPort].SetValue(strconv.Itoa(conn.Port))
	f.inputs[fieldUser].SetValue(conn.User)
	f.inputs[fieldDatabase].SetValue(conn.Database)
	f.inputs[fieldSSLMode].SetValue(conn.SSLMode)

	// Password field is left empty — the user re-enters it only if they want
	// to change it. The caller detects an empty password and keeps the old one.
	f.step = stepFields
	f.focusIdx = fieldName
	f.inputs[fieldName].Focus()
	return f
}

// SetStyles updates lipgloss styles.
func (f *ConnForm) SetStyles(s styles.Styles) { f.styles = s }

// SetSize sets the form dimensions.
func (f *ConnForm) SetSize(w, h int) { f.width = w; f.height = h }

// SetTestResult is called by the parent when the connection test completes.
// err is nil on success.
func (f *ConnForm) SetTestResult(err error) {
	f.testingErr = err
	f.testingDone = true
}

// IsTesting reports whether the form is currently on the testing step.
func (f *ConnForm) IsTesting() bool { return f.step == stepTesting && !f.testingDone }

// parsePort converts the port field value to an int.
// If the value is empty or invalid, it returns the driver's default port.
func (f *ConnForm) parsePort() int {
	port, err := strconv.Atoi(f.inputs[fieldPort].Value())
	if err != nil || port <= 0 || port > 65535 {
		return drivers[f.driverCursor].port // fall back to driver default
	}
	return port
}

// PendingConn returns the Connection built from the current field values.
// Only valid when IsTesting() is true.
func (f *ConnForm) PendingConn() config.Connection {
	return config.Connection{
		Name:     f.inputs[fieldName].Value(),
		Driver:   drivers[f.driverCursor].id,
		Host:     f.inputs[fieldHost].Value(),
		Port:     f.parsePort(),
		User:     f.inputs[fieldUser].Value(),
		Database: f.inputs[fieldDatabase].Value(),
		SSLMode:  f.inputs[fieldSSLMode].Value(),
	}
}

// PendingPassword returns the password from the current field values.
func (f *ConnForm) PendingPassword() string { return f.inputs[fieldPassword].Value() }

// TakeAction returns and clears the pending form action.
func (f *ConnForm) TakeAction() *ConnFormAction {
	a := f.lastAction
	f.lastAction = nil
	return a
}

// Update handles keyboard events for the active step.
func (f ConnForm) Update(msg tea.Msg) (ConnForm, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch f.step {
		case stepDriverPicker:
			return f.updateDriverPicker(msg)
		case stepActivation:
			return f.updateActivation(msg)
		case stepFields:
			return f.updateFields(msg)
		case stepTesting:
			return f.updateTesting(msg)
		}
	}
	return f, nil
}

func (f ConnForm) updateDriverPicker(msg tea.KeyMsg) (ConnForm, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if f.driverCursor > 0 {
			f.driverCursor--
		}
	case "down", "j":
		if f.driverCursor < len(drivers)-1 {
			f.driverCursor++
		}
	case "enter":
		selected := drivers[f.driverCursor]
		// Update port default for selected driver.
		f.inputs[fieldPort].SetValue(strconv.Itoa(selected.port))
		// Update SSLMode default.
		if selected.id == "postgres" {
			f.inputs[fieldSSLMode].SetValue("disable")
		} else {
			f.inputs[fieldSSLMode].SetValue("")
		}

		if f.activatedFor[selected.id] {
			f.step = stepFields
		} else {
			f.step = stepActivation
		}
	case "esc":
		f.lastAction = &ConnFormAction{Cancelled: true}
	}
	return f, nil
}

func (f ConnForm) updateActivation(msg tea.KeyMsg) (ConnForm, tea.Cmd) {
	switch msg.String() {
	case "enter":
		f.activatedFor[drivers[f.driverCursor].id] = true
		f.step = stepFields
	case "esc":
		f.step = stepDriverPicker
	}
	return f, nil
}

func (f ConnForm) updateFields(msg tea.KeyMsg) (ConnForm, tea.Cmd) {
	isNavKey := false

	switch msg.String() {
	case "tab", "down":
		isNavKey = true
		f.inputs[f.focusIdx].Blur()
		f.focusIdx = (f.focusIdx + 1) % f.visibleFieldCount()
		f.inputs[f.focusIdx].Focus()

	case "shift+tab", "up":
		isNavKey = true
		f.inputs[f.focusIdx].Blur()
		f.focusIdx = (f.focusIdx - 1 + f.visibleFieldCount()) % f.visibleFieldCount()
		f.inputs[f.focusIdx].Focus()

	case "ctrl+t":
		isNavKey = true
		if msg := f.validateFields(); msg != "" {
			f.validationError = msg
		} else {
			f.validationError = ""
			f.step = stepTesting
			f.testingDone = false
			f.testingErr = nil
		}

	case "enter":
		isNavKey = true
		if f.focusIdx < f.visibleFieldCount()-1 {
			f.inputs[f.focusIdx].Blur()
			f.focusIdx++
			f.inputs[f.focusIdx].Focus()
		} else {
			if msg := f.validateFields(); msg != "" {
				f.validationError = msg
			} else {
				f.validationError = ""
				f.step = stepTesting
				f.testingDone = false
				f.testingErr = nil
			}
		}

	case "esc":
		isNavKey = true
		f.step = stepDriverPicker
		f.focusIdx = 0
		f.inputs[0].Focus()
	}

	if !isNavKey {
		f.validationError = ""
		var cmd tea.Cmd
		f.inputs[f.focusIdx], cmd = f.inputs[f.focusIdx].Update(msg)
		return f, cmd
	}
	return f, nil
}

// validateFields checks that all required fields have values.
// Returns a human-readable error string, or empty string if valid.
// Database is intentionally optional — when empty the driver default is used
// and the sidebar lists all available databases on the instance.
func (f ConnForm) validateFields() string {
	required := []struct {
		idx   int
		label string
	}{
		{fieldName, "Connection name"},
		{fieldHost, "Host"},
		{fieldUser, "User"},
	}
	for _, r := range required {
		if strings.TrimSpace(f.inputs[r.idx].Value()) == "" {
			return r.label + " is required"
		}
	}
	if f.parsePort() <= 0 {
		return "Port must be a number between 1 and 65535"
	}
	return ""
}

func (f ConnForm) visibleFieldCount() int {
	if drivers[f.driverCursor].id == "postgres" {
		return fieldCount // all fields including SSLMode
	}
	return fieldCount - 1 // no SSLMode for mysql/sqlserver
}

func (f ConnForm) updateTesting(msg tea.KeyMsg) (ConnForm, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if !f.testingDone {
			f.lastAction = &ConnFormAction{Cancelled: true}
		} else {
			f.step = stepFields
			f.testingDone = false
			f.testingErr = nil
		}
	case "enter":
		if f.testingDone && f.testingErr == nil {
			f.lastAction = &ConnFormAction{
				Connection: config.Connection{
					Name:     f.inputs[fieldName].Value(),
					Driver:   drivers[f.driverCursor].id,
					Host:     f.inputs[fieldHost].Value(),
					Port:     f.parsePort(),
					User:     f.inputs[fieldUser].Value(),
					Database: f.inputs[fieldDatabase].Value(),
					SSLMode:  f.inputs[fieldSSLMode].Value(),
				},
				Password:     f.inputs[fieldPassword].Value(),
				IsEdit:       f.isEdit,
				OriginalName: f.originalName,
			}
		}
	}
	return f, nil
}

// View renders the current step of the connection form.
func (f ConnForm) View() string {
	boxW := f.width * 4 / 5
	if boxW < 50 {
		boxW = 50
	}
	if f.width > 0 && boxW > f.width-8 {
		boxW = f.width - 8
	}

	var inner string
	switch f.step {
	case stepDriverPicker:
		inner = f.viewDriverPicker(boxW)
	case stepActivation:
		inner = f.viewActivation(boxW)
	case stepFields:
		inner = f.viewFields(boxW)
	case stepTesting:
		inner = f.viewTesting(boxW)
	}

	box := f.styles.PanelFocused.
		Width(boxW-2).
		Padding(0, 1).
		Render(inner)

	boxH := lipgloss.Height(box)
	topPad := (f.height - boxH) / 2
	if topPad < 0 {
		topPad = 0
	}
	leftPad := (f.width - lipgloss.Width(box)) / 2
	if leftPad < 0 {
		leftPad = 0
	}

	return strings.Repeat("\n", topPad) +
		strings.Repeat(" ", leftPad) +
		strings.ReplaceAll(box, "\n", "\n"+strings.Repeat(" ", leftPad))
}

// wrapHints wraps a hint string like "[A] act  [B] act  [C] act" into
// multiple lines at most maxW runes wide. Shortcuts are treated as atomic
// tokens (split on double-space "  ") so a shortcut is never broken across
// lines. Each line is indented with two spaces.
func wrapHints(hints string, maxW int) string {
	tokens := strings.Split(hints, "  ")
	var lines []string
	current := ""
	for _, tok := range tokens {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		candidate := ""
		if current == "" {
			candidate = "  " + tok
		} else {
			candidate = current + "  " + tok
		}
		if len([]rune(candidate)) <= maxW {
			current = candidate
		} else {
			if current != "" {
				lines = append(lines, current)
			}
			current = "  " + tok
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return strings.Join(lines, "\n")
}

func (f ConnForm) viewDriverPicker(boxW int) string {
	title := f.styles.Title.Render(styles.IconDatabase + "  Select Driver")
	sep := f.styles.Muted.Render(strings.Repeat("─", boxW-4))

	var rows []string
	for i, d := range drivers {
		label := "  " + d.icon + "  " + d.label
		if i == f.driverCursor {
			rows = append(rows, f.styles.TreeSelected.Render(label))
		} else {
			rows = append(rows, f.styles.Text.Render(label))
		}
	}

	hint := f.styles.Muted.Render("\n" + wrapHints("[↑↓] Navigate  [Enter] Select  [Esc] Cancel", boxW-4))
	return lipgloss.JoinVertical(lipgloss.Left,
		title, sep, strings.Join(rows, "\n"), hint)
}

func (f ConnForm) viewActivation(boxW int) string {
	d := drivers[f.driverCursor]
	title := f.styles.Title.Render(d.icon + "  Activating " + d.label + " Driver")
	sep := f.styles.Muted.Render(strings.Repeat("─", boxW-4))

	body := fmt.Sprintf(
		"  Driver : %s\n  Status : bundled in binary  %s",
		d.pkg, f.styles.Success.Render(styles.IconConnected),
	)

	hint := f.styles.Muted.Render("\n" + wrapHints("[Enter] Continue  [Esc] Back", boxW-4))
	return lipgloss.JoinVertical(lipgloss.Left,
		title, sep, "", f.styles.Text.Render(body), hint)
}

func (f ConnForm) viewFields(boxW int) string {
	d := drivers[f.driverCursor]
	title := f.styles.Title.Render(styles.IconNew + "  New " + d.label + " Connection")
	sep := f.styles.Muted.Render(strings.Repeat("─", boxW-4))

	fieldDefs := []struct {
		label string
		idx   int
	}{
		{"Name    ", fieldName},
		{"Host    ", fieldHost},
		{"Port    ", fieldPort},
		{"User    ", fieldUser},
		{"Database", fieldDatabase},
		{"Password", fieldPassword},
	}
	if d.id == "postgres" {
		fieldDefs = append(fieldDefs, struct {
			label string
			idx   int
		}{"SSL Mode", fieldSSLMode})
	}

	var rows []string
	for _, fd := range fieldDefs {
		label := f.styles.Subtext.Render("  " + fd.label + "  ")
		rows = append(rows, label+f.inputs[fd.idx].View())
	}

	hint := f.styles.Muted.Render("\n" + wrapHints("[Tab/↓] Next  [Shift+Tab/↑] Prev  [Ctrl+T] Test  [Enter] Test (last field)  [Esc] Back", boxW-4))

	var footer string
	if f.validationError != "" {
		footer = "\n" + f.styles.Error.Render("  "+styles.IconError+"  "+f.validationError)
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		title, sep, "", strings.Join(rows, "\n"), hint, footer)
}

func (f ConnForm) viewTesting(boxW int) string {
	connName := f.inputs[fieldName].Value()
	if connName == "" {
		connName = "connection"
	}

	if !f.testingDone {
		body := f.styles.Warning.Render(
			"  " + styles.IconConnecting + "  Connecting to " + connName + "…",
		)
		hint := f.styles.Muted.Render("\n" + wrapHints("[Esc] Cancel test", boxW-4))
		return lipgloss.JoinVertical(lipgloss.Left, "", body, hint)
	}

	if f.testingErr != nil {
		errLine := f.styles.Error.Render(
			"  " + styles.IconError + "  " + f.testingErr.Error(),
		)
		hint := f.styles.Muted.Render("\n" + wrapHints("[Esc] Back to fields", boxW-4))
		return lipgloss.JoinVertical(lipgloss.Left, "", errLine, hint)
	}

	ok := f.styles.Success.Render(
		"  " + styles.IconConnected + "  Connected",
	)
	hint := f.styles.Muted.Render("\n" + wrapHints("[Enter] Save  [Esc] Back", boxW-4))
	return lipgloss.JoinVertical(lipgloss.Left, "", ok, hint)
}
