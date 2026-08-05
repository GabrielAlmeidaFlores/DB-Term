# AGENTS.md — db-term Contributor & AI Agent Guide

This document defines the rules, conventions, and patterns that every
contributor — human or AI agent — must follow when working on db-term.
Deviations require explicit justification in the commit message.

---

## Table of Contents

1. [Project Overview](#project-overview)
2. [Comment Policy](#comment-policy)
3. [Go Code Style](#go-code-style)
4. [Error Handling](#error-handling)
5. [Package Architecture & Import Rules](#package-architecture--import-rules)
6. [Bubble Tea Patterns](#bubble-tea-patterns)
7. [Concurrency Rules](#concurrency-rules)
8. [Testing Requirements](#testing-requirements)
9. [Adding a New Feature](#adding-a-new-feature)
10. [Adding a New Database Driver](#adding-a-new-database-driver)
11. [Visual Design Rules](#visual-design-rules)
12. [Commit & PR Conventions](#commit--pr-conventions)
13. [What AI Agents Must Never Do](#what-ai-agents-must-never-do)

---

## Project Overview

db-term is a terminal UI database client for PostgreSQL, MySQL, and SQL Server,
built with Go 1.22+, Bubble Tea, Lip Gloss, and GORM. The architecture separates
concerns strictly: the `db` package knows nothing about the UI, panels know nothing
about each other, and the `app` package is the only place that orchestrates everything.

Reference files:
- `ARCHITECTURE.md` — full design specification
- `internal/app/app.go` — root Bubble Tea model, message routing
- `internal/ui/styles/theme.go` — single source of truth for all visual styles

---

## Comment Policy

### Rule: documentation comments only

The only permitted comments are:

1. **Godoc** — directly above an exported declaration (`func`, `type`, `const`, `var`, `package`).
2. **Regression test comments** — a single line inside a test function naming the bug being prevented.

Every other comment is forbidden. This includes section dividers, inline labels, narration of what the code does, and comments on unexported symbols.

**Forbidden — all of the following:**

```go
// Centre the box.
topPad := (m.height - lipgloss.Height(box)) / 2

// ── Two-key sequences ────────────────────────────────────────────────────────
switch combo {

// Single-quoted string literal.
case runes[i] == '\'':

// Skip current word characters, then skip spaces.
for col < len(line) && !isSpace(line[col]) {

// editorMode is the current vi editing mode.
type editorMode int
```

**Permitted — godoc on exported symbol:**

```go
// Manager is a thread-safe pool of named GORM connections.
// It exposes synchronous methods only; callers wrap them in tea.Cmd.
type Manager struct { ... }

// Connect opens a GORM connection for the given config and password.
// Returns an error wrapping the underlying driver error on failure.
func (m *Manager) Connect(c config.Connection, password string) error { ... }
```

**Permitted — regression test comment:**

```go
func TestModal_TabWithZeroButtonsNoPanic(t *testing.T) {
    // Regression: tab with 0 buttons previously caused division by zero.
    ...
}
```

### Godoc comments — required for all exported symbols

Every exported function, type, constant, variable, and package must have a
complete godoc comment. The comment must:

1. Start with the symbol name.
2. Be a complete English sentence ending with a period.
3. Explain what the symbol **is** or **does**, not how it works internally.
4. Document all notable parameters, return values, and error conditions.

```go
// Manager is a thread-safe pool of named GORM connections.
// It exposes synchronous methods only; callers wrap them in tea.Cmd.
type Manager struct { ... }

// Connect opens a GORM connection for the given config and password.
// It pings the server to verify reachability before storing the entry.
// Returns an error wrapping the underlying driver error on failure.
func (m *Manager) Connect(c config.Connection, password string) error { ... }
```

---

## Go Code Style

### Formatting

- `gofmt` and `goimports` must be run before every commit. Use `make fmt`.
- CI enforces formatting via `make fmt-check`. PRs with formatting violations are rejected.
- Line length has no hard limit, but lines over 100 characters should be broken
  for readability.

### Naming

- Follow standard Go naming conventions (Go Code Review Comments).
- Acronyms are all-caps: `DSN`, `SQL`, `TUI`, `UI`, not `Dsn`, `Sql`, `Tui`.
- Avoid stuttering: `db.Manager`, not `db.DBManager`.
- Unexported types do not need godoc but benefit from a single-line comment
  when their purpose is not self-evident.

### Errors

See [Error Handling](#error-handling).

### Constants and magic numbers

Named constants are required for any literal that:
- Appears more than once.
- Represents a domain concept (sizes, limits, intervals, ports).
- Would require a comment to explain its meaning.

```go
// Required named constants:
const tablePreviewLimit = 200
const panelCount = 3
const PingInterval = 3 * time.Minute

// Forbidden:
sql := fmt.Sprintf("SELECT * FROM %s LIMIT 200", table)
next := (m.activePanel + 1) % 3
```

### Value vs pointer receivers

- Methods that modify the receiver must use pointer receivers (`*T`).
- Methods that only read must use value receivers (`T`) unless the type is
  large or contains a mutex.
- **Critical**: Bubble Tea sub-model methods (`Update`, `View`, `Init`) use
  value receivers by convention. Do not mutate state visible outside the method
  in a value receiver — the change will be silently discarded.

```go
// Wrong — m is a copy; m.sidebar change is lost.
func (m Model) connectCmd(...) tea.Cmd {
    m.sidebar.SetConnState(name, StateConnecting) // lost
    return ...
}

// Correct — call SetConnState at the call site on the real model.
m.sidebar.SetConnState(name, StateConnecting)
cmds = append(cmds, m.connectCmd(conn, pass))
```

### Slices

- Use rune-based indexing, not byte-based, for any string that may contain
  multi-byte characters (user-provided names, table names, schema names).

```go
// Wrong — panics or garbles multi-byte chars:
label = label[:maxWidth]

// Correct:
runes := []rune(label)
if len(runes) > maxWidth {
    label = string(runes[:maxWidth-1]) + "…"
}
```

---

## Error Handling

### Wrapping

Every error returned from an external package or lower-level function must be
wrapped with context using `fmt.Errorf("context: %w", err)`. The context string
must identify the operation and the subject.

```go
// Required:
return fmt.Errorf("db: opening connection %q: %w", c.Name, err)
return fmt.Errorf("config: reading file: %w", err)
return fmt.Errorf("secrets: decryption failed for %q: %w", connName, err)

// Forbidden:
return err
return fmt.Errorf("failed: %v", err)  // loses the original error for errors.Is/As
```

### Discarding errors

`_ = someFunc()` is permitted **only** for best-effort operations during
graceful shutdown. All other error discards require explicit justification
in the commit message and a comment explaining the decision at the call site.

```go
// Permitted — best-effort on shutdown:
_ = m.Disconnect(name) // best-effort; ignore individual errors on shutdown

// Forbidden — silently swallows actionable errors:
_ = m.cfg.Save()
_ = m.secrets.Save()
```

### User-visible errors

Errors that affect the user's work must surface through the UI:
- Query errors → red banner in the results panel + error-border on the panel.
- Connection errors → error modal with the wrapped error message.
- Config/secrets save errors → transient statusbar message.

---

## Package Architecture & Import Rules

The dependency graph is strictly enforced. No package may import another in a
direction that creates a cycle. The allowed import directions are:

```
cmd/main
    └── internal/app        ← orchestrates everything; may import all packages
            ├── internal/config      ← no internal imports (stdlib + go-toml + x/crypto)
            ├── internal/types       ← no internal imports (pure data structs)
            ├── internal/clipboard   ← no internal imports (wraps atotto/clipboard)
            ├── internal/db          ← imports config + types only
            └── internal/ui/...
                    └── internal/ui/styles  ← imports config only (one-way)
```

Enforcement table:

| Package          | May import                               | Must NOT import          |
|------------------|------------------------------------------|--------------------------|
| `types`          | stdlib only                              | everything else          |
| `config`         | stdlib, go-toml/v2, x/crypto             | `db`, `ui`, `types`      |
| `clipboard`      | stdlib, atotto/clipboard                 | everything else          |
| `db`             | `config`, `types`, stdlib, gorm drivers  | `ui`, `app`              |
| `ui/styles`      | `config`, lipgloss                       | `db`, `app`              |
| `ui/panels/*`    | `config`, `types`, `ui/styles`, bubbles  | `db`, `app`              |
| `ui/components`  | `config`, `types`, `ui/styles`, bubbles  | `db`, `app`              |
| `app`            | all packages                             | (none forbidden)         |

### Panel–app communication: the pending-action pattern

Panels cannot import `app` because `app` imports panels (cycle). Therefore,
panels communicate upward via **pending actions**, not Bubble Tea messages.

Each panel defines its own action type and exposes a `TakeXxx()` method:

```go
// In sidebar/sidebar.go:
type ActionKind int
const (ActionNone ActionKind = iota; ActionOpenTable; ActionNewConn; ActionDeleteConn)
type Action struct { Kind ActionKind; ConnName, SchemaName, TableName string }
func (m *Model) TakeAction() Action { ... }

// In app.go:
m.sidebar, cmd = m.sidebar.Update(msg)
if action := m.sidebar.TakeAction(); action.Kind != sidebar.ActionNone {
    // handle here
}
```

`TakeAction()` must clear the pending action atomically (set to zero value)
and be idempotent (calling it twice in a row returns zero value the second time).

### Async DB operations

Database operations run in background goroutines via `tea.Cmd` closures.
These closures are defined in `app.go` and return unexported message types.
The `db` package is pure synchronous — it does not return `tea.Cmd` or `tea.Msg`.

```go
// Pattern for all async DB operations:
cmds = append(cmds, func() tea.Msg {
    result := m.dbMgr.RunQuery(connName, sql)
    return queryResultMsg{
        columns: result.Columns,
        rows:    result.Rows,
        elapsed: result.Elapsed,
        err:     result.Err,
    }
})
```

---

## Bubble Tea Patterns

### Model/Update/View contract

Every sub-model must implement all three methods:

```go
func (m Model) Init() tea.Cmd
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd)
func (m Model) View() string
```

- `Init()` returns any startup commands (spinner ticks, blink cmds, etc.).
- `Update()` must return the modified model; never mutate in place.
- `View()` must be a pure function of model state with no side effects.
- `View()` must return an empty string when dimensions are zero:
  ```go
  func (m Model) View() string {
      if m.width == 0 || m.height == 0 {
          return ""
      }
      ...
  }
  ```

### Returned tea.Cmd must never be discarded

Every call to a sub-model's `Update()` that returns a `tea.Cmd` must append
that command to the batch. Discarding a `tea.Cmd` breaks animations (spinner,
cursor blink) and prevents async operations from completing.

```go
// Required pattern:
var mo components.Modal
var moCmd tea.Cmd
mo, moCmd = m.modal.Update(msg)
if moCmd != nil {
    cmds = append(cmds, moCmd)
}

// Forbidden:
mo, _ = m.modal.Update(msg)  // discards animation ticks
```

### Propagating non-key messages to all animated panels

Panels with animations (spinner, cursor blink) must receive non-key messages
even when they do not have keyboard focus. The root `Update()` must forward
all non-key messages to animated panels before processing the message itself.

```go
// In app.go Update():
if _, isKey := msg.(tea.KeyMsg); !isKey {
    m.results, reCmd = m.results.Update(msg)
    cmds = append(cmds, reCmd)
}
```

### `SetSize()` propagation

Every `tea.WindowSizeMsg` must propagate to every sub-model and overlay.
Failure to do so causes panels to render at the wrong dimensions after resize.

```go
case tea.WindowSizeMsg:
    m.width, m.height = msg.Width, msg.Height
    m = m.resize()  // propagates to all sub-models

func (m Model) resize() Model {
    d := layout.Compute(m.width, m.height, m.sidebarVisible)
    m.sidebar.SetSize(d.SidebarW, d.SidebarH)
    m.editor.SetSize(d.RightW, d.EditorH)
    m.results.SetSize(d.RightW, d.ResultsH)
    m.statusbar.SetWidth(d.StatusW)
    m.settings.SetSize(m.width, m.height)
    if m.modal != nil { m.modal.SetSize(m.width, m.height) }
    if m.connform != nil { m.connform.SetSize(m.width, m.height) }
    return m
}
```

### Styles propagation on theme change

When the theme changes, `ToStyles()` must be called to rebuild the `Styles`
struct and the result propagated to every sub-model via its `SetStyles()` method.
No panel should cache lipgloss styles beyond the `Styles` struct received from
the app layer.

---

## Concurrency Rules

### `db.Manager` is shared across goroutines

`db.Manager` is accessed from the main goroutine (Bubble Tea's `Update`) and
from background `tea.Cmd` goroutines concurrently. All reads and writes to
the internal `conns` map are protected by `sync.RWMutex`:

- Use `m.mu.RLock()` / `m.mu.RUnlock()` for reads.
- Use `m.mu.Lock()` / `m.mu.Unlock()` for writes.
- Never hold the lock across a blocking call (DB open, ping, query).
- Never upgrade an `RLock` to a `Lock` — release and re-acquire.

### Context-based query cancellation

Every `RunQuery` call creates a `context.WithCancel` and stores the
`CancelFunc` in the `ConnEntry`. `CancelQuery` calls this function to
interrupt an in-flight query. The `CancelFunc` is cleared after the query
finishes (deferred `clearCancel()`).

### Goroutine variable capture in loops

When creating `tea.Cmd` closures inside a `for` loop, always copy the loop
variable to a local before the closure:

```go
// Required — captures a stable local copy:
for _, snap := range m.dbMgr.All() {
    name := snap.Config.Name  // local copy
    cmds = append(cmds, func() tea.Msg {
        return m.dbMgr.Ping(name)
    })
}

// Forbidden — all closures capture the same loop variable:
for _, snap := range m.dbMgr.All() {
    cmds = append(cmds, func() tea.Msg {
        return m.dbMgr.Ping(snap.Config.Name)  // snap is the loop variable
    })
}
```

---

## Testing Requirements

### Unit tests

Every new exported function must have at least one unit test. Tests live in
`*_test.go` files in the same package (white-box tests).

Required test coverage per file:

| File | Minimum tests |
|---|---|
| `config/config.go` | `DefaultConfig` no empty fields; `ValidateKeybinds` detects conflict; `Load`/`Save` round-trip |
| `config/secrets.go` | encrypt/decrypt round-trip; persist/reload; delete; nonce uniqueness; 0600 permissions |
| `db/drivers.go` | `BuildDSN` for all 3 drivers; SSL fields; unknown driver error |
| `db/manager.go` | `Get`/`All` on empty; `CancelQuery`/`Disconnect`/`Ping`/`RunQuery` on missing connection |
| `ui/styles/theme.go` | all builtins non-empty; fallback; `Names()` order; `ToStyles` sets foreground colors |
| `ui/panels/sidebar/*` | `BuildTree`; `FlatList` respects Expanded; `FilterNodes` case-insensitive, parent inclusion |
| `ui/panels/results/pagination.go` | `TotalPages`; `CurrentRows` first/last page; `NextPage`/`PrevPage` no-op at bounds |
| `ui/components/modal.go` | cursor navigation; `Enter` fires action; `Esc` dismisses; `Tab` with 0 buttons no panic |
| `ui/components/connform.go` | step progression; Esc navigation; activation skip on second use |

### Integration tests

Integration tests require a live database and are gated by the build tag `integration`:

```go
//go:build integration
```

Run with:
```bash
go test ./internal/db/... -tags integration
```

Environment variables: `DB_HOST`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`.

### Regression tests

Every bug fix must include a regression test that:
1. Fails on the code before the fix.
2. Passes on the code after the fix.
3. Has a comment naming the specific bug it prevents.

```go
func TestModal_TabWithZeroButtonsNoPanic(t *testing.T) {
    // Regression: tab with 0 buttons previously caused division by zero.
    m := NewModal("T", "B", []ModalButton{}, testStyles())
    defer func() {
        if r := recover(); r != nil {
            t.Errorf("Modal Tab with 0 buttons panicked: %v", r)
        }
    }()
    m, _ = m.Update(pressKey("tab"))
    _ = m
}
```

### Test helpers

Shared test helpers live in `helpers_test.go` within the same package.
They must not be exported and must not import production packages not
already imported by the package under test.

---

## Adding a New Feature

1. **Update `ARCHITECTURE.md` first.** Define the data model, message types,
   and affected packages before writing code.

2. **Follow the phase order:**
   - Config/secrets changes → Phase 2 layer
   - DB layer changes → Phase 4 layer
   - New panel → Phase 5 layer (own package under `ui/panels/`)
   - New component → Phase 6 layer (`ui/components/`)
   - App integration → Phase 7 (`app/app.go`)

3. **New panel checklist:**
   - [ ] Implements `Init() tea.Cmd`, `Update(msg tea.Msg) (Model, tea.Cmd)`, `View() string`
   - [ ] Implements `SetStyles(s styles.Styles)` and `SetSize(w, h int)`
   - [ ] Implements `Focus()` and `Blur()`
   - [ ] Uses pending-action pattern for upward communication (no app import)
   - [ ] `View()` returns `""` when `width == 0 || height == 0`
   - [ ] Registered in `app.Model`, `app.New()`, `app.resize()`, and `app.applyTheme()`
   - [ ] Has unit tests

4. **New message type checklist (for background DB ops):**
   - [ ] Defined as unexported struct in `app/app.go`
   - [ ] Handled in the `switch` block inside `Update()`
   - [ ] Corresponding `tea.Cmd` closure defined in `app.go`, not in `db/`

5. **New config field checklist:**
   - [ ] Added to the appropriate struct in `config/config.go` with `toml:` tag
   - [ ] Set to a sensible default in `DefaultConfig()`
   - [ ] If it is a keybind: registered in `ValidateKeybinds()`
   - [ ] `config.toml` example in `ARCHITECTURE.md` updated

---

## Adding a New Database Driver

1. Add the Go module: `go get gorm.io/driver/<name>`
2. Add the driver ID string to `drivers.go:BuildDSN()` switch and `NewDialector()` switch
3. Add the entry to `connform.go:drivers` slice (id, label, pkg string, default port)
4. Add driver-specific introspection SQL to `app.go:introspectSQL()`
5. Add the driver to `ARCHITECTURE.md` supported databases table
6. Add `BuildDSN` unit test for the new driver in `db/db_test.go`

---

## Visual Design Rules

### Single source of truth

All colors, icons, and styles originate from `internal/ui/styles/`. No panel
or component defines `lipgloss.Color`, `lipgloss.NewStyle()`, or icon string
literals inline. Every style is a field on `styles.Styles`, populated by
`ToStyles()`.

```go
// Forbidden:
return lipgloss.NewStyle().Foreground(lipgloss.Color("#f38ba8")).Render(s)

// Required:
return m.styles.Error.Render(s)
```

The single exception is `buildSwatch()` in `tab_theme.go`, which constructs
temporary styles purely for color preview rendering.

### Theme change propagation

When a theme changes, `applyTheme(themeID)` in `app.go` must call `SetStyles(s)`
on every sub-model and overlay. Any new sub-model added to `app.Model` must be
registered in `applyTheme()`.

### Nerd Font icons

All icon strings are defined in `internal/ui/styles/icons.go` as named constants.
Use the constant name, not the unicode literal:

```go
// Required:
label := styles.IconDatabase + "  " + connName

// Forbidden:
label := "󰆼  " + connName
```

### Panel borders

Use `m.styles.PanelFocused`, `m.styles.PanelUnfocused`, and `m.styles.PanelError`.
Do not construct borders ad-hoc with `lipgloss.RoundedBorder()` in panel code.

---

## Commit & PR Conventions

### Commit message format

```
<type>: <subject> (imperative, ≤72 chars)

<body: what and why, not how. Wrap at 72 chars.>
```

Types: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`

Subject rules:
- Imperative mood: "add feature", not "added" or "adds"
- No trailing period
- Specific: "fix sidebar cursor after connection removal" not "fix bug"

### What belongs in a commit

- One logical change per commit.
- Bug fixes include the regression test in the same commit.
- Refactors do not include behaviour changes.
- `gofmt` must be clean before committing (`make fmt-check`).

---

## What AI Agents Must Never Do

The following are hard prohibitions. Breaking any of these requires explicit
user approval and a documented reason in the commit.

1. **Never write loose inline comments** that describe what the immediately
   following code does. See [Comment Policy](#comment-policy).

   This includes section dividers (`// ── ...`), case labels, loop annotations,
   and any comment on an unexported symbol. Only godoc on exported declarations
   and regression-test bug-naming comments are permitted.

2. **Never discard a `tea.Cmd`** returned from any `Update()` call using `_`.
   Every command must be appended to the batch.

3. **Never import `internal/app` from a panel or component package.** Use the
   pending-action pattern.

4. **Never define colors or icons inline** in panel or component code.
   All visual values come from `styles.Styles` or `styles` constants.

5. **Never use `len(s)` to index or slice a string** that may contain
   multi-byte characters. Always convert to `[]rune` first.

6. **Never write `_ = someFunc(err-returning-function)`** unless it is a
   documented best-effort operation at shutdown.

7. **Never mutate state inside a value receiver** and expect the change to
   persist. If a method needs to update model state, it must either use a
   pointer receiver or return the modified model.

8. **Never use `fmt.Sprintf("%v", v)` for raw database scan results.** Use
   `valueToString(v)` from `db/manager.go` which handles `nil`, `[]byte`,
   and other types correctly.

9. **Never add a new sub-model to `app.Model` without registering it in
   `resize()` and `applyTheme()`.**

10. **Never skip the regression test for a bug fix.**

11. **Never create a git commit automatically.** Only commit when explicitly
    instructed by the user in that same message. Preparing, staging, or
    committing changes without a direct "commit" instruction is forbidden.
