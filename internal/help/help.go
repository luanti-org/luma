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
  luma [global flags] <noun> <verb> ...   run a non-interactive command

Global flags must come before the command.

Global flags:
`

const footer = `
Run 'luma <noun> --help' for its commands.
Run with no command or only --dir to open the Terminal UI.
`

// Print writes the top-level help to w: global flags and the command groups.
func Print(w io.Writer, globals *pflag.FlagSet) {
	fmt.Fprint(w, header)

	prev := globals.Output()
	globals.SetOutput(w)
	globals.PrintDefaults()
	globals.SetOutput(prev)

	fmt.Fprint(w, "\nCommands:\n")
	tw := tabwriter.NewWriter(w, 0, 0, 4, ' ', 0)
	for _, n := range cli.Nouns() {
		fmt.Fprintf(tw, "  %s\t%s\n", n.Name, n.Summary)
	}
	tw.Flush()

	fmt.Fprint(w, footer)
}
