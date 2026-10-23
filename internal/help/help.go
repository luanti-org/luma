// Package help prints usage for both the terminal UI and the non-interactive CLI.
package help

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/pflag"

	"github.com/luanti-org/luma/internal/cli"
)

const header = `luma - manage Luanti mods, games and texture packs

Usage:
  luma [global flags]                     launch the terminal UI
  luma [global flags] <command> ...       run a non-interactive command

Global flags must come before the command.

Global flags:
`

const footer = `
Environment:
  LUMA_LUANTI_INSTALL_DIR    the install to use without --dir or --flatpak: a path, or FLATPAK for the flatpak

Run 'luma <command> --help' for its usage.
Run with no command or only --dir to open the Terminal UI.
`

// Print writes the top-level help to w: global flags and the commands.
func Print(w io.Writer, globals *pflag.FlagSet) {
	fmt.Fprint(w, header)

	prev := globals.Output()
	globals.SetOutput(w)
	globals.PrintDefaults()
	globals.SetOutput(prev)

	fmt.Fprint(w, "\nCommands:\n")
	tw := tabwriter.NewWriter(w, 0, 0, 4, ' ', 0)
	for _, n := range cli.Commands() {
		fmt.Fprintf(tw, "  %s\t%s\n", n.Name, n.Summary)
	}
	tw.Flush()

	fmt.Fprint(w, footer)
}
