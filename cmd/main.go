package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gabrielfloresousion/db-term/internal/app"
	"github.com/gabrielfloresousion/db-term/internal/config"
	"github.com/gabrielfloresousion/db-term/internal/db"
)

func main() {
	cfg, conflicts, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "dbterm: failed to load config: %v\n", err)
		os.Exit(1)
	}
	if len(conflicts) > 0 {
		fmt.Fprintln(os.Stderr, "dbterm: keybind conflicts detected — resetting to defaults:")
		for _, c := range conflicts {
			fmt.Fprintf(os.Stderr, "  %s\n", c)
		}
	}

	secrets, err := config.LoadSecrets()
	if err != nil {
		fmt.Fprintf(os.Stderr, "dbterm: failed to load secrets: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(
		app.New(cfg, secrets, db.NewManager()),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "dbterm: %v\n", err)
		os.Exit(1)
	}
}
