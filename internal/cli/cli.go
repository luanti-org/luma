// Package cli runs non-interactive luma commands, in the form <noun> <verb> [flags] or <command> [flags].
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

	cmdName string // e.g. "mods update" or "install"
	cmd     verb
}

type handler func(c *ctx, args []string) int

type verb struct {
	usage   string // what follows "luma <noun> <verb>" or "luma <command>", usually flags and params: "[-n] [name...]"
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
	"games": {
		summary: "manage installed games",
		verbs: map[string]verb{
			"list":     {"[--names]", "list installed games", gamesList},
			"outdated": {"[--names]", "list games with a newer release on ContentDB", gamesOutdated},
			"update":   {"[-n] [-x a,b] [id...]", "update the named games, or all outdated games if none are named", gamesUpdate},
		},
	},
	"mods": {
		summary: "manage installed mods",
		verbs: map[string]verb{
			"install-deps": {"[-n] [-y] [--optional] [--game id] name", "install the missing required dependencies of a mod, or its optional ones", modsInstallDeps},
			"list":         {"[--names]", "list installed mods", modsList},
			"list-deps":    {"[--names] [--optional] [--game id] name", "list the required or optional dependencies of a mod and whether they're installed", modsListDeps},
			"outdated":     {"[--names]", "list mods with a newer release on ContentDB", modsOutdated},
			"update":       {"[-n] [-y] [-x a,b] [--no-deps] [--game id] [name...]", "update the named mods, or all outdated mods if none are named, with any new dependencies", modsUpdate},
		},
	},
	"texturepacks": {
		summary: "manage installed texture packs",
		verbs: map[string]verb{
			"list":     {"[--names]", "list installed texture packs", texturepacksList},
			"outdated": {"[--names]", "list texture packs with a newer release on ContentDB", texturepacksOutdated},
			"update":   {"[-n] [-x a,b] [name...]", "update the named texture packs, or all outdated texture packs if none are named", texturepacksUpdate},
		},
	},
}

// topLevel holds the commands with no noun, for getting content of any type. Their names must not be nouns.
var topLevel = map[string]verb{
	"install": {"[-n] [-y] [--no-deps] [--game id] name|author/name...", "install ContentDB packages, with the dependencies of mods", install},
}

// aliases maps alternative noun names to their entry in commands, help lists only the latter
var aliases = map[string]string{
	"textures": "texturepacks",
}

// Command is a top-level command or command group, as listed in help
type Command struct {
	Name    string
	Summary string
}

// Commands returns the top-level commands and command groups in name order
func Commands() []Command {
	out := make([]Command, 0, len(commands)+len(topLevel))
	for name, n := range commands {
		out = append(out, Command{Name: name, Summary: n.summary})
	}
	for name, v := range topLevel {
		out = append(out, Command{Name: name, Summary: v.summary})
	}
	slices.SortFunc(out, func(a, b Command) int { return strings.Compare(a.Name, b.Name) })
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
		return c.usageErr("expected a command")
	}

	if v, ok := topLevel[args[0]]; ok {
		c.cmdName = args[0]
		c.cmd = v
		return v.run(c, args[1:])
	}

	nounName := args[0]
	if canonical, ok := aliases[nounName]; ok {
		nounName = canonical
	}

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
