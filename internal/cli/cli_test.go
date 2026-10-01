package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/luanti-org/luma/internal/contentdb"
	"github.com/luanti-org/luma/internal/engine"
)

func run(t *testing.T, eng engine.Info, args ...string) (code int, stdout, stderr string) {
	t.Helper()

	return runWithCDB(t, eng, nil, args...)
}

func runWithCDB(t *testing.T, eng engine.Info, cdb *contentdb.Client, args ...string) (code int, stdout, stderr string) {
	t.Helper()

	var out, errOut bytes.Buffer
	c := &ctx{eng: eng, cdb: cdb, stdout: &out, stderr: &errOut}
	code = c.run(args)

	return code, out.String(), errOut.String()
}

// withTestCommands swaps in a fake command table for the duration of the test.
func withTestCommands(t *testing.T) {
	t.Helper()

	prev := commands
	t.Cleanup(func() { commands = prev })

	list := func(c *ctx, args []string) int {
		fs := c.newFlagSet()
		fs.Bool("names", false, "")
		code, _ := c.parseFlags(fs, args)
		return code
	}
	update := func(c *ctx, args []string) int {
		fs := c.newFlagSet()
		fs.StringP("exclude", "x", "", "")
		code, _ := c.parseFlagsWithArgs(fs, args)
		return code
	}

	commands = map[string]noun{
		"things": {summary: "manage things", verbs: map[string]verb{
			"list":   {"[--names]", "list things", list},
			"update": {"[-x a,b] [name...]", "update things", update},
		}},
		"apples": {summary: "manage apples", verbs: map[string]verb{
			"list": {"", "list apples", list},
		}},
	}
}

func TestRunUsageErrors(t *testing.T) {
	withTestCommands(t)

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no verb", []string{"things"}, "Verbs:"},
		{"unknown noun", []string{"worlds", "list"}, `unknown command "worlds"`},
		{"unknown verb", []string{"things", "frobnicate"}, `unknown command "things frobnicate"`},
		{"extra args", []string{"things", "list", "extra"}, "unexpected arguments"},
		{"unknown flag", []string{"things", "list", "--dir", "x"}, "unknown flag: --dir"},
		{"single dash long flag", []string{"things", "list", "-names"}, "use --names, not -names"},
		{"single dash long flag with value", []string{"things", "update", "-exclude=alpha"}, "use --exclude=alpha, not -exclude=alpha"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _, stderr := run(t, engine.Info{}, tt.args...)
			if code != exitUsage {
				t.Errorf("code = %d, want %d", code, exitUsage)
			}
			if !strings.Contains(stderr, tt.want) {
				t.Errorf("stderr = %q, want it to contain %q", stderr, tt.want)
			}
		})
	}
}

func TestNounsSorted(t *testing.T) {
	withTestCommands(t)

	nouns := Nouns()
	for i := 1; i < len(nouns); i++ {
		if nouns[i-1].Name >= nouns[i].Name {
			t.Errorf("Nouns not sorted: %q before %q", nouns[i-1].Name, nouns[i].Name)
		}
	}
	for _, n := range nouns {
		if n.Summary == "" {
			t.Errorf("noun %q has no summary", n.Name)
		}
	}
}
