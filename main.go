package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

//go:embed all.txt
var authorsData string

func main() {
	if len(os.Args) > 1 {
		flag.Usage = func() {
			fmt.Println("Uso: gutberg (sin argumentos)")
		}
		flag.Parse()
	}

	cfg, err := loadConfig()
	if err != nil {
		exitErr(fmt.Errorf("load config: %w", err))
	}

	authors, err := loadAuthorsFromEmbedded(authorsData)
	if err != nil {
		exitErr(fmt.Errorf("load authors: %w", err))
	}

	state, err := loadState(cfg.StateFile)
	if err != nil {
		exitErr(fmt.Errorf("load state: %w", err))
	}

	m, err := newModel(cfg, state, authors)
	if err != nil {
		exitErr(err)
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		exitErr(err)
	}
}

func exitErr(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
