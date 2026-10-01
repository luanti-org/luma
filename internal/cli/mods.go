package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/luanti-org/luma/internal/content"
	"github.com/luanti-org/luma/internal/update"
)

func modsList(c *ctx, args []string) int {
	fs := c.newFlagSet()
	namesOnly := fs.Bool("names", false, "print only mod names, one per line")
	if code, done := c.parseFlags(fs, args); done {
		return code
	}

	mods, ok := c.scanMods()
	if !ok {
		return exitError
	}

	if *namesOnly {
		for _, m := range mods {
			fmt.Fprintln(c.stdout, m.Name)
		}
		return exitOK
	}

	tw := tabwriter.NewWriter(c.stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tAUTHOR\tRELEASE")

	for _, m := range mods {
		name := m.Name
		if m.IsModpack {
			name += " [P]"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", name, orDash(m.Author), releaseStr(m.Release))
	}

	tw.Flush()

	return exitOK
}

func modsOutdated(c *ctx, args []string) int {
	fs := c.newFlagSet()
	namesOnly := fs.Bool("names", false, "print only mod names, one per line")
	if code, done := c.parseFlags(fs, args); done {
		return code
	}

	mods, ok := c.scanMods()
	if !ok {
		return exitError
	}

	updates, err := update.CheckAllModUpdates(mods, c.cdb, c.eng)
	if err != nil {
		return c.err("checking for updates: %v", err)
	}

	if *namesOnly {
		for _, u := range updates {
			fmt.Fprintln(c.stdout, u.Mod.Name)
		}
		return exitOK
	}

	if len(updates) == 0 {
		fmt.Fprintln(c.stdout, "All mods are up to date.")
		return exitOK
	}

	printUpdates(c, updates)

	return exitOK
}

func modsUpdate(c *ctx, args []string) int {
	fs := c.newFlagSet()
	dryRun := fs.BoolP("dry-run", "n", false, "show what would be updated without changing anything")
	excludeList := fs.StringP("exclude", "x", "", "comma-separated mod names to skip")
	if code, done := c.parseFlagsWithArgs(fs, args); done {
		return code
	}

	mods, ok := c.scanMods()
	if !ok {
		return exitError
	}

	installed := make(map[string]content.Mod, len(mods))
	for _, m := range mods {
		installed[m.Name] = m
	}

	names := dedupe(fs.Args())
	excluded := dedupe(splitList(*excludeList))

	// checked before any network use so a typo fails fast and a mistyped --exclude can't update what it meant to skip
	var unknown []string
	for _, n := range append(append([]string{}, names...), excluded...) {
		if _, ok := installed[n]; !ok {
			unknown = append(unknown, n)
		}
	}
	if len(unknown) > 0 {
		return c.err("not installed: %s", strings.Join(dedupe(unknown), ", "))
	}

	updates, err := update.CheckAllModUpdates(mods, c.cdb, c.eng)
	if err != nil {
		return c.err("checking for updates: %v", err)
	}

	plan := planUpdates(c, installed, updates, names, excluded)

	if len(plan) == 0 {
		fmt.Fprintln(c.stdout, "Nothing to update.")
		return exitOK
	}

	if *dryRun {
		fmt.Fprintln(c.stdout, "Would update:")
		printUpdates(c, plan)
		return exitOK
	}

	failed := 0
	for _, u := range plan {
		if err := update.UpdateMod(c.cdb, u); err != nil {
			failed++
			fmt.Fprintf(c.stderr, "luma: %s: update failed: %v\n", u.Mod.Name, err)
			continue
		}
		fmt.Fprintf(c.stdout, "%s: updated %s -> %d\n", u.Mod.Name, releaseStr(u.Mod.Release), u.LatestRelease)
	}

	if failed > 0 {
		return c.err("%d of %d updates failed", failed, len(plan))
	}

	return exitOK
}

// planUpdates picks which of updates to apply: those named (in order), or all when names is empty, minus excluded.
func planUpdates(c *ctx, installed map[string]content.Mod, updates []update.ModUpdate, names, excluded []string) []update.ModUpdate {
	skip := make(map[string]bool, len(excluded))
	for _, n := range excluded {
		skip[n] = true
	}

	if len(names) == 0 {
		var plan []update.ModUpdate
		for _, u := range updates {
			if !skip[u.Mod.Name] {
				plan = append(plan, u)
			}
		}
		return plan
	}

	byName := make(map[string]update.ModUpdate, len(updates))
	for _, u := range updates {
		byName[u.Mod.Name] = u
	}

	var plan []update.ModUpdate
	for _, n := range names {
		if skip[n] {
			continue
		}
		u, ok := byName[n]
		if !ok {
			fmt.Fprintf(c.stdout, "%s: %s\n", n, noUpdateReason(installed[n]))
			continue
		}
		plan = append(plan, u)
	}

	return plan
}

func noUpdateReason(m content.Mod) string {
	if m.Author != "" {
		return "no update available"
	}

	conf := "mod.conf"
	if m.IsModpack {
		conf = "modpack.conf"
	}
	return "can't check for updates, no author in " + conf
}

func printUpdates(c *ctx, updates []update.ModUpdate) {
	tw := tabwriter.NewWriter(c.stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tCURRENT\tLATEST")

	for _, u := range updates {
		fmt.Fprintf(tw, "%s\t%s\t%d\n", u.Mod.Name, releaseStr(u.Mod.Release), u.LatestRelease)
	}

	tw.Flush()
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func dedupe(in []string) []string {
	seen := make(map[string]bool, len(in))
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// scanMods scans the detected mods dir, reporting any error itself. ok is false on failure.
func (c *ctx) scanMods() (mods []content.Mod, ok bool) {
	if c.eng.ModsDir == "" {
		c.err("no Luanti install detected, pass --dir")
		return nil, false
	}

	mods, err := content.ScanMods(c.eng.ModsDir)
	// a flatpak install that was never launched has no mods dir yet
	if c.eng.ViaFlatpak && errors.Is(err, fs.ErrNotExist) {
		return nil, true
	}
	if err != nil {
		c.err("scanning mods: %v", err)
		return nil, false
	}

	return mods, true
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func releaseStr(r int) string {
	if r == 0 {
		return "-"
	}
	return strconv.Itoa(r)
}
