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

func texturepacksList(c *ctx, args []string) int {
	fs := c.newFlagSet()
	namesOnly := fs.Bool("names", false, "print only texture pack names, one per line")
	if code, done := c.parseFlags(fs, args); done {
		return code
	}

	texturepacks, ok := c.scanTexturepacks()
	if !ok {
		return exitError
	}

	if *namesOnly {
		for _, tp := range texturepacks {
			fmt.Fprintln(c.stdout, tp.Name)
		}
		return exitOK
	}

	tw := tabwriter.NewWriter(c.stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tAUTHOR\tRELEASE\tTITLE")

	for _, tp := range texturepacks {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", tp.Name, orDash(tp.Author), releaseStr(tp.Release), orDash(tp.Title))
	}

	tw.Flush()

	return exitOK
}

func texturepacksOutdated(c *ctx, args []string) int {
	fs := c.newFlagSet()
	namesOnly := fs.Bool("names", false, "print only texture pack names, one per line")
	if code, done := c.parseFlags(fs, args); done {
		return code
	}

	texturepacks, ok := c.scanTexturepacks()
	if !ok {
		return exitError
	}

	updates, err := update.CheckAllTexturepackUpdates(texturepacks, c.cdb, c.eng)
	if err != nil {
		return c.err("checking for updates: %v", err)
	}

	if *namesOnly {
		for _, u := range updates {
			fmt.Fprintln(c.stdout, u.Texturepack.Name)
		}
		return exitOK
	}

	if len(updates) == 0 {
		fmt.Fprintln(c.stdout, "All texture packs are up to date.")
		return exitOK
	}

	printTexturepackUpdates(c.stdout, updates)

	return exitOK
}

func texturepacksUpdate(c *ctx, args []string) int {
	fs := c.newFlagSet()
	dryRun := fs.BoolP("dry-run", "n", false, "show what would be updated without changing anything")
	excludeList := fs.StringP("exclude", "x", "", "comma-separated texture pack names to skip")
	if code, done := c.parseFlagsWithArgs(fs, args); done {
		return code
	}

	texturepacks, ok := c.scanTexturepacks()
	if !ok {
		return exitError
	}

	installed := make(map[string]content.Texturepack, len(texturepacks))
	for _, tp := range texturepacks {
		installed[tp.Name] = tp
	}

	names := dedupe(fs.Args())
	excluded := dedupe(splitList(*excludeList))

	// checked before any network use so a typo fails fast and a mistyped --exclude can't update what it meant to skip
	var unknown []string
	for _, name := range append(append([]string(nil), names...), excluded...) {
		if _, ok := installed[name]; !ok {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		return c.err("not installed: %s", strings.Join(dedupe(unknown), ", "))
	}

	updates, err := update.CheckAllTexturepackUpdates(texturepacks, c.cdb, c.eng)
	if err != nil {
		return c.err("checking for updates: %v", err)
	}

	plan := planTexturepackUpdates(c, installed, updates, names, excluded)

	if len(plan) == 0 {
		fmt.Fprintln(c.stdout, "Nothing to update.")
		return exitOK
	}

	if *dryRun {
		fmt.Fprintln(c.stdout, "Would update:")
		printTexturepackUpdates(c.stdout, plan)
		return exitOK
	}

	failed := 0
	for _, u := range plan {
		tp := u.Texturepack
		if err := update.UpdateTexturepack(c.cdb, u); err != nil {
			failed++
			fmt.Fprintf(c.stderr, "luma: %s: update failed: %v\n", tp.Name, err)
			continue
		}
		fmt.Fprintf(c.stdout, "%s: updated %s -> %d\n", tp.Name, releaseStr(tp.Release), u.LatestRelease)
	}

	if failed > 0 {
		return c.err("%d of %d updates failed", failed, len(plan))
	}

	return exitOK
}

// planTexturepackUpdates picks which of updates to apply: those named (in order), or all when names is empty, minus excluded.
func planTexturepackUpdates(c *ctx, installed map[string]content.Texturepack, updates []update.TexturepackUpdate, names, excluded []string) []update.TexturepackUpdate {
	skip := make(map[string]bool, len(excluded))
	for _, name := range excluded {
		skip[name] = true
	}

	if len(names) == 0 {
		var plan []update.TexturepackUpdate
		for _, u := range updates {
			if !skip[u.Texturepack.Name] {
				plan = append(plan, u)
			}
		}
		return plan
	}

	byName := make(map[string]update.TexturepackUpdate, len(updates))
	for _, u := range updates {
		byName[u.Texturepack.Name] = u
	}

	var plan []update.TexturepackUpdate
	for _, name := range names {
		if skip[name] {
			continue
		}
		u, ok := byName[name]
		if !ok {
			fmt.Fprintf(c.stdout, "%s: %s\n", name, noTexturepackUpdateReason(installed[name]))
			continue
		}
		plan = append(plan, u)
	}

	return plan
}

func noTexturepackUpdateReason(tp content.Texturepack) string {
	if tp.Author == "" || tp.Release <= 0 {
		return "can't check for updates, no author or release in texture_pack.conf"
	}
	return "no update available"
}

func printTexturepackUpdates(w io.Writer, updates []update.TexturepackUpdate) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tCURRENT\tLATEST")

	for _, u := range updates {
		fmt.Fprintf(tw, "%s\t%s\t%d\n", u.Texturepack.Name, releaseStr(u.Texturepack.Release), u.LatestRelease)
	}

	tw.Flush()
}

// scanTexturepacks scans the detected textures dir, reporting any error itself. ok is false on failure.
func (c *ctx) scanTexturepacks() (texturepacks []content.Texturepack, ok bool) {
	if c.eng.TexturesDir == "" {
		c.err("no Luanti install detected, pass --dir")
		return nil, false
	}

	texturepacks, err := content.ScanTextures(c.eng.TexturesDir)
	// a flatpak install that was never launched has no textures dir yet
	if c.eng.ViaFlatpak && errors.Is(err, fs.ErrNotExist) {
		return nil, true
	}
	if err != nil {
		c.err("scanning texture packs: %v", err)
		return nil, false
	}

	return texturepacks, true
}
