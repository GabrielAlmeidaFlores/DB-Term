// Package styles is the single source of truth for all visual decisions in db-term.
// No panel or component defines colors or icons inline — everything comes from here.
package styles

const (
	IconDatabase  = "󰆼" // nf-md-database
	IconPostgres  = "" // nf-dev-postgresql
	IconMySQL     = "" // nf-dev-mysql
	IconSQLServer = "" // nf-dev-windows
	IconTable     = "󰓫" // nf-md-table
	IconView      = "󰒔" // nf-md-eye_outline
	IconSchema    = "" // nf-fa-folder_open
	IconColumn    = "󰠵" // nf-md-table_column
	IconPK        = "" // nf-fa-key
	IconNullable  = "󰌾" // nf-md-null
	IconFunction  = "" // nf-fa-code
	IconIndex     = "󰆖" // nf-md-lightning_bolt

	IconConnected    = "●" // filled circle
	IconDisconnected = "○" // empty circle
	IconConnecting   = "◌" // dotted circle
	IconError        = "" // nf-fa-warning

	IconEditor      = "󰦕" // nf-md-code_braces
	IconResults     = "󰓪" // nf-md-table_large
	IconSidebar     = "" // nf-fa-sitemap
	IconConnections = "󱘖" // nf-md-connection
	IconSettings    = "" // nf-fa-cog
	IconHelp        = "󰋗" // nf-md-help_circle_outline
	IconHistory     = "󱑁" // nf-md-history
	IconTheme       = "󰉦" // nf-md-palette
	IconFilter      = "󰈿" // nf-md-filter_outline

	IconRun    = "" // nf-fa-play
	IconStop   = "" // nf-fa-stop
	IconCopy   = "" // nf-fa-copy
	IconSave   = "" // nf-fa-save
	IconDelete = "󰆴" // nf-md-delete
	IconNew    = "" // nf-fa-plus
	IconEdit   = "" // nf-fa-pencil

	IconExpanded  = "" // nf-fa-chevron_down
	IconCollapsed = "" // nf-fa-chevron_right

	IconClock = "󱑍" // nf-md-timer_outline
	IconKey   = ""  // nf-fa-key
	// IconFK marks a foreign-key cell value.
	IconFK        = ""
	IconSeparator = "›" // breadcrumb separator
	IconCancel    = "󰜺" // nf-md-cancel
	IconPaging    = "󰒿" // nf-md-page_next
)
