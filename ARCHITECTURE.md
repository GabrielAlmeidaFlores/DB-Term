# DB-Term — Architecture & Implementation Plan

A terminal UI database client inspired by DBeaver, built with Go + Bubble Tea.

> **Implementation status:** MVP complete. The sections below reflect the
> original design specification. Refer to the source code for the canonical
> truth. Key divergences from the original plan are summarised here:
>
> - `internal/app/messages.go` was deleted; all async message types are
>   unexported structs defined directly in `app/app.go`.
> - `layout.Compute` takes two additional parameters: `sidebarW int` and
>   `editorRatio int` for runtime panel resizing (alt+arrow shortcuts).
> - `NodeDatabase` was added to the sidebar tree between `NodeConnection` and
>   `NodeSchema` to support instance-wide database browsing without a target DB.
> - `db.SubConnKey(parent, dbName)` creates per-database sub-connections stored
>   in the same `Manager` pool.
> - `Init()` auto-reconnects all saved connections that have a stored password.
> - `config.Keybinds` has four new resize fields: `ResizePanelLeft/Right/Up/Down`.
> - Foreign-key cells are marked in query results and can navigate to the
>   referenced row through the configurable `FollowFK` key binding.
> - GORM logger is silenced (`logger.Silent`) to prevent SQL output from
>   corrupting the Bubble Tea alt-screen.

---

## Table of Contents

1. [Overview](#overview)
2. [Tech Stack & Dependencies](#tech-stack--dependencies)
3. [Tooling & CI](#tooling--ci)
4. [Project Structure](#project-structure)
5. [Package Dependency Graph](#package-dependency-graph)
6. [Visual Design System](#visual-design-system)
7. [Config & Secrets Layer](#config--secrets-layer)
8. [DB Layer](#db-layer)
9. [Bubble Tea Architecture](#bubble-tea-architecture)
10. [Active Connection Model](#active-connection-model)
11. [UI Layer — Panels](#ui-layer--panels)
12. [UI Layer — Components](#ui-layer--components)
13. [UI Layout & Navigation](#ui-layout--navigation)
14. [Cross-Cutting Concerns](#cross-cutting-concerns)
15. [Implementation Phases](#implementation-phases)
16. [Future Roadmap](#future-roadmap)

---

## Overview

| Attribute      | Value                                      |
|----------------|--------------------------------------------|
| Binary name    | `dbterm`                                   |
| Repo name      | `db-term`                                  |
| Language       | Go 1.22+                                   |
| TUI framework  | Bubble Tea + Lip Gloss + Bubbles           |
| ORM            | GORM                                       |
| Config path    | `~/.config/dbterm/config.toml`             |
| Secrets path   | `~/.config/dbterm/.secrets`                |
| Icons          | Nerd Fonts (constants in `styles/icons.go`)|

### Supported Databases (v0.1)

| Database   | GORM Driver                | Go module                   |
|------------|----------------------------|-----------------------------|
| PostgreSQL | `gorm.io/driver/postgres`  | Uses `pgx` under the hood   |
| MySQL      | `gorm.io/driver/mysql`     | Uses `go-sql-driver/mysql`  |
| SQL Server | `gorm.io/driver/sqlserver` | Uses `microsoft/go-mssqldb` |

---

## Tech Stack & Dependencies

### `go.mod` — exact modules to add

```
github.com/charmbracelet/bubbletea           # TUI event loop + Model/Update/View
github.com/charmbracelet/lipgloss            # terminal styling (colors, borders, layout)
github.com/charmbracelet/bubbles             # ready-made: textarea, table, spinner, textinput
gorm.io/gorm                                 # ORM core
gorm.io/driver/postgres                      # PostgreSQL driver
gorm.io/driver/mysql                         # MySQL driver
gorm.io/driver/sqlserver                     # SQL Server driver
github.com/pelletier/go-toml/v2              # TOML parser — preserves comments on round-trip
golang.org/x/crypto                          # AES-256-GCM + scrypt for secrets
golang.org/x/term                            # terminal size detection
atotto/clipboard                             # cross-platform clipboard (macOS/Linux/WSL)
```

> **Why `pelletier/go-toml/v2` instead of `BurntSushi/toml`:**
> `BurntSushi/toml` discards all comments when marshaling back to disk.
> `go-toml/v2` preserves the document structure on round-trip, so user-written
> comments in `config.toml` survive a `Save()` call.

### Init commands (run once in Phase 1)

```bash
go mod init github.com/<youruser>/db-term
go get github.com/charmbracelet/bubbletea
go get github.com/charmbracelet/lipgloss
go get github.com/charmbracelet/bubbles
go get gorm.io/gorm
go get gorm.io/driver/postgres
go get gorm.io/driver/mysql
go get gorm.io/driver/sqlserver
go get github.com/pelletier/go-toml/v2
go get golang.org/x/crypto
go get golang.org/x/term
go get github.com/atotto/clipboard
```

---

## Tooling & CI

### Makefile

```makefile
.PHONY: build run test lint fmt fmt-check clean

build:
	go build -o bin/dbterm ./cmd/main.go

run:
	go run ./cmd/main.go

test:
	go test ./... -race -timeout 30s

test-integration:
	go test ./internal/db/... -tags integration -race

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .
	goimports -w .

# fmt-check verifies formatting without modifying files (used in CI)
fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "Run 'make fmt' to fix formatting:" && gofmt -l . && exit 1)

clean:
	rm -rf bin/
```

### `.golangci.yml`

```yaml
linters:
  enable:
    - govet        # correctness checks
    - errcheck     # unchecked errors
    - staticcheck  # advanced static analysis
    - gosimple     # simplification hints
    - ineffassign  # unused assignments
    - unused       # unused code
    - gofmt        # formatting
    - goimports    # import ordering
    - gocritic     # style and performance hints
    - wrapcheck    # ensures errors from external packages are wrapped

linters-settings:
  wrapcheck:
    ignorePackageGlobs:
      - "github.com/charmbracelet/*"
```

### GitHub Actions (`.github/workflows/ci.yml`)

```yaml
name: CI

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"
      - uses: golangci/golangci-lint-action@v6
        with:
          version: latest
      - run: make fmt-check
      - run: make lint
      - run: make test

  build:
    runs-on: ubuntu-latest
    needs: test
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"
      - run: make build
```

---

## Project Structure

```
db-term/
├── .github/
│   └── workflows/
│       └── ci.yml
├── .golangci.yml
├── Makefile
│
├── cmd/
│   └── main.go                        # entrypoint
│
├── internal/
│   ├── types/
│   │   └── types.go                   # shared DB types — imported by db/ and app/
│   │
│   ├── app/
│   │   ├── app.go                     # root Bubble Tea Model
│   │   └── messages.go                # ALL custom tea.Msg types
│   │
│   ├── config/
│   │   ├── config.go                  # TOML read/write, structs, defaults
│   │   └── secrets.go                 # AES-256-GCM password encryption
│   │
	│   ├── db/
	│   │   ├── manager.go                 # thread-safe connection pool
	│   │   ├── drivers.go                 # GORM dialector factory
	│   │   ├── introspect.go              # schema/table/column queries
	│   │   └── keepalive.go               # periodic ping + reconnect logic
│   │
│   ├── clipboard/
│   │   └── clipboard.go               # cross-platform copy abstraction
│   │
│   └── ui/
│       ├── layout/
│       │   └── layout.go
│       ├── panels/
│       │   ├── sidebar/
│       │   │   ├── sidebar.go
│       │   │   ├── tree.go
│       │   │   └── filter.go          # sidebar search/filter logic
│       │   ├── editor/
│       │   │   └── editor.go
│       │   ├── results/
│       │   │   ├── results.go
│       │   │   └── pagination.go      # result set page management
│       │   └── settings/
│       │       ├── settings.go
│       │       ├── tab_keybinds.go    # keybinds tab (with conflict validation)
│       │       ├── tab_connections.go # connections tab
│       │       └── tab_theme.go       # theme tab
│       ├── components/
│       │   ├── modal.go
│       │   ├── connform.go
│       │   └── statusbar.go
│       └── styles/
│           ├── theme.go
│           └── icons.go
│
├── go.mod
└── go.sum
```

---

## Package Dependency Graph

```
cmd/main
    │
    ├── internal/app          ← root, orchestrates everything
    │       ├── internal/config
    │       ├── internal/db
    │       ├── internal/types
    │       ├── internal/clipboard
    │       └── internal/ui/...
    │
    ├── internal/config       ← no internal imports (stdlib only)
    │
    ├── internal/types        ← no internal imports (pure data types)
    │
    ├── internal/clipboard    ← no internal imports (wraps atotto/clipboard)
    │
    ├── internal/db
    │       ├── internal/config
    │       └── internal/types
    │
    └── internal/ui/styles
            └── internal/config   ← one-way only; config never imports styles
```

### Import rules enforced by this graph

| Package | May import | Must NOT import |
|---|---|---|
| `types` | stdlib only | everything else |
| `config` | stdlib, go-toml, x/crypto | `db`, `ui`, `types` |
| `clipboard` | stdlib, atotto/clipboard | everything else |
| `db` | `config`, `types`, stdlib, gorm | `ui`, `app`, `messages` |
| `ui/styles` | `config`, lipgloss | `db`, `app` |
| `app/messages` | `config`, `types`, bubbletea | `db`, `ui` |
| `app` | all internal packages | none forbidden |

---

## Visual Design System

This section is the single source of truth for every visual decision in the app.
No panel or component defines colors or icons inline — everything comes from here.

---

### Nerd Font Icons (`internal/ui/styles/icons.go`)

```go
package styles

const (
    // ── Database objects ──────────────────────────────────────
    IconDatabase  = "󰆼"   // nf-md-database
    IconPostgres  = "󰆼"   // nf-md-database  (blue tint via style)
    IconMySQL     = "󰆼"   // nf-md-database  (orange tint via style)
    IconSQLServer = "󰆼"   // nf-md-database  (red tint via style)
    IconTable     = "󰓫"   // nf-md-table
    IconView      = "󰒔"   // nf-md-eye_outline
    IconSchema    = ""    // nf-fa-folder_open
    IconColumn    = "󰠵"   // nf-md-table_column
    IconPK        = ""    // nf-fa-key        ← primary key marker
    IconNullable  = "󰌾"   // nf-md-null
    IconFunction  = ""   // nf-fa-code
    IconIndex     = "󰆖"   // nf-md-lightning_bolt

    // ── Connection states ─────────────────────────────────────
    IconConnected    = "●"
    IconDisconnected = "○"
    IconConnecting   = "◌"
    IconError        = ""   // nf-fa-warning

    // ── Panels & navigation ───────────────────────────────────
    IconEditor   = "󰦕"   // nf-md-code_braces
    IconResults  = "󰓪"   // nf-md-table_large
    IconSidebar  = ""   // nf-fa-sitemap
    IconSettings = ""   // nf-fa-cog
    IconHelp     = "󰋗"   // nf-md-help_circle_outline
    IconHistory  = "󱑁"   // nf-md-history
    IconTheme    = "󰉦"   // nf-md-palette
    IconFilter   = "󰈿"   // nf-md-filter_outline

    // ── Actions ───────────────────────────────────────────────
    IconRun    = ""   // nf-fa-play
    IconStop   = ""   // nf-fa-stop
    IconCopy   = ""   // nf-fa-copy
    IconSave   = ""   // nf-fa-save
    IconDelete = "󰆴"   // nf-md-delete
    IconNew    = ""   // nf-fa-plus
    IconEdit   = ""   // nf-fa-pencil

    // ── Tree expand/collapse ──────────────────────────────────
    IconExpanded  = ""   // nf-fa-chevron_down
    IconCollapsed = ""   // nf-fa-chevron_right

    // ── Misc ──────────────────────────────────────────────────
    IconClock     = "󱑍"   // nf-md-timer_outline
    IconKey       = ""   // nf-fa-key
    IconSeparator = "›"
    IconCancel    = "󰜺"   // nf-md-cancel  (query cancel)
    IconPaging    = "󰒿"   // nf-md-page_next
)
```

---

### Theme System

#### `Theme` struct (`internal/ui/styles/theme.go`)

```go
type Theme struct {
    Name string

    Bg      string   // main background
    Surface string   // panel / card background
    Border  string   // inactive panel border
    Focus   string   // focused panel border + accent

    Text    string   // primary text
    Subtext string   // secondary / hints
    Muted   string   // disabled / placeholders

    Primary string   // titles, selected items
    Success string   // connected, true values
    Warning string   // connecting, caution
    Error   string   // error, false values

    SynKeyword  string   // SELECT FROM WHERE ...
    SynDML      string   // INSERT UPDATE DELETE ...
    SynFunction string   // COUNT SUM AVG ...
    SynString   string   // 'literals'
    SynNumber   string   // 42, 3.14
    SynComment  string   // -- comments
    SynOperator string   // = > < AND OR ...
}
```

#### Built-in themes (5 themes)

```go
var builtinThemes = map[string]Theme{
    "catppuccin-mocha": {
        Name:    "Catppuccin Mocha",
        Bg:      "#1e1e2e", Surface: "#313244", Border: "#45475a", Focus: "#89b4fa",
        Text:    "#cdd6f4", Subtext: "#a6adc8", Muted: "#6c7086",
        Primary: "#89b4fa", Success: "#a6e3a1", Warning: "#f9e2af", Error: "#f38ba8",
        SynKeyword: "#cba6f7", SynDML: "#f38ba8", SynFunction: "#94e2d5",
        SynString:  "#fab387", SynNumber: "#f9e2af", SynComment: "#6c7086", SynOperator: "#89b4fa",
    },
    "catppuccin-latte": {
        Name:    "Catppuccin Latte",
        Bg:      "#eff1f5", Surface: "#e6e9ef", Border: "#bcc0cc", Focus: "#1e66f5",
        Text:    "#4c4f69", Subtext: "#5c5f77", Muted: "#9ca0b0",
        Primary: "#1e66f5", Success: "#40a02b", Warning: "#df8e1d", Error: "#d20f39",
        SynKeyword: "#8839ef", SynDML: "#d20f39", SynFunction: "#179299",
        SynString:  "#fe640b", SynNumber: "#df8e1d", SynComment: "#9ca0b0", SynOperator: "#1e66f5",
    },
    "dracula": {
        Name:    "Dracula",
        Bg:      "#282a36", Surface: "#383a59", Border: "#44475a", Focus: "#bd93f9",
        Text:    "#f8f8f2", Subtext: "#6272a4", Muted: "#6272a4",
        Primary: "#bd93f9", Success: "#50fa7b", Warning: "#f1fa8c", Error: "#ff5555",
        SynKeyword: "#bd93f9", SynDML: "#ff5555", SynFunction: "#8be9fd",
        SynString:  "#f1fa8c", SynNumber: "#bd93f9", SynComment: "#6272a4", SynOperator: "#ff79c6",
    },
    "tokyo-night": {
        Name:    "Tokyo Night",
        Bg:      "#1a1b26", Surface: "#24283b", Border: "#414868", Focus: "#7aa2f7",
        Text:    "#c0caf5", Subtext: "#9aa5ce", Muted: "#565f89",
        Primary: "#7aa2f7", Success: "#9ece6a", Warning: "#e0af68", Error: "#f7768e",
        SynKeyword: "#bb9af7", SynDML: "#f7768e", SynFunction: "#2ac3de",
        SynString:  "#9ece6a", SynNumber: "#ff9e64", SynComment: "#565f89", SynOperator: "#89ddff",
    },
    "gruvbox": {
        Name:    "Gruvbox Dark",
        Bg:      "#282828", Surface: "#3c3836", Border: "#504945", Focus: "#83a598",
        Text:    "#ebdbb2", Subtext: "#d5c4a1", Muted: "#928374",
        Primary: "#83a598", Success: "#b8bb26", Warning: "#fabd2f", Error: "#fb4934",
        SynKeyword: "#d3869b", SynDML: "#fb4934", SynFunction: "#8ec07c",
        SynString:  "#b8bb26", SynNumber: "#d79921", SynComment: "#928374", SynOperator: "#83a598",
    },
}
```

#### Theme resolution — decoupled from `*Config`

`Resolve` receives only the fields it needs, not the entire config pointer.
This keeps `styles` loosely coupled and easier to test in isolation.

```go
// Resolve returns the active Theme.
// themeName: the ID string from config.Settings.Theme
// custom:    the ThemeConfig block from config.Theme.Custom (only used when themeName == "custom")
func Resolve(themeName string, custom config.ThemeConfig) Theme {
    if themeName == "custom" {
        return FromConfig(custom)
    }
    if t, ok := builtinThemes[themeName]; ok {
        return t
    }
    return builtinThemes["catppuccin-mocha"] // fallback
}

// FromConfig converts a config.ThemeConfig into a Theme.
func FromConfig(tc config.ThemeConfig) Theme { ... }

// Names returns all available theme IDs in display order.
func Names() []string {
    return []string{
        "catppuccin-mocha", "catppuccin-latte",
        "dracula", "tokyo-night", "gruvbox", "custom",
    }
}
```

Call site in `app.go`:
```go
// On startup and on ThemeChangedMsg:
m.styles = styles.ToStyles(styles.Resolve(m.config.Settings.Theme, m.config.Theme.Custom))
```

#### `Styles` struct and `ToStyles()`

```go
type Styles struct {
    PanelFocused   lipgloss.Style
    PanelUnfocused lipgloss.Style
    PanelError     lipgloss.Style
    Title          lipgloss.Style
    Text           lipgloss.Style
    Subtext        lipgloss.Style
    Muted          lipgloss.Style
    Success        lipgloss.Style
    Warning        lipgloss.Style
    Error          lipgloss.Style
    TableHeader    lipgloss.Style
    TableRow       lipgloss.Style
    TableSelected  lipgloss.Style
    TablePK        lipgloss.Style
    TreeSelected   lipgloss.Style
    TreeConn       lipgloss.Style
    TreeSchema     lipgloss.Style
    TreeTable      lipgloss.Style
    TreeView       lipgloss.Style
    StatusBar      lipgloss.Style
    StatusConn     lipgloss.Style
    StatusMuted    lipgloss.Style
    SynKeyword     lipgloss.Style
    SynDML         lipgloss.Style
    SynFunction    lipgloss.Style
    SynString      lipgloss.Style
    SynNumber      lipgloss.Style
    SynComment     lipgloss.Style
    SynOperator    lipgloss.Style
}

func ToStyles(t Theme) Styles { ... }
```

---

### Panel border states

```
INACTIVE                FOCUSED                  ERROR
╭──────────────╮        ╭──────────────╮         ╭──────────────╮
│              │        │              │         │              │
╰──────────────╯        ╰──────────────╯         ╰──────────────╯
Border (Muted)          Focus (Blue)             Error (Red)
```

---

### SQL Syntax Highlight

| Token        | Keywords                                              | Style        |
|--------------|-------------------------------------------------------|--------------|
| `SynKeyword` | SELECT FROM WHERE JOIN ON AS ORDER GROUP BY HAVING …  | Bold + Mauve |
| `SynDML`     | INSERT UPDATE DELETE CREATE DROP ALTER TABLE VIEW …   | Red          |
| `SynFunction`| COUNT SUM AVG MIN MAX COALESCE NULLIF CAST NOW …      | Teal         |
| `SynString`  | `'...'`                                               | Peach        |
| `SynNumber`  | integers, floats, `NULL`, `TRUE`, `FALSE`             | Yellow       |
| `SynComment` | `-- ...` and `/* ... */`                              | Muted italic |
| `SynOperator`| `= > < <> >= <= AND OR NOT IN IS LIKE BETWEEN`        | Blue         |

---

### Results table — value coloring

`bubbles/table` accepts pre-styled strings with ANSI codes.
Apply lipgloss **before** inserting rows in `results.Update()`:

| Value          | Color   |
|----------------|---------|
| `true` / `1`   | Success |
| `false` / `0`  | Error   |
| `NULL`         | Muted   |
| negative number| Warning |
| anything else  | Text    |

---

## Config & Secrets Layer

### `internal/config/config.go`

```go
type Config struct {
    Settings    Settings     `toml:"settings"`
    Keybinds    Keybinds     `toml:"keybinds"`
    Theme       ThemeSection `toml:"theme"`
    Connections []Connection `toml:"connections"`
}

type Settings struct {
    Theme string `toml:"theme"` // one of: catppuccin-mocha | catppuccin-latte | dracula | tokyo-night | gruvbox | custom
}

// ThemeSection wraps [theme]; custom colors live under [theme.custom].
type ThemeSection struct {
    Custom ThemeConfig `toml:"custom"`
}

type Keybinds struct {
    RunQuery         string `toml:"run_query"`           // default: "ctrl+enter"
    CancelQuery      string `toml:"cancel_query"`        // default: "ctrl+x"
    NewConnection    string `toml:"new_connection"`      // default: "n"
    DeleteConnection string `toml:"delete_connection"`   // default: "d"
    ToggleSidebar    string `toml:"toggle_sidebar"`      // default: "ctrl+b"
    FocusEditor      string `toml:"focus_editor"`        // default: "ctrl+e"
    FocusResults     string `toml:"focus_results"`       // default: "ctrl+r"
    OpenSettings     string `toml:"open_settings"`       // default: "s"
    FilterSidebar    string `toml:"filter_sidebar"`      // default: "/"
    Quit             string `toml:"quit"`                // default: "q"
    Help             string `toml:"help"`                // default: "?"
    CopyCell         string `toml:"copy_cell"`           // default: "y"
    FollowFK         string `toml:"follow_fk"`           // default: "F"
    NextPage         string `toml:"next_page"`           // default: "ctrl+f"
    PrevPage         string `toml:"prev_page"`           // default: "ctrl+u"
    // Panel resize — like tmux prefix+arrow
    ResizePanelLeft  string `toml:"resize_panel_left"`  // default: "alt+left"
    ResizePanelRight string `toml:"resize_panel_right"` // default: "alt+right"
    ResizePanelUp    string `toml:"resize_panel_up"`    // default: "alt+up"
    ResizePanelDown  string `toml:"resize_panel_down"`  // default: "alt+down"
}

// ThemeConfig: only used when Settings.Theme == "custom".
// config package does NOT import styles — no toTheme() method here.
type ThemeConfig struct {
    Bg          string `toml:"bg"`
    Surface     string `toml:"surface"`
    Border      string `toml:"border"`
    Focus       string `toml:"focus"`
    Text        string `toml:"text"`
    Subtext     string `toml:"subtext"`
    Muted       string `toml:"muted"`
    Primary     string `toml:"primary"`
    Success     string `toml:"success"`
    Warning     string `toml:"warning"`
    Error       string `toml:"error"`
    SynKeyword  string `toml:"keyword"`
    SynDML      string `toml:"dml_word"`
    SynFunction string `toml:"function"`
    SynString   string `toml:"str"`
    SynNumber   string `toml:"number"`
    SynComment  string `toml:"comment"`
    SynOperator string `toml:"operator"`
}

type Connection struct {
    Name        string `toml:"name"`
    Driver      string `toml:"driver"`      // "postgres" | "mysql" | "sqlserver"
    Host        string `toml:"host"`
    Port        int    `toml:"port"`
    User        string `toml:"user"`
    Database    string `toml:"database"`
    SSLMode     string `toml:"ssl_mode"`    // postgres: "disable" | "require" | "verify-full"
    SSLCert     string `toml:"ssl_cert"`    // path to client cert file (optional)
    SSLKey      string `toml:"ssl_key"`     // path to client key file (optional)
    SSLRootCert string `toml:"ssl_rootcert"`// path to CA cert file (optional)
    // Password NOT stored here — lives encrypted in .secrets
}
```

#### Keybind conflict validation

Called in `config.Load()` and in the Settings keybinds tab before saving a remap:

```go
// ValidateKeybinds returns a list of human-readable conflict descriptions.
// A conflict is when two different actions share the same key string.
func ValidateKeybinds(kb Keybinds) []string {
    seen := map[string]string{} // key → actionName
    var conflicts []string
    check := func(key, action string) {
        if prev, ok := seen[key]; ok {
            conflicts = append(conflicts,
                fmt.Sprintf("%q is bound to both %q and %q", key, prev, action))
        }
        seen[key] = action
    }
    check(kb.RunQuery,         "run_query")
    check(kb.CancelQuery,      "cancel_query")
    check(kb.NewConnection,    "new_connection")
    check(kb.DeleteConnection, "delete_connection")
    check(kb.ToggleSidebar,    "toggle_sidebar")
    check(kb.FocusEditor,      "focus_editor")
    check(kb.FocusResults,     "focus_results")
    check(kb.OpenSettings,     "open_settings")
    check(kb.FilterSidebar,    "filter_sidebar")
    check(kb.Quit,             "quit")
    check(kb.Help,             "help")
    check(kb.CopyCell,         "copy_cell")
    check(kb.FollowFK,         "follow_fk")
    check(kb.NextPage,         "next_page")
    check(kb.PrevPage,         "prev_page")
    return conflicts
}
```

If `ValidateKeybinds` returns any conflicts on `Load()`, the app shows a warning
modal listing them and falls back to defaults for the conflicting keys.

#### Full `config.toml` example

```toml
[settings]
theme = "catppuccin-mocha"

[keybinds]
run_query         = "ctrl+enter"
cancel_query      = "ctrl+x"
new_connection    = "n"
delete_connection = "d"
toggle_sidebar    = "ctrl+b"
focus_editor      = "ctrl+e"
focus_results     = "ctrl+r"
open_settings     = "s"
filter_sidebar    = "/"
quit              = "q"
help              = "?"
copy_cell         = "y"
follow_fk         = "F"
next_page         = "ctrl+f"
prev_page         = "ctrl+u"

[theme.custom]
bg       = "#0d1117"
surface  = "#161b22"
border   = "#30363d"
focus    = "#58a6ff"
text     = "#e6edf3"
subtext  = "#8b949e"
muted    = "#6e7681"
primary  = "#58a6ff"
success  = "#3fb950"
warning  = "#d29922"
error    = "#f85149"
keyword  = "#ff7b72"
dml_word = "#f85149"
function = "#79c0ff"
str      = "#a5d6ff"
number   = "#f2cc60"
comment  = "#6e7681"
operator = "#58a6ff"

[[connections]]
name         = "local-postgres"
driver       = "postgres"
host         = "localhost"
port         = 5432
user         = "admin"
database     = "mydb"
ssl_mode     = "disable"

[[connections]]
name         = "prod-postgres"
driver       = "postgres"
host         = "db.example.com"
port         = 5432
user         = "app_user"
database     = "production"
ssl_mode     = "verify-full"
ssl_cert     = "~/.config/dbterm/certs/client.crt"
ssl_key      = "~/.config/dbterm/certs/client.key"
ssl_rootcert = "~/.config/dbterm/certs/ca.crt"
```

#### Functions

```go
func Load() (*Config, error)      // reads file; creates with defaults if missing; validates keybinds
func (c *Config) Save() error     // writes TOML preserving comments (go-toml/v2)
func DefaultConfig() *Config      // all defaults including theme and keybinds
func ConfigDir() string           // resolves ~/.config/dbterm/
```

---

### `internal/config/secrets.go`

```
key   = scrypt(machineID, salt, N=32768, r=8, p=1) → 32 bytes
enc   = AES-256-GCM(key, nonce, plaintext)
store = base64(nonce + ciphertext)  per connName in JSON
file permissions: 0600
```

`machineID` = `sha256(hostname + os.Getenv("USER"))` — ties secrets to local machine.

```go
type SecretsStore map[string]string

func LoadSecrets() (SecretsStore, error)
func (s SecretsStore) Save() error
func (s SecretsStore) SetPassword(connName, password string) error
func (s SecretsStore) GetPassword(connName string) (string, error)
func (s SecretsStore) DeletePassword(connName string)
func machineKey() ([]byte, error)
```

---

## DB Layer

### `internal/types/types.go`

No internal imports. Pure data types shared across `db` and `app/messages`.

```go
package types

type Column struct {
    Name       string
    DataType   string
    IsNullable bool
    IsPK       bool
}

type Table struct {
    Name    string
    Schema  string
    IsView  bool
    Columns []Column // populated lazily on tree expand
}

type Schema struct {
    Name   string
    Tables []Table
}

type ConnState int
const (
    StateDisconnected ConnState = iota
    StateConnecting
    StateConnected
    StateError
)
```

---

### `internal/db/manager.go` — thread-safe connection pool

`Manager` is accessed from the main goroutine (Bubble Tea Update) AND from
`tea.Cmd` goroutines running concurrently. All map reads and writes must be
protected with `sync.RWMutex`.

```go
type ConnEntry struct {
    Config config.Connection
    DB     *gorm.DB
    State  types.ConnState
    Err    error
    cancel context.CancelFunc // cancels the current running query, if any
}

type Manager struct {
    mu    sync.RWMutex
    conns map[string]*ConnEntry
}

func NewManager() *Manager

// Connect opens a GORM connection asynchronously.
// Returns a tea.Cmd that fires ConnectedMsg or ConnectErrorMsg.
func (m *Manager) Connect(c config.Connection, password string) tea.Cmd

// Disconnect closes the connection and removes the entry.
func (m *Manager) Disconnect(connName string) error

// RunQuery executes SQL asynchronously with a cancellable context.
// Stores the CancelFunc so CancelQuery() can interrupt it.
// Returns a tea.Cmd that fires QueryResultMsg.
func (m *Manager) RunQuery(connName, sql string) tea.Cmd

// CancelQuery cancels the currently running query for the given connection.
// No-op if no query is running.
func (m *Manager) CancelQuery(connName string)

// Get returns a read-only copy of the ConnEntry (nil if not found).
func (m *Manager) Get(connName string) *ConnEntry

// All returns a snapshot of all ConnEntries (safe to iterate).
func (m *Manager) All() []*ConnEntry

// Ping sends a lightweight query ("SELECT 1") to check liveness.
// Used by keepalive.go.
func (m *Manager) Ping(connName string) error
```

#### Thread-safe implementation pattern

```go
func (m *Manager) RunQuery(connName, sql string) tea.Cmd {
    return func() tea.Msg {
        m.mu.Lock()
        entry, ok := m.conns[connName]
        if !ok {
            m.mu.Unlock()
            return messages.QueryResultMsg{Err: fmt.Errorf("connection %q not found", connName)}
        }
        // cancel any previous query for this connection
        if entry.cancel != nil {
            entry.cancel()
        }
        ctx, cancel := context.WithCancel(context.Background())
        entry.cancel = cancel
        db := entry.DB
        m.mu.Unlock()

        start := time.Now()
        rows, err := db.WithContext(ctx).Raw(sql).Rows()
        if err != nil {
            return messages.QueryResultMsg{
                Err: fmt.Errorf("executing query on %q: %w", connName, err),
            }
        }
        defer rows.Close()

        cols, _ := rows.Columns()
        var result [][]string
        for rows.Next() {
            vals := make([]interface{}, len(cols))
            ptrs := make([]interface{}, len(cols))
            for i := range vals { ptrs[i] = &vals[i] }
            rows.Scan(ptrs...)
            row := make([]string, len(cols))
            for i, v := range vals {
                if v == nil { row[i] = "NULL" } else { row[i] = fmt.Sprintf("%v", v) }
            }
            result = append(result, row)
        }
        return messages.QueryResultMsg{
            Columns: cols,
            Rows:    result,
            Elapsed: time.Since(start),
        }
    }
}
```

---

### `internal/db/keepalive.go`

Prevents silent connection drops on idle connections.

```go
const pingInterval = 3 * time.Minute

// StartKeepalive returns a tea.Cmd that ticks every pingInterval
// and pings all connected entries. Fires KeepaliveTickMsg each tick.
func StartKeepalive() tea.Cmd {
    return tea.Tick(pingInterval, func(t time.Time) tea.Msg {
        return messages.KeepaliveTickMsg{At: t}
    })
}
```

In `app.Update()`, on `KeepaliveTickMsg`:
```go
case messages.KeepaliveTickMsg:
    var cmds []tea.Cmd
    for _, entry := range m.db.All() {
        if entry.State == types.StateConnected {
            name := entry.Config.Name
            cmds = append(cmds, func() tea.Msg {
                if err := m.db.Ping(name); err != nil {
                    return messages.ConnectErrorMsg{ConnName: name, Err: err}
                }
                return nil
            })
        }
    }
    cmds = append(cmds, db.StartKeepalive()) // re-arm
    return m, tea.Batch(cmds...)
```

---

### `internal/db/introspect.go`

```go
func LoadSchemas(db *gorm.DB, driver string) ([]types.Schema, error)
func LoadColumns(db *gorm.DB, driver, schema, table string) ([]types.Column, error)
```

SQL per driver:
```sql
-- postgres
SELECT table_schema, table_name, table_type
FROM information_schema.tables
WHERE table_schema NOT IN ('pg_catalog','information_schema')
ORDER BY table_schema, table_name;

-- mysql
SELECT table_schema, table_name, table_type
FROM information_schema.tables WHERE table_schema = DATABASE()
ORDER BY table_name;

-- sqlserver
SELECT s.name AS table_schema, t.name AS table_name, 'BASE TABLE' AS table_type
FROM sys.tables t JOIN sys.schemas s ON t.schema_id = s.schema_id
ORDER BY s.name, t.name;
```

---

### `internal/clipboard/clipboard.go`

Cross-platform abstraction over `atotto/clipboard`.
Handles macOS (`pbcopy`), Linux X11 (`xclip`/`xsel`), Wayland (`wl-copy`), WSL.
`atotto/clipboard` handles all of this transparently — the wrapper just adds
error handling and a fallback message.

```go
package clipboard

import "github.com/atotto/clipboard"

// Copy writes text to the system clipboard.
// Returns an error if the clipboard is unavailable (e.g. headless server).
func Copy(text string) error {
    if err := clipboard.WriteAll(text); err != nil {
        return fmt.Errorf("clipboard unavailable: %w", err)
    }
    return nil
}
```

In `results.Update()` on `y` keypress:
```go
case "y":
    cell := m.getFocusedCell()
    if err := clipboard.Copy(cell); err != nil {
        // show brief status bar message: "clipboard unavailable"
    } else {
        // show brief status bar message: "copied!"
    }
```

---

### `internal/db/drivers.go`

```go
func BuildDSN(c config.Connection, password string) (string, error)
func NewDialector(c config.Connection, password string) (gorm.Dialector, error)
```

DSN formats (including SSL fields):
```
postgres:
  host=localhost user=admin password=x dbname=mydb port=5432
  sslmode=verify-full sslcert=/path/client.crt sslkey=/path/client.key sslrootcert=/path/ca.crt

mysql:
  admin:password@tcp(localhost:3306)/mydb?charset=utf8mb4&parseTime=True&tls=custom

sqlserver:
  sqlserver://admin:password@localhost:1433?database=mydb&encrypt=true&trustServerCertificate=false
```

---

## Bubble Tea Architecture

### Root model (`internal/app/app.go`)

```go
type PanelID int
const (
    PanelSidebar PanelID = iota
    PanelEditor
    PanelResults
)

type Model struct {
    sidebar   sidebar.Model
    editor    editor.Model
    results   results.Model
    settings  settings.Model
    modal     *modal.Model
    statusbar statusbar.Model

    activePanel  PanelID
    showSettings bool
    showHelp     bool
    queryRunning bool        // true while a query tea.Cmd is in flight
    width        int
    height       int

    db      *db.Manager
    config  *config.Config
    secrets config.SecretsStore
    styles  styles.Styles
}
```

### All message types (`internal/app/messages.go`)

```go
package messages

import (
    "time"
    "github.com/<user>/db-term/internal/config"
    "github.com/<user>/db-term/internal/types"
    tea "github.com/charmbracelet/bubbletea"
)

// ── Connection lifecycle ──────────────────────────────────────
type ConnectRequestMsg struct{ Conn config.Connection; Password string }
type ConnectedMsg      struct{ ConnName string }
type ConnectErrorMsg   struct{ ConnName string; Err error }
type DisconnectMsg     struct{ ConnName string }

// ── Schema loading ────────────────────────────────────────────
type SchemaLoadedMsg   struct{ ConnName string; Schemas []types.Schema }

// ── Query execution ───────────────────────────────────────────
type RunQueryMsg    struct{ ConnName string; SQL string }
type CancelQueryMsg struct{ ConnName string }
type QueryResultMsg struct {
    Columns []string
    Rows    [][]string
    Elapsed time.Duration
    Err     error
}

// ── Pagination ────────────────────────────────────────────────
type NextPageMsg struct{}
type PrevPageMsg struct{}

// ── Sidebar ───────────────────────────────────────────────────
type OpenTableMsg       struct{ ConnName, SchemaName, TableName string }
type ShowNewConnFormMsg struct{}
type DeleteConnMsg      struct{ ConnName string }

// ── Active connection ─────────────────────────────────────────
type SetActiveConnMsg struct{ ConnName string; SchemaName string }

// ── Keepalive ─────────────────────────────────────────────────
type KeepaliveTickMsg struct{ At time.Time }

// ── Modals ────────────────────────────────────────────────────
type ShowModalMsg struct{ Title, Body string; Actions []ModalAction }
type ModalAction  struct{ Label string; Msg tea.Msg }
type HideModalMsg struct{}

// ── Driver activation ─────────────────────────────────────────
type DriverActivateMsg struct{ Driver string; OnAccept tea.Msg }

// ── Theme ─────────────────────────────────────────────────────
type ThemeChangedMsg struct{ ThemeID string }

// ── Clipboard feedback ────────────────────────────────────────
type ClipboardFeedbackMsg struct{ Err error } // nil = success
```

### `Update()` — message routing

```go
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {

    case tea.WindowSizeMsg:
        // store m.width, m.height; recalculate panel sizes; propagate to all sub-models

    case tea.KeyMsg:
        // Priority order:
        // 1. if modal visible → route exclusively to modal
        // 2. if showSettings  → route exclusively to settings
        // 3. global shortcuts:
        //    cancel_query keybind  → CancelQueryMsg if m.queryRunning
        //    n     → ShowNewConnFormMsg
        //    d     → DeleteConnMsg (focused conn in sidebar)
        //    s     → m.showSettings = true
        //    ?     → m.showHelp toggle
        //    q/^C  → disconnect all, tea.Quit
        //    ctrl+b→ toggle sidebar
        //    ctrl+e→ PanelEditor
        //    ctrl+r→ PanelResults
        //    Tab   → cycle panels
        // 4. delegate to activePanel

    case messages.ConnectRequestMsg:
        // set sidebar node to StateConnecting; fire db.Connect() tea.Cmd

    case messages.ConnectedMsg:
        // set sidebar node to StateConnected
        // fire introspect.LoadSchemas as tea.Cmd

    case messages.ConnectErrorMsg:
        // set sidebar node to StateError; show error modal

    case messages.SchemaLoadedMsg:
        // sidebar.SetSchemas(msg.ConnName, msg.Schemas)

    case messages.SetActiveConnMsg:
        // m.editor.SetConn(msg.ConnName, msg.SchemaName)
        // m.statusbar.SetConn(msg.ConnName, msg.SchemaName)

    case messages.RunQueryMsg:
        // m.queryRunning = true
        // fire db.RunQuery() as tea.Cmd

    case messages.CancelQueryMsg:
        // m.db.CancelQuery(msg.ConnName)
        // m.queryRunning = false

    case messages.QueryResultMsg:
        // m.queryRunning = false
        // results.SetResult(msg); if msg.Err != nil → show error modal

    case messages.NextPageMsg:
        // results.NextPage()

    case messages.PrevPageMsg:
        // results.PrevPage()

    case messages.OpenTableMsg:
        // fire RunQueryMsg with SELECT * FROM schema.table LIMIT 200
        // fire SetActiveConnMsg

    case messages.KeepaliveTickMsg:
        // ping all connected DBs; re-arm keepalive tick

    case messages.ThemeChangedMsg:
        // m.config.Settings.Theme = msg.ThemeID; m.config.Save()
        // m.styles = styles.ToStyles(styles.Resolve(m.config.Settings.Theme, m.config.Theme.Custom))
        // propagate m.styles to all sub-models

    case messages.ClipboardFeedbackMsg:
        // show brief statusbar message: "copied!" or "clipboard unavailable"

    case messages.ShowModalMsg:
        // m.modal = &modal.New(msg.Title, msg.Body, msg.Actions, m.styles)

    case messages.HideModalMsg:
        // m.modal = nil
    }
}
```

---

## Active Connection Model

When multiple connections are open, the editor and results panels need to know
which connection to use. This is managed through a single `activeConn` field.

### Rules

1. **On sidebar Enter on a table node:** fires `SetActiveConnMsg{ConnName, SchemaName}`.
   The editor's header and the status bar update to reflect the new active connection.
2. **On `RunQueryMsg`:** the editor always sends its stored `activeConn`.
   If `activeConn` is empty (no connection selected), the editor shows a red inline
   message: `No connection selected — click a table in the sidebar`.
3. **On connection disconnect:** if `activeConn` was that connection, reset to `""`.
4. **Visual indicator:** the editor panel border title shows `local-pg › public`.

```
╭─  󰦕 Editor ─── local-pg › public ─────────── [Ctrl+↵] Run ─╮
│                                                              │
│  ...                                                         │
╰──────────────────────────────────────────────────────────────╯
```

If no connection is active:
```
╭─  󰦕 Editor ─── no connection selected ─────────────────────╮
│                                                              │
│  Select a table in the sidebar to set the active connection. │
╰──────────────────────────────────────────────────────────────╯
```

---

## UI Layer — Panels

### Sidebar (`internal/ui/panels/sidebar/`)

#### Visual specification

```
╭─  Connections ──────────────╮
│                             │
│  󰆼  local-pg       ●        │
│   ▼  public                 │
│      󰓫 Tables               │
│      │    users        │
│      │    orders       │
│      󰒔 Views               │
│          active_users       │
│                             │
│  󰆼  mysql-dev      ○        │
│  󰆼  sqlserver      ◌  ⠋    │
│                             │
│   New [n]   Delete [d]     │
╰─────────────────────────────╯
```

Filter mode (activated by `/`):
```
╭─  Connections ──────────────╮
│  󰈿 Filter: users_           │  ← filter input replaces hint bar
│ ─────────────────────────── │
│  󰆼  local-pg       ●        │
│      󰓫 Tables               │
│           users        │  ← only matching nodes shown
│           users_log    │
╰─────────────────────────────╯
```

#### `filter.go`

```go
// FilterNodes returns a flattened list of nodes whose Label contains query (case-insensitive).
// Parent nodes (connection, schema, group) are always shown if they have at least one match.
func FilterNodes(roots []*tree.TreeNode, query string) []*tree.TreeNode
```

#### `sidebar.go` key behavior

```
↑ / k       move cursor up
↓ / j       move cursor down
Enter / →   expand node; on table/view → fire OpenTableMsg + SetActiveConnMsg
←           collapse node
n           fire ShowNewConnFormMsg
d           fire DeleteConnMsg
/           enter filter mode (shows filter input in sidebar footer)
Esc         exit filter mode, restore full tree
```

#### `tree.go` types

```go
type NodeKind int
const (
    NodeConnection NodeKind = iota
    NodeDatabase             // database within a connection instance (lazy-loaded schemas)
    NodeSchema
    NodeTableGroup
    NodeViewGroup
    NodeTable
    NodeView
)

type TreeNode struct {
    Kind       NodeKind
    Label      string
    ConnName   string
    DBName     string          // database name for NodeDatabase and below
    SchemaName string
    Expanded   bool
    Loading    bool            // true while schemas are loading asynchronously
    Children   []*TreeNode
    State      types.ConnState // meaningful only for NodeConnection
}

func BuildConnectionNode(connName string, state types.ConnState, databases []string) *TreeNode
func BuildTree(connName string, state types.ConnState, schemas []types.Schema) *TreeNode
func AttachSchemas(dbNode *TreeNode, schemas []types.Schema)
func FlatList(root *TreeNode) []*TreeNode

// FlatItem pairs a node with its pre-computed tree-connector prefix for rendering.
type FlatItem struct {
    Node   *TreeNode
    Prefix string // e.g. "├─ " or "│  └─ "
}

func FlatItems(roots []*TreeNode) []FlatItem
```

---

### Editor (`internal/ui/panels/editor/editor.go`)

```go
type Model struct {
    textarea     textarea.Model
    activeConn   string
    activeSchema string
    width        int
    height       int
    focused      bool
    styles       styles.Styles
    keybinds     config.Keybinds
}
```

Key behavior:
```
run_query keybind    fire RunQueryMsg{ConnName: m.activeConn, SQL: m.textarea.Value()}
Ctrl+A               select all
Everything else      passed to bubbles/textarea
```

---

### Results (`internal/ui/panels/results/`)

#### Pagination (`pagination.go`)

Result sets are split into pages of `pageSize = 500` rows.
All rows are held in memory; only the current page is rendered.

```go
type PagedResult struct {
    AllRows  [][]string
    PageSize int        // default 500
    Page     int        // 0-indexed current page
}

func (p *PagedResult) CurrentRows() [][]string  // rows for current page
func (p *PagedResult) TotalPages() int
func (p *PagedResult) NextPage()                // no-op if on last page
func (p *PagedResult) PrevPage()                // no-op if on first page
```

Status bar suffix when paginated:
```
42,000 rows  󱑍 0.82s    Page 1 / 84   [Ctrl+F] Next  [Ctrl+U] Prev
```

#### Column truncation

Long cell values are truncated with `…` to a max display width of `maxCellWidth = 40` chars.
The full value is still in `AllRows` and is what gets copied to clipboard with `y`.

```go
func truncateCell(s string, max int) string {
    if len([]rune(s)) <= max {
        return s
    }
    return string([]rune(s)[:max-1]) + "…"
}
```

#### `results.go` model

```go
type Model struct {
    paged    PagedResult
    table    table.Model
    columns  []string
    elapsed  time.Duration
    rowCount int
    err      error
    querying bool       // true while query is in flight → show spinner
    width    int
    height   int
    focused  bool
    styles   styles.Styles
}
```

Visual while query is running:
```
╭─  󰓪 Results ─────────────────────────────────────────────────╮
│                                                               │
│          ⠋  Executing query…    [Ctrl+X] Cancel              │
│                                                               │
╰───────────────────────────────────────────────────────────────╯
```

Key behavior:
```
↑ / k        scroll up
↓ / j        scroll down
← / h        scroll columns left
→ / l        scroll columns right
y            copy focused cell (truncated display value NOT used — copies full value)
next_page    fire NextPageMsg
prev_page    fire PrevPageMsg
```

---

### Settings (`internal/ui/panels/settings/`)

Split into four files for maintainability. Each tab is its own model.

#### `settings.go` — root

```go
type Tab int
const (
    TabKeybinds Tab = iota
    TabConnections
    TabTheme
)

type Model struct {
    activeTab   Tab
    keybinds    tabKeybinds.Model
    connections tabConnections.Model
    theme       tabTheme.Model
    config      *config.Config
    styles      styles.Styles
    width       int
    height      int
}
```

Tab switching: `Tab` / `Shift+Tab` while settings is open.

---

#### `tab_keybinds.go` — keybinds tab with conflict validation

```
╭─  Settings ─────────────────────────────────────────────────────────╮
│  [ 󰌌 Keybinds ]   [ Connections ]   [ 󰉦 Theme ]                     │
│                                                                      │
│  Action              Current      New (editing)                      │
│  ──────────────────  ───────────  ──────────────                     │
│  Run query           ctrl+enter                                      │
│  Cancel query        ctrl+x                                          │
│▶ Toggle sidebar      ctrl+b       _            ← inline input active │
│  Focus editor        ctrl+e                                          │
│  ...                                                                 │
│                                                                      │
│  ⚠  No conflicts detected                                            │
│                                                                      │
│  Enter to edit · Esc to cancel · conflicts shown in red              │
╰──────────────────────────────────────────────────────────────────────╯
```

If a conflict is introduced while editing:
```
│  ✗  "ctrl+b" is already bound to "toggle_sidebar"    ← Error color  │
```

The Save button is disabled while conflicts exist.

---

#### `tab_theme.go` — theme tab

```
╭─  Settings ─────────────────────────────────────────────────────────╮
│  [ 󰌌 Keybinds ]   [ Connections ]   [ 󰉦 Theme ]                     │
│                                                                      │
│  Active:  catppuccin-mocha                                           │
│                                                                      │
│▶  catppuccin-mocha    ██ ██ ██ ██   Dark · purple-blue               │
│   catppuccin-latte    ██ ██ ██ ██   Light · soft tones               │
│   dracula             ██ ██ ██ ██   Dark · classic purple            │
│   tokyo-night         ██ ██ ██ ██   Dark · midnight blue             │
│   gruvbox             ██ ██ ██ ██   Dark · warm earthtones           │
│   custom              ██ ██ ██ ██   Defined in config.toml           │
│                                                                      │
│  Swatches: Primary · Success · Warning · Error                       │
│  Navigate to preview · Enter to apply · Esc to revert                │
╰──────────────────────────────────────────────────────────────────────╯
```

---

#### `tab_connections.go` — connections tab

```
╭─  Settings ─────────────────────────────────────────────────────────╮
│  [ 󰌌 Keybinds ]   [ Connections ]   [ 󰉦 Theme ]                     │
│                                                                      │
│▶  󰆼  local-postgres      postgres   localhost:5432    ●              │
│   󰆼  prod-postgres       postgres   db.example.com   ○              │
│   󰆼  dev-mysql           mysql      127.0.0.1:3306   ○              │
│                                                                      │
│  [n] New   [Enter] Edit   [d] Delete   [t] Test connection           │
╰──────────────────────────────────────────────────────────────────────╯
```

---

## UI Layer — Components

### Modal (`internal/ui/components/modal.go`)

```go
type Model struct {
    Title   string
    Body    string
    Actions []messages.ModalAction
    cursor  int
    width   int
    styles  styles.Styles
}

func New(title, body string, actions []messages.ModalAction, s styles.Styles) Model
// ← →   move between buttons
// Enter  dispatch actions[cursor].Msg
// Esc    dispatch HideModalMsg
```

---

### Connection Form (`internal/ui/components/connform.go`)

```go
type Model struct {
    step         int                 // 0=driver | 1=activation | 2=fields | 3=testing
    driver       string
    inputs       []textinput.Model   // Name, Host, Port, User, Database, Password, SSLMode
    focusIndex   int
    testing      bool
    testErr      error
    activatedFor map[string]bool     // tracks which drivers shown activation modal this session
    width        int
    styles       styles.Styles
}
```

Steps visual:
```
Step 0 — Driver picker
  ▶  󰆼  PostgreSQL  /  󰆼  MySQL  /  󰆼  SQL Server

Step 1 — Activation (first use of that driver type per session only)
  Driver : gorm.io/driver/postgres | Backend: pgx/v5 | Status: bundled 󰄬

Step 2 — Fields
  Name / Host / Port / User / Database / Password (masked) / SSL Mode (postgres only)

Step 3 — Test result
  Connecting: ◌  spinner
  Success:    󰄬  Connected!           [← Back]  [Save]
  Failure:     <error message>  [← Back to fix fields]
```

---

### Status Bar (`internal/ui/components/statusbar.go`)

```go
type Model struct {
    activeConn   string
    activeSchema string
    rowCount     int
    totalRows    int        // total across all pages
    currentPage  int
    totalPages   int
    elapsed      time.Duration
    message      string     // transient feedback: "copied!", "clipboard unavailable"
    messageTimer int        // countdown ticks to clear message
    queryRunning bool
    width        int
    styles       styles.Styles
    keybinds     config.Keybinds
}
```

Renders:
```
Normal:
 󰆼 local-pg  ›  public   500/42000 rows  Page 1/84  󱑍 0.08s   [Tab] Panel  [s] Settings  [?] Help

Query running:
 󰆼 local-pg  ›  public   ⠋ Executing…  [Ctrl+X] Cancel

Transient feedback (3 seconds, then clears):
 󰆼 local-pg  ›  public   ✓ Copied to clipboard!

No connection:
 No connection   Press [n] to add one      [s] Settings  [?] Help  [q] Quit
```

---

## UI Layout & Navigation

### Panel dimensions

```
sidebarWidth  = 26 cols (0 when toggled off)
editorHeight  = int(availableRows * 0.40)
resultsHeight = availableRows - editorHeight
statusHeight  = 1 row (always visible)
```

### Layout rendering

```go
func Render(s sidebar.Model, e editor.Model, r results.Model, sb statusbar.Model, w, h int) string {
    rightCol := lipgloss.JoinVertical(lipgloss.Left, e.View(), r.View())
    main     := lipgloss.JoinHorizontal(lipgloss.Top, s.View(), rightCol)
    return lipgloss.JoinVertical(lipgloss.Left, main, sb.View())
}
```

### Focus & global shortcuts

```
Tab cycles:   Sidebar → Editor → Results → Sidebar

Global (always active unless modal/settings is open):
  n           ShowNewConnFormMsg
  d           DeleteConnMsg (focused sidebar connection)
  s           open Settings
  ?           toggle Help overlay
  q / Ctrl+C  quit (disconnect all, tea.Quit)
  Ctrl+B      toggle sidebar width
  Ctrl+E      focus Editor
  Ctrl+R      focus Results
  /           sidebar filter (only when Sidebar is focused)
  Ctrl+X      CancelQueryMsg (only when queryRunning == true)
```

---

## Cross-Cutting Concerns

### Concurrency & safety summary

| Concern | Solution |
|---|---|
| Map reads/writes from multiple goroutines | `sync.RWMutex` in `Manager` |
| Long-running query blocking UI | `tea.Cmd` runs in background goroutine |
| Cancelling an in-flight query | `context.WithCancel`; `CancelFunc` stored in `ConnEntry` |
| Idle connections dropping | Keepalive ping every 3 minutes via `tea.Tick` |

### Error handling strategy

All errors are wrapped at the point of origin with context using `fmt.Errorf`:

```go
// ✓ correct — provides context for debugging
return fmt.Errorf("introspecting schemas for %q: %w", connName, err)

// ✗ wrong — loses context
return err
```

Errors surface to the user in two ways:
- **Query errors:** red banner at top of results panel
- **Connection errors:** modal dialog with the full error message
- **Config errors:** modal on startup; falls back to defaults

### Testing strategy

#### Unit tests (no external dependencies)

| Package | What to test | File |
|---|---|---|
| `config` | `DefaultConfig()` has no empty fields; `ValidateKeybinds()` detects conflicts | `config_test.go` |
| `config` | `SetPassword` + `GetPassword` round-trip | `secrets_test.go` |
| `ui/styles` | `Resolve()` falls back to catppuccin-mocha for unknown theme | `theme_test.go` |
| `ui/styles` | `ToStyles()` produces non-zero lipgloss styles for each theme | `theme_test.go` |
| `db` | `BuildDSN()` output matches expected format per driver | `drivers_test.go` |
| `ui/panels/sidebar` | `FlatList()` returns correct depth-first order | `tree_test.go` |
| `ui/panels/sidebar` | `FilterNodes()` returns only matching nodes | `filter_test.go` |
| `ui/panels/results` | `PagedResult.CurrentRows()` returns correct slice | `pagination_test.go` |
| `ui/panels/results` | `truncateCell()` truncates at correct rune boundary | `pagination_test.go` |

#### Integration tests (require a real database, tagged `//go:build integration`)

```go
//go:build integration

// Run with: go test ./internal/db/... -tags integration
```

| Test | Requires |
|---|---|
| Connect to Postgres, run `SELECT 1`, verify no error | local Postgres |
| `LoadSchemas()` returns at least one schema | local Postgres |
| `RunQuery()` with long-running query + `CancelQuery()` returns context error | local Postgres |
| Keepalive ping does not error on fresh connection | local Postgres |

#### TUI snapshot tests

Bubble Tea models can be snapshot-tested by calling `View()` directly
without running a full program:

```go
func TestEditorView_NoConnection(t *testing.T) {
    m := editor.New(styles.ToStyles(styles.Resolve("catppuccin-mocha", config.ThemeConfig{})), config.DefaultConfig().Keybinds)
    m.SetSize(80, 20)
    got := m.View()
    // assert contains "no connection selected"
    if !strings.Contains(got, "no connection selected") {
        t.Errorf("expected 'no connection selected' in view, got:\n%s", got)
    }
}
```

---

## Implementation Phases

### Phase 1 — Scaffolding
**Goal:** project compiles; `dbterm` boots to a blank styled frame; `q` exits.

1. `go mod init` + all `go get` commands from [Tech Stack](#tech-stack--dependencies)
2. Create full directory tree
3. Add `Makefile`, `.golangci.yml`, `.github/workflows/ci.yml`
4. Write `internal/types/types.go`
5. Write minimal `internal/app/app.go` (empty Model/Init/Update/View)
6. Write `cmd/main.go`: `config.Load()` → `db.NewManager()` → `app.New()` → `tea.NewProgram(..., tea.WithAltScreen()).Run()`
7. **Verify:** `make build` succeeds; `make test` passes; binary boots and exits on `q`

---

### Phase 2 — Config & Secrets
**Goal:** config auto-created with defaults, comments preserved on save; passwords round-trip correctly.

1. Implement `config.go` with all structs (`Config`, `Settings`, `ThemeSection`, `ThemeConfig`, `Keybinds`, `Connection`)
2. `DefaultConfig()` fills all fields including new `CancelQuery`, `FilterSidebar`, `NextPage`, `PrevPage` keybinds
3. `Load()` creates `~/.config/dbterm/`; writes default TOML; runs `ValidateKeybinds()` and shows warning on conflicts
4. `Save()` uses `go-toml/v2` encoder to preserve comments
5. Implement `secrets.go`: `machineKey()`, `SetPassword()`, `GetPassword()`, `LoadSecrets()`, `Save()`
6. **Verify:** `make test` — secrets round-trip; default keybinds have no conflicts; config file is created on first run

---

### Phase 3 — Visual Design System
**Goal:** all theme/style/icon infrastructure in place.
> Moved before DB Layer so the visual scaffold works early and UI development can proceed in parallel.

1. Write `icons.go` with all constants
2. Write `theme.go`:
   - `Theme` struct, `builtinThemes` map (5 themes, exact hex values)
   - `Resolve(themeName string, custom config.ThemeConfig) Theme` — no `*Config` pointer
   - `FromConfig(tc config.ThemeConfig) Theme`
   - `Names()`, `Styles` struct, `ToStyles(t Theme) Styles`
3. **Verify:** unit tests in `theme_test.go`; render test printing each theme's border to stdout

---

### Phase 4 — DB Layer
**Goal:** can connect, run a query, introspect schemas, cancel queries, and survive idle timeouts.

1. `drivers.go`: `BuildDSN()` for all 3 drivers including SSL fields; `NewDialector()` switch
2. `manager.go`: thread-safe with `sync.RWMutex`; `Connect()`, `RunQuery()` with `context.WithCancel`, `CancelQuery()`, `Disconnect()`, `Ping()`, `All()`
3. `introspect.go`: `LoadSchemas()` with driver-specific SQL; `LoadColumns()` (lazy)
4. `keepalive.go`: `StartKeepalive()` returning a `tea.Tick` cmd
5. `clipboard/clipboard.go`: thin wrapper over `atotto/clipboard`
6. **Verify:** integration test (`-tags integration`) — connect, query, cancel long query, verify context error

---

### Phase 5 — Panels
**Goal:** each panel renders and handles keyboard events independently.

#### 5a — Sidebar
1. `tree.go`: `TreeNode`, `BuildTree()`, `FlatList()`
2. `filter.go`: `FilterNodes(roots, query)`
3. `sidebar.go`: full Model, Init, Update (including filter mode), View
4. On `Enter` over table → fire `OpenTableMsg` + `SetActiveConnMsg`

#### 5b — Editor
1. Init `bubbles/textarea`; set placeholder
2. `Update()`: `run_query` keybind → `RunQueryMsg`; show "no connection" message if `activeConn == ""`
3. `View()`: token scanner for SQL highlight; panel title shows `activeConn › activeSchema`

#### 5c — Results
1. Init `bubbles/table`; init `PagedResult`
2. `Update()`: `QueryResultMsg` → pre-color cells → `PagedResult.AllRows = rows` → `SetPage(0)` → populate table
3. `Update()`: `NextPageMsg`/`PrevPageMsg` → update paged result → repopulate table
4. `View()`: spinner if `querying`; error banner; table; footer with row/page counts

#### 5d — Settings (fully specified)
1. `settings.go`: root model with tab switching (`Tab`/`Shift+Tab`)
2. `tab_theme.go`:
   - List of themes with color swatches (`██ ██ ██ ██` = Primary, Success, Warning, Error)
   - Navigate preview: each cursor move fires `ThemeChangedMsg` without saving
   - `Enter` saves and fires `ThemeChangedMsg`; `Esc` fires `ThemeChangedMsg` with the previously-saved theme ID
3. `tab_keybinds.go`:
   - Editable list of all keybinds
   - `Enter` on a row enables inline `textinput.Model` for that row
   - Every keystroke in the input runs `ValidateKeybinds()` and shows conflicts in red inline
   - Save disabled while conflicts exist; auto-save on `Enter` with no conflicts
4. `tab_connections.go`:
   - List of saved connections with driver, host, state
   - `n` → `ShowNewConnFormMsg`; `d` → `DeleteConnMsg`; `Enter` → edit (opens connform pre-filled); `t` → test connection

---

### Phase 6 — Components
**Goal:** modal, connection form, and status bar work end-to-end.

1. `modal.go`: centered box, action buttons, `Enter`/`Esc`
2. `connform.go`: all 4 steps; `activatedFor` map; async test via `tea.Cmd`; SSL fields shown only for postgres
3. `statusbar.go`: full render including page info, transient messages (3-tick countdown), query-running state
4. Wire `n` → `ShowModalMsg` containing connform in `app.Update()`

---

### Phase 7 — App Integration
**Goal:** full end-to-end flow; all messages route correctly.

1. Flesh out all cases in `app.Update()` as specified in [message routing section](#update--message-routing)
2. `layout.go`: `Render()` composing all four views
3. Keepalive: `app.Init()` returns `tea.Batch(sidebar.Init(), editor.Init(), results.Init(), db.StartKeepalive())`
4. `ThemeChangedMsg`: save config → re-resolve styles → propagate to all sub-models
5. `tea.WindowSizeMsg`: recalculate dimensions → propagate
6. **Verify end-to-end:** launch → add connection → connect → select table → query runs → results paginate → cancel query → change theme → restart → theme persists

---

### Phase 8 — Polish
**Goal:** production-quality feel; no rough edges.

1. `bubbles/spinner` in sidebar node while connecting; in results panel while query runs
2. Help overlay (`?`): two-column shortcut list rendered as a modal
3. Graceful quit: `Disconnect()` all connections in `app.Update()` on `tea.Quit`
4. Terminal size guard: `width < 80 || height < 24` → centered resize prompt instead of normal UI
5. First-run hint: if `len(config.Connections) == 0`, sidebar shows `Press [n] to add your first connection`
6. Sidebar toggle: `ctrl+b` sets `sidebarWidth` to 0 or 26; layout recalculates immediately
7. **Run full test suite:** `make test && make lint`

---

## Future Roadmap (post-MVP)

| Feature                      | Notes                                               |
|------------------------------|-----------------------------------------------------|
| Query history                | Persist in `~/.config/dbterm/history.db` (SQLite)   |
| Export results (CSV / JSON)  | `e` key from results panel                          |
| Autocomplete in editor       | Table/column names from loaded schema               |
| Table data editor            | Edit rows inline → generate UPDATE/INSERT           |
| Multiple editor tabs         | One tab per query session                           |
| SSH tunnel support           | Jump host config per connection                     |
| `.env` / `DATABASE_URL`      | Auto-detect from current project directory          |
| Connection groups / folders  | Organize connections in sidebar                     |
| Column details panel         | Full column list + types on table select            |
| Additional themes            | Nord, One Dark, Solarized, Rose Pine                |
| Panel resizing               | Drag sidebar width; adjustable editor/results split |
