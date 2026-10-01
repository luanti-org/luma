package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/pflag"

	"github.com/luanti-org/luma/internal/cli"
	"github.com/luanti-org/luma/internal/contentdb"
	"github.com/luanti-org/luma/internal/engine"
	"github.com/luanti-org/luma/internal/help"
	"github.com/luanti-org/luma/internal/tui"
)

// engineVersionsTimeout bounds the startup ContentDB lookup, so a stalled server can't hang detection
const engineVersionsTimeout = 15 * time.Second

func main() {
	root := pflag.StringP("dir", "d", "", "path to the Luanti install. Defaults to the current directory.")
	flatpak := pflag.Bool("flatpak", false, "use the Luanti flatpak install ("+engine.DefaultFlatpakAppID+")")
	engineVersion := pflag.StringP("engine-version", "e", "", "engine version to assume if it can't be detected, e.g. 5.17")

	pflag.Usage = func() { help.Print(pflag.CommandLine.Output(), pflag.CommandLine) }
	pflag.CommandLine.SetInterspersed(false) // everything from the command on belongs to the cli

	if err := cli.CheckFlagStyle(pflag.CommandLine, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "luma:", err)
		os.Exit(2)
	}

	pflag.Parse()

	if *flatpak && *root != "" {
		fmt.Fprintln(os.Stderr, "luma: --dir and --flatpak can't be used together")
		os.Exit(2)
	}

	cdb := contentdb.New("")
	cdb.HTTPClient = &http.Client{Timeout: engineVersionsTimeout}

	engineInfo, warn, fatal := engine.Detect(engine.Options{
		Dir:           *root,
		Flatpak:       *flatpak,
		EngineVersion: *engineVersion,
		Versions:      cdb.EngineVersions,
	})
	if fatal != nil {
		fmt.Fprintln(os.Stderr, "luma:", fatal)
		os.Exit(1)
	}

	if pflag.NArg() > 0 {
		if warn != nil {
			fmt.Fprintln(os.Stderr, "luma: warning:", warn)
		}
		os.Exit(cli.Run(pflag.Args(), engineInfo, os.Stdout, os.Stderr))
	}

	p := tea.NewProgram(tui.New(engineInfo), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Println("error running program:", err)
		os.Exit(1)
	}
}
