package components

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gabrielfloresousion/db-term/internal/config"
	"github.com/gabrielfloresousion/db-term/internal/ui/styles"
)

func testStyles() styles.Styles {
	return styles.ToStyles(styles.Resolve("catppuccin-mocha", config.ThemeConfig{}))
}

func testKeybinds() config.Keybinds {
	return config.DefaultConfig().Keybinds
}

func TestWrapText_ShortLine(t *testing.T) {
	lines := wrapText("hello world", 80)
	if len(lines) != 1 {
		t.Errorf("wrapText: expected 1 line, got %d", len(lines))
	}
}

func TestWrapText_WrapsAtMaxWidth(t *testing.T) {
	text := "one two three four five six seven eight"
	lines := wrapText(text, 15)
	for _, l := range lines {
		if len(l) > 15 {
			t.Errorf("wrapText: line %q exceeds maxW=15", l)
		}
	}
	if len(lines) < 2 {
		t.Errorf("wrapText: expected multiple lines, got %d", len(lines))
	}
}

func TestWrapText_EmptyString(t *testing.T) {
	lines := wrapText("", 40)
	if len(lines) != 1 {
		t.Errorf("wrapText empty: expected 1 line, got %d", len(lines))
	}
}

func TestModal_TabWithZeroButtonsNoPanic(t *testing.T) {
	// Regression: tab with 0 buttons previously caused division by zero.
	m := NewModal("T", "B", []ModalButton{}, testStyles())
	m.SetSize(80, 24)
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Modal Tab with 0 buttons panicked: %v", r)
		}
	}()
	m, _ = m.Update(pressKey("tab"))
	_ = m
}

func TestModal_InitialCursorIsZero(t *testing.T) {
	m := NewModal("Title", "Body", []ModalButton{{Label: "OK"}}, testStyles())
	if m.cursor != 0 {
		t.Errorf("Modal initial cursor = %d, want 0", m.cursor)
	}
}

func TestModal_EscSetsДismissed(t *testing.T) {
	m := NewModal("T", "B", []ModalButton{{Label: "OK"}}, testStyles())
	m.SetSize(80, 24)
	msg := pressKey("esc")
	m, _ = m.Update(msg)
	if !m.Dismissed() {
		t.Error("Modal: Esc should set Dismissed = true")
	}
}

func TestModal_EnterFiresAction(t *testing.T) {
	m := NewModal("T", "B", []ModalButton{{Label: "Confirm"}}, testStyles())
	m.SetSize(80, 24)
	m, _ = m.Update(pressKey("enter"))
	a := m.TakeAction()
	if a == nil {
		t.Fatal("Modal: Enter should produce an action")
	}
	if a.Label != "Confirm" {
		t.Errorf("Modal action Label = %q, want %q", a.Label, "Confirm")
	}
}

func TestModal_RightMoveCursor(t *testing.T) {
	m := NewModal("T", "B", []ModalButton{{Label: "A"}, {Label: "B"}}, testStyles())
	m.SetSize(80, 24)
	m, _ = m.Update(pressKey("right"))
	if m.cursor != 1 {
		t.Errorf("Modal right: cursor = %d, want 1", m.cursor)
	}
}

func TestModal_CursorDoesNotGoNegative(t *testing.T) {
	m := NewModal("T", "B", []ModalButton{{Label: "A"}, {Label: "B"}}, testStyles())
	m.SetSize(80, 24)
	m, _ = m.Update(pressKey("left"))
	if m.cursor != 0 {
		t.Errorf("Modal left at 0: cursor = %d, want 0", m.cursor)
	}
}

func TestModal_ViewContainsTitle(t *testing.T) {
	m := NewModal("My Dialog", "Some body text.", []ModalButton{{Label: "OK"}}, testStyles())
	m.SetSize(80, 24)
	v := m.View()
	if !strings.Contains(v, "My Dialog") {
		t.Errorf("Modal.View() does not contain title 'My Dialog'\nGot:\n%s", v)
	}
}

func TestModal_TakeActionClearsIt(t *testing.T) {
	m := NewModal("T", "B", []ModalButton{{Label: "OK"}}, testStyles())
	m.SetSize(80, 24)
	m, _ = m.Update(pressKey("enter"))
	_ = m.TakeAction()
	if m.TakeAction() != nil {
		t.Error("TakeAction: second call should return nil")
	}
}

func TestConnForm_StartsOnDriverPicker(t *testing.T) {
	f := NewConnForm(testStyles())
	if f.step != stepDriverPicker {
		t.Errorf("ConnForm: initial step = %d, want stepDriverPicker", f.step)
	}
}

func TestConnForm_EscOnDriverPickerCancels(t *testing.T) {
	f := NewConnForm(testStyles())
	f.SetSize(80, 24)
	f, _ = f.Update(pressKey("esc"))
	a := f.TakeAction()
	if a == nil || !a.Cancelled {
		t.Error("ConnForm: Esc on driver picker should cancel")
	}
}

func TestConnForm_EnterAdvancesToActivation(t *testing.T) {
	f := NewConnForm(testStyles())
	f.SetSize(80, 24)
	f, _ = f.Update(pressKey("enter"))
	if f.step != stepActivation {
		t.Errorf("ConnForm: after Enter on driver picker, step = %d, want stepActivation", f.step)
	}
}

func TestConnForm_ActivationEnterAdvancesToFields(t *testing.T) {
	f := NewConnForm(testStyles())
	f.SetSize(80, 24)
	f, _ = f.Update(pressKey("enter")) // pick driver
	f, _ = f.Update(pressKey("enter")) // confirm activation
	if f.step != stepFields {
		t.Errorf("ConnForm: after activation, step = %d, want stepFields", f.step)
	}
}

func TestConnForm_SecondTimeNoActivationStep(t *testing.T) {
	f := NewConnForm(testStyles())
	f.SetSize(80, 24)
	f, _ = f.Update(pressKey("enter")) // pick driver
	f, _ = f.Update(pressKey("enter")) // confirm activation → fields
	f, _ = f.Update(pressKey("esc"))   // back to driver picker
	f, _ = f.Update(pressKey("enter")) // pick driver again
	// Already activated, should skip activation.
	if f.step != stepFields {
		t.Errorf("ConnForm: second time should skip activation, got step=%d", f.step)
	}
}

func TestConnForm_ActivationEscGoesBackToDriverPicker(t *testing.T) {
	f := NewConnForm(testStyles())
	f.SetSize(80, 24)
	f, _ = f.Update(pressKey("enter")) // pick driver
	f, _ = f.Update(pressKey("esc"))   // back from activation
	if f.step != stepDriverPicker {
		t.Errorf("ConnForm: Esc on activation should go back to driver picker, got step=%d", f.step)
	}
}

func TestConnForm_ViewRendersWithoutPanic(t *testing.T) {
	f := NewConnForm(testStyles())
	f.SetSize(80, 24)
	_ = f.View()
	f, _ = f.Update(pressKey("enter"))
	_ = f.View()
	f, _ = f.Update(pressKey("enter"))
	_ = f.View()
}

func TestConnForm_EscAfterTestErrorReturnsToFields(t *testing.T) {
	// Regression: Esc after a connection error was caught by the first ESC
	// handler in updateTesting (which set Cancelled=true and closed the form)
	// instead of going back to the fields step so the user could fix credentials.
	f := NewConnForm(testStyles())
	f.SetSize(80, 24)
	f, _ = f.Update(pressKey("enter"))
	f, _ = f.Update(pressKey("enter"))

	f.inputs[fieldName].SetValue("test-conn")
	f.inputs[fieldHost].SetValue("localhost")
	f.inputs[fieldPort].SetValue("5432")
	f.inputs[fieldUser].SetValue("admin")

	f.focusIdx = f.visibleFieldCount() - 1
	f, _ = f.Update(pressKey("enter"))

	if f.step != stepTesting {
		t.Fatalf("expected stepTesting before error, got step=%d", f.step)
	}

	f.SetTestResult(fmt.Errorf("connection refused"))

	f, _ = f.Update(pressKey("esc"))

	if f.step != stepFields {
		t.Errorf("step = %d after Esc on error, want stepFields", f.step)
	}
	if a := f.TakeAction(); a != nil && a.Cancelled {
		t.Error("Esc after test error must not cancel the form, only go back to fields")
	}
}

func TestConnForm_EscDuringTestingCancels(t *testing.T) {
	// Regression: Esc was blocked while testingDone==false, leaving the user
	// stuck on the "Connecting…" screen with no way to dismiss it.
	f := NewConnForm(testStyles())
	f.SetSize(80, 24)
	f, _ = f.Update(pressKey("enter")) // pick driver
	f, _ = f.Update(pressKey("enter")) // confirm activation → fields

	f.inputs[fieldName].SetValue("test-conn")
	f.inputs[fieldHost].SetValue("localhost")
	f.inputs[fieldPort].SetValue("5432")
	f.inputs[fieldUser].SetValue("admin")
	f.inputs[fieldDatabase].SetValue("mydb")

	f.focusIdx = f.visibleFieldCount() - 1
	f, _ = f.Update(pressKey("enter"))

	if f.step != stepTesting {
		t.Fatalf("expected stepTesting, got step=%d", f.step)
	}
	f, _ = f.Update(pressKey("esc"))
	a := f.TakeAction()
	if a == nil || !a.Cancelled {
		t.Error("ConnForm: Esc during testing should produce Cancelled action")
	}
}

func TestConnForm_ValidationBlocksEmptyRequired(t *testing.T) {
	// Database is now optional (user can browse all databases after connecting).
	// Validation still blocks when Name, Host, or User are empty.
	f := NewConnForm(testStyles())
	f.SetSize(80, 24)
	f, _ = f.Update(pressKey("enter")) // pick driver
	f, _ = f.Update(pressKey("enter")) // confirm activation → fields

	// Leave Name empty, fill other required fields.
	f.inputs[fieldHost].SetValue("localhost")
	f.inputs[fieldPort].SetValue("5432")
	f.inputs[fieldUser].SetValue("admin")
	// fieldName intentionally left empty.

	f.focusIdx = f.visibleFieldCount() - 1
	f, _ = f.Update(pressKey("enter"))

	if f.step == stepTesting {
		t.Error("ConnForm: should not advance to stepTesting with empty Name")
	}
	if f.validationError == "" {
		t.Error("ConnForm: validationError should be set when Name is empty")
	}
}

func TestConnForm_EmptyDatabaseIsAllowed(t *testing.T) {
	// Database field is optional — leaving it empty should allow transition to stepTesting.
	f := NewConnForm(testStyles())
	f.SetSize(80, 24)
	f, _ = f.Update(pressKey("enter")) // pick driver
	f, _ = f.Update(pressKey("enter")) // confirm activation → fields

	f.inputs[fieldName].SetValue("test-conn")
	f.inputs[fieldHost].SetValue("localhost")
	f.inputs[fieldPort].SetValue("5432")
	f.inputs[fieldUser].SetValue("admin")
	// fieldDatabase intentionally empty — should be allowed.

	f.focusIdx = f.visibleFieldCount() - 1
	f, _ = f.Update(pressKey("enter"))

	if f.step != stepTesting {
		t.Errorf("ConnForm: empty Database should be allowed, got step=%d validationError=%q",
			f.step, f.validationError)
	}
}

func TestStatusBar_ViewWithNoConnection(t *testing.T) {
	sb := NewStatusBar(testStyles(), testKeybinds())
	sb.SetWidth(80)
	v := sb.View()
	if !strings.Contains(v, "No connection") {
		t.Errorf("StatusBar no-conn: expected 'No connection', got:\n%s", v)
	}
}

func TestStatusBar_ViewWithActiveConn(t *testing.T) {
	sb := NewStatusBar(testStyles(), testKeybinds())
	sb.SetWidth(80)
	sb.SetConn("local-pg", "public")
	v := sb.View()
	if !strings.Contains(v, "local-pg") {
		t.Errorf("StatusBar: expected conn name 'local-pg', got:\n%s", v)
	}
}

func TestStatusBar_ShowMessageDisplaysTransient(t *testing.T) {
	sb := NewStatusBar(testStyles(), testKeybinds())
	sb.SetWidth(80)
	sb.ShowMessage("copied!")
	v := sb.View()
	if !strings.Contains(v, "copied!") {
		t.Errorf("StatusBar transient: expected 'copied!', got:\n%s", v)
	}
}

func TestStatusBar_MessageClearsAfterTick(t *testing.T) {
	sb := NewStatusBar(testStyles(), testKeybinds())
	sb.SetWidth(80)
	sb.ShowMessage("hello")
	sb.messageTTL = 1
	sb.Tick()
	if sb.message != "" {
		t.Errorf("StatusBar Tick: message should be cleared, got %q", sb.message)
	}
}

func TestStatusBar_SetResultShowsRowCount(t *testing.T) {
	sb := NewStatusBar(testStyles(), testKeybinds())
	sb.SetWidth(80)
	sb.SetConn("pg", "public")
	sb.SetResult(42, 80*time.Millisecond)
	v := sb.View()
	if !strings.Contains(v, "42 rows") {
		t.Errorf("StatusBar result: expected '42 rows', got:\n%s", v)
	}
}

func TestStatusBar_QueryingState(t *testing.T) {
	sb := NewStatusBar(testStyles(), testKeybinds())
	sb.SetWidth(80)
	sb.SetConn("pg", "public")
	sb.SetQuerying(true)
	v := sb.View()
	if !strings.Contains(v, "Executing") {
		t.Errorf("StatusBar querying: expected 'Executing', got:\n%s", v)
	}
}

func TestNewConnFormDuplicate_NameSuffixedWithCopy(t *testing.T) {
	// Regression: pressing 'y' on a connection must open the form pre-filled with
	// the original data and a " copy" suffix on the Name field so the user does
	// not accidentally overwrite the original connection on save.
	conn := config.Connection{
		Name:     "production",
		Driver:   "postgres",
		Host:     "db.example.com",
		Port:     5432,
		User:     "admin",
		Database: "mydb",
		SSLMode:  "require",
	}
	f := NewConnFormDuplicate(conn, testStyles())

	if got := f.inputs[fieldName].Value(); got != "production copy" {
		t.Errorf("Name = %q, want %q", got, "production copy")
	}
	if f.isEdit {
		t.Error("isEdit should be false for a duplicate form")
	}
	if f.originalName != "" {
		t.Errorf("originalName = %q, want empty", f.originalName)
	}
	if got := f.inputs[fieldHost].Value(); got != conn.Host {
		t.Errorf("Host = %q, want %q", got, conn.Host)
	}
	if got := f.inputs[fieldUser].Value(); got != conn.User {
		t.Errorf("User = %q, want %q", got, conn.User)
	}
	if got := f.inputs[fieldDatabase].Value(); got != conn.Database {
		t.Errorf("Database = %q, want %q", got, conn.Database)
	}
	if f.step != stepFields {
		t.Errorf("step = %v, want stepFields", f.step)
	}
}
