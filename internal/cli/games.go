package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"text/tabwriter"

	"github.com/luanti-org/luma/internal/content"
	"github.com/luanti-org/luma/internal/update"
)

func gamesList(c *ctx, args []string) int {
	fs := c.newFlagSet()
	namesOnly := fs.Bool("names", false, "print only game ids, one per line")
	if code, done := c.parseFlags(fs, args); done {
		return code
	}

	games, ok := c.scanGames()
	if !ok {
		return exitError
	}

	if *namesOnly {
		for _, g := range games {
			fmt.Fprintln(c.stdout, g.ID)
		}
		return exitOK
	}

	tw := tabwriter.NewWriter(c.stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tAUTHOR\tRELEASE\tTITLE")

	for _, g := range games {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", g.ID, orDash(g.Author), releaseStr(g.Release), g.Title)
	}

	tw.Flush()

	return exitOK
}

func gamesOutdated(c *ctx, args []string) int {
	fs := c.newFlagSet()
	namesOnly := fs.Bool("names", false, "print only game ids, one per line")
	if code, done := c.parseFlags(fs, args); done {
		return code
	}

	games, ok := c.scanGames()
	if !ok {
		return exitError
	}

	updates, err := update.CheckAllGameUpdates(games, c.cdb, c.eng)
	if err != nil {
		return c.err("checking for updates: %v", err)
	}

	if *namesOnly {
		for _, u := range updates {
			fmt.Fprintln(c.stdout, u.Game.ID)
		}
		return exitOK
	}

	if len(updates) == 0 {
		fmt.Fprintln(c.stdout, "All games are up to date.")
		return exitOK
	}

	printGameUpdates(c.stdout, updates)

	return exitOK
}

func gamesUpdate(c *ctx, args []string) int {
	fs := c.newFlagSet()
	dryRun := fs.BoolP("dry-run", "n", false, "show what would be updated without changing anything")
	excludeList := fs.StringP("exclude", "x", "", "comma-separated game ids to skip")
	if code, done := c.parseFlagsWithArgs(fs, args); done {
		return code
	}

	games, ok := c.scanGames()
	if !ok {
		return exitError
	}

	installed := make(map[string]content.Game, len(games))
	for _, g := range games {
		installed[g.ID] = g
	}

	ids := dedupe(fs.Args())
	excluded := dedupe(splitList(*excludeList))

	// checked before any network use so a typo fails fast and a mistyped --exclude can't update what it meant to skip
	var unknown []string
	for _, id := range append(append([]string(nil), ids...), excluded...) {
		if _, ok := installed[id]; !ok {
			unknown = append(unknown, id)
		}
	}
	if len(unknown) > 0 {
		return c.err("not installed: %s", strings.Join(dedupe(unknown), ", "))
	}

	updates, err := update.CheckAllGameUpdates(games, c.cdb, c.eng)
	if err != nil {
		return c.err("checking for updates: %v", err)
	}

	plan := planGameUpdates(c, installed, updates, ids, excluded)

	if len(plan) == 0 {
		fmt.Fprintln(c.stdout, "Nothing to update.")
		return exitOK
	}

	if *dryRun {
		fmt.Fprintln(c.stdout, "Would update:")
		printGameUpdates(c.stdout, plan)
		return exitOK
	}

	failed := 0
	for _, u := range plan {
		if err := update.UpdateGame(c.cdb, u); err != nil {
			failed++
			fmt.Fprintf(c.stderr, "luma: %s: update failed: %v\n", u.Game.ID, err)
			continue
		}
		fmt.Fprintf(c.stdout, "%s: updated %s -> %d\n", u.Game.ID, releaseStr(u.Game.Release), u.LatestRelease)
	}

	if failed > 0 {
		return c.err("%d of %d updates failed", failed, len(plan))
	}

	return exitOK
}

// planGameUpdates picks which of updates to apply: those named (in order), or all when ids is empty, minus excluded.
func planGameUpdates(c *ctx, installed map[string]content.Game, updates []update.GameUpdate, ids, excluded []string) []update.GameUpdate {
	skip := make(map[string]bool, len(excluded))
	for _, id := range excluded {
		skip[id] = true
	}

	if len(ids) == 0 {
		var plan []update.GameUpdate
		for _, u := range updates {
			if !skip[u.Game.ID] {
				plan = append(plan, u)
			}
		}
		return plan
	}

	byID := make(map[string]update.GameUpdate, len(updates))
	for _, u := range updates {
		byID[u.Game.ID] = u
	}

	var plan []update.GameUpdate
	for _, id := range ids {
		if skip[id] {
			continue
		}
		u, ok := byID[id]
		if !ok {
			fmt.Fprintf(c.stdout, "%s: %s\n", id, noGameUpdateReason(installed[id]))
			continue
		}
		plan = append(plan, u)
	}

	return plan
}

func noGameUpdateReason(g content.Game) string {
	if g.Author == "" || g.Release <= 0 {
		return "can't check for updates, no author or release in game.conf"
	}
	return "no update available"
}

func printGameUpdates(w io.Writer, updates []update.GameUpdate) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tCURRENT\tLATEST")

	for _, u := range updates {
		fmt.Fprintf(tw, "%s\t%s\t%d\n", u.Game.ID, releaseStr(u.Game.Release), u.LatestRelease)
	}

	tw.Flush()
}

// scanGames scans the detected games dir, reporting any error itself. ok is false on failure.
func (c *ctx) scanGames() (games []content.Game, ok bool) {
	if c.eng.GamesDir == "" {
		c.err("no Luanti install detected, pass --dir")
		return nil, false
	}

	games, err := content.ScanGames(c.eng.GamesDir)
	// a flatpak install that was never launched has no games dir yet
	if c.eng.ViaFlatpak && errors.Is(err, fs.ErrNotExist) {
		return nil, true
	}
	if err != nil {
		c.err("scanning games: %v", err)
		return nil, false
	}

	return games, true
}
