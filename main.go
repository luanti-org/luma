package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/luanti-org/luma/internal/tui"
)

func main() {
	root := flag.String("dir", ".", "path to the Luanti install (contains mods/ and games/)")
	flag.Parse()

	modsDir := filepath.Join(*root, "mods")
	gamesDir := filepath.Join(*root, "games")

	p := tea.NewProgram(tui.New(modsDir, gamesDir))
	if _, err := p.Run(); err != nil {
		fmt.Println("error running program:", err)
		os.Exit(1)
	}
}
