package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/luanti-org/luma/internal/engine"
	"github.com/luanti-org/luma/internal/tui"
	"github.com/luanti-org/luma/internal/version"
)

func main() {
	root := flag.String("dir", "", "path to the Luanti install. Leave empty to try current directory or if that fails, find flatpak install.")
	showVersion := flag.Bool("version", false, "print the luma version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("luma", version.String())
		return
	}

	dirGiven := *root != ""
	if !dirGiven {
		*root = "."
	}

	engineInfo, _ := engine.DetectAuto(*root, dirGiven)

	p := tea.NewProgram(tui.New(engineInfo), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Println("error running program:", err)
		os.Exit(1)
	}
}
