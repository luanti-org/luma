// Package cli runs non-interactive luma commands, in the form <noun> <verb> [flags].
package cli

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/spf13/pflag"

	"github.com/luanti-org/luma/internal/contentdb"
	"github.com/luanti-org/luma/internal/engine"
)

const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

const cdbRequestTimeout = 30 * time.Second

type ctx struct {
	eng    engine.Info
	cdb    *contentdb.Client
	stdout io.Writer
	stderr io.Writer

	stdin       io.Reader
	interactive bool // stdin is a terminal, so the user can be asked to confirm

	cmdName string // e.g. "mods update"
	cmd     verb
}

type handler func(c *ctx, args []string) int

type verb struct {
	usage   string // what follows "luma <noun> <verb>", usually flags and params: "[-n] [name...]"
	summary string
	run     handler
}

type noun struct {
	summary string
	verbs   map[string]verb
}

// commands is the registry of every noun and its verbs. Add new commands here and help picks them up.
// Each verb's handler parses its own flags from args, so nothing else in this file needs to change.
var commands = map[string]noun{
	"mods": {
		summary: "manage installed mods",
		verbs: map[string]verb{
			"list":     {"[--names]", "list installed mods", modsList},
			"outdated": {"[--names]", "list mods with a newer release on ContentDB", modsOutdated},
			"update":   {"[-n] [-y] [-x a,b] [--no-deps] [--game id] [name...]", "update the named mods, or all outdated mods if none are named, with any new dependencies", modsUpdate},
		},
	},
}

// Noun is a top-level command group, as listed in help
type Noun struct {
	Name    string
	Summary string
}

// Nouns returns the command groups in name order
func Nouns() []Noun {
	out := make([]Noun, 0, len(commands))
	for _, name := range slices.Sorted(maps.Keys(commands)) {
		out = append(out, Noun{Name: name, Summary: commands[name].summary})
	}
	return out
}

// Run executes the command in args and returns the process exit code.
func Run(args []string, eng engine.Info, stdout, stderr io.Writer) int {
	cdb := contentdb.New("")
	cdb.HTTPClient = &http.Client{Timeout: cdbRequestTimeout} // don't share/mutate http.DefaultClient

	c := &ctx{eng: eng, cdb: cdb, stdout: stdout, stderr: stderr, stdin: os.Stdin}
	c.interactive = isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())

	return c.run(args)
}

func (c *ctx) run(args []string) int {
	if len(args) == 0 {
		return c.usageErr("expected <noun> <verb>")
	}

	nounName := args[0]
	n, ok := commands[nounName]
	if !ok {
		return c.usageErr("unknown command %q", nounName)
	}

	if len(args) == 1 {
		printNounHelp(c.stderr, nounName, n)
		return exitUsage
	}
	if args[1] == "-h" || args[1] == "--help" {
		printNounHelp(c.stderr, nounName, n)
		return exitOK
	}

	v, ok := n.verbs[args[1]]
	if !ok {
		return c.usageErr("unknown command %q", nounName+" "+args[1])
	}

	c.cmdName = nounName + " " + args[1]
	c.cmd = v

	return v.run(c, args[2:])
}

func printNounHelp(w io.Writer, name string, n noun) {
	fmt.Fprintf(w, "Usage: luma %s <verb> [flags]\n\nVerbs:\n", name)

	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	for _, vn := range slices.Sorted(maps.Keys(n.verbs)) {
		fmt.Fprintf(tw, "  %s\t%s\n", vn, n.verbs[vn].summary)
	}
	tw.Flush()

	fmt.Fprintf(w, "\nRun 'luma %s <verb> --help' for its flags.\n", name)
}

// newFlagSet returns a flag set for the running command, with a usage built from its verb entry.
func (c *ctx) newFlagSet() *pflag.FlagSet {
	fs := pflag.NewFlagSet(c.cmdName, pflag.ContinueOnError)
	fs.Usage = func() {
		w := fs.Output()
		fmt.Fprintf(w, "Usage: luma %s %s\n\n%s\n", c.cmdName, c.cmd.usage, c.cmd.summary)
		if fs.HasFlags() {
			fmt.Fprintf(w, "\nFlags:\n")
			fs.PrintDefaults()
		}
	}
	return fs
}

func (c *ctx) usageErr(format string, a ...any) int {
	fmt.Fprintf(c.stderr, "luma: "+format+"\nRun 'luma --help' for usage.\n", a...)
	return exitUsage
}

func (c *ctx) err(format string, a ...any) int {
	fmt.Fprintf(c.stderr, "luma: "+format+"\n", a...)
	return exitError
}

// parseFlags parses a subcommand's flags and rejects positional args.
// done is true when the caller should return code immediately.
func (c *ctx) parseFlags(fs *pflag.FlagSet, args []string) (code int, done bool) {
	if code, done := c.parseFlagsWithArgs(fs, args); done {
		return code, true
	}

	if fs.NArg() > 0 {
		return c.usageErr("%s: unexpected arguments %q", fs.Name(), fs.Args()), true
	}

	return exitOK, false
}

// parseFlagsWithArgs is parseFlags for commands that take positional args, left in fs.Args().
func (c *ctx) parseFlagsWithArgs(fs *pflag.FlagSet, args []string) (code int, done bool) {
	fs.SetOutput(c.stderr)

	if err := CheckFlagStyle(fs, args); err != nil {
		return c.usageErr("%v", err), true
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			return exitOK, true
		}
		return c.usageErr("%s: %v", fs.Name(), err), true
	}

	return exitOK, false
}

// CheckFlagStyle rejects "-word" and "-word=v" where word is a long flag of fs,
// which pflag would otherwise read as -w with the value "ord".
func CheckFlagStyle(fs *pflag.FlagSet, args []string) error {
	for _, a := range args {
		if a == "--" {
			break
		}
		if len(a) < 3 || a[0] != '-' || a[1] == '-' {
			continue
		}

		name, _, _ := strings.Cut(a[1:], "=")
		if name == "help" || fs.Lookup(name) != nil {
			return fmt.Errorf("use -%s, not %s", a, a)
		}
	}

	return nil
}
