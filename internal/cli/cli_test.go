package cli

import (
	"bytes"
	"errors"
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
		{"unknown verb help hint", []string{"things", "frobnicate"}, "Run 'luma things --help'"},
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

func TestCommandsSorted(t *testing.T) {
	withTestCommands(t)

	cmds := Commands()
	for i := 1; i < len(cmds); i++ {
		if cmds[i-1].Name >= cmds[i].Name {
			t.Errorf("Commands not sorted: %q before %q", cmds[i-1].Name, cmds[i].Name)
		}
	}
	for _, c := range cmds {
		if c.Summary == "" {
			t.Errorf("command %q has no summary", c.Name)
		}
	}
}

func TestTopLevelNotNouns(t *testing.T) {
	for name := range topLevel {
		if _, ok := commands[name]; ok {
			t.Errorf("%q is both a top-level command and a noun", name)
		}
		if _, ok := aliases[name]; ok {
			t.Errorf("%q is both a top-level command and a noun alias", name)
		}
	}
}

func TestHelpAndUsageErrorsSkipDetection(t *testing.T) {
	tests := []struct {
		args []string
		code int
	}{
		{[]string{"mods"}, exitUsage},
		{[]string{"mods", "--help"}, exitOK},
		{[]string{"mods", "update", "--help"}, exitOK},
		{[]string{"install", "-h"}, exitOK},
		{[]string{"mods", "list", "--bogus"}, exitUsage},
		{[]string{"mods", "list", "extra"}, exitUsage},
		{[]string{"mods", "bogus"}, exitUsage},
		{[]string{"mdos", "list"}, exitUsage},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			var out, errOut bytes.Buffer
			detect := func() (engine.Info, error, error) {
				t.Fatal("engine detection ran")
				return engine.Info{}, nil, nil
			}
			c := &ctx{detect: detect, stdout: &out, stderr: &errOut}
			if code := c.run(tt.args); code != tt.code {
				t.Errorf("exit code = %d, want %d, stderr: %s", code, tt.code, errOut.String())
			}
		})
	}
}

func TestDetectionErrors(t *testing.T) {
	var out, errOut bytes.Buffer
	calls := 0
	detect := func() (engine.Info, error, error) {
		calls++
		return engine.Info{}, nil, errors.New("no Luanti install in the current directory")
	}
	c := &ctx{detect: detect, stdout: &out, stderr: &errOut}

	if code := c.run([]string{"mods", "list"}); code != exitError {
		t.Errorf("exit code = %d, want %d", code, exitError)
	}
	if calls != 1 {
		t.Errorf("detect called %d times, want 1", calls)
	}
	if got := errOut.String(); got != "luma: no Luanti install in the current directory\n" {
		t.Errorf("stderr = %q", got)
	}
}

func TestDetectionWarning(t *testing.T) {
	var out, errOut bytes.Buffer
	detect := func() (engine.Info, error, error) {
		return engine.Info{ModsDir: t.TempDir()}, errors.New("version unknown"), nil
	}
	c := &ctx{detect: detect, stdout: &out, stderr: &errOut}

	if code := c.run([]string{"mods", "list"}); code != exitOK {
		t.Errorf("exit code = %d, want %d, stderr: %s", code, exitOK, errOut.String())
	}
	if !strings.HasPrefix(errOut.String(), "luma: warning: version unknown\n") {
		t.Errorf("stderr = %q", errOut.String())
	}
}
