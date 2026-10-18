package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/luanti-org/luma/internal/content"
	"github.com/luanti-org/luma/internal/contentdb"
	"github.com/luanti-org/luma/internal/update"
)

func install(c *ctx, args []string) int {
	fs := c.newFlagSet()
	dryRun := fs.BoolP("dry-run", "n", false, "show what would be installed without changing anything")
	yes := fs.BoolP("yes", "y", false, "install dependencies without asking")
	noDeps := fs.Bool("no-deps", false, "don't install the dependencies of mods")
	gameID := fs.String("game", "", "count this game's mods as installed when resolving dependencies")
	if code, done := c.parseFlagsWithArgs(fs, args); done {
		return code
	}
	if fs.NArg() == 0 {
		return c.usageErr("%s: expected at least one package", fs.Name())
	}

	installed, ok := c.scanInstalled()
	if !ok {
		return exitError
	}

	provided, ok := c.providedModNames(installed.mods, *gameID)
	if !ok {
		return exitError
	}

	index, err := c.packageIndex()
	if err != nil {
		return c.err("%v", err)
	}

	// every package is checked before anything is installed, so a run installs all or nothing
	pkgs, ok := c.planInstall(index, installed, dedupe(fs.Args()))
	if !ok {
		return exitError
	}

	var roots []string
	for _, p := range pkgs {
		if p.Type == "mod" {
			roots = append(roots, p.Author+"/"+p.Name)
		}
	}

	var deps update.DepPlan
	if !*noDeps && len(roots) > 0 {
		deps, err = update.ResolveDeps(c.cdb, roots, provided, index, nil)
		if err != nil {
			return c.err("%v", err)
		}
	}

	c.printNotFound(deps.NotFound)

	if *dryRun {
		fmt.Fprintln(c.stdout, "Would install:")
		printPackages(c.stdout, pkgs)
		if len(deps.Install) > 0 {
			fmt.Fprintln(c.stdout, "\nWould install as dependencies:")
			printDepInstalls(c.stdout, deps.Install)
		}
		return exitOK
	}

	if len(deps.Install) > 0 && !*yes {
		if !c.interactive {
			return c.err("dependencies required (%s), rerun with -y or --no-deps", depNames(deps.Install))
		}
		fmt.Fprintln(c.stderr, "Will install:")
		printPackages(c.stderr, pkgs)
		fmt.Fprintln(c.stderr, "\nWill also install as dependencies:")
		printDepInstalls(c.stderr, deps.Install)
		if !c.askContinue() {
			return c.err("aborted, nothing was changed")
		}
	}

	type step struct {
		pkg  contentdb.Package
		note string
		run  func() error
	}

	// dependencies go first, a mod missing one would not load
	var steps []step
	for _, d := range deps.Install {
		run := func() error { return update.InstallDep(c.cdb, c.eng.ModsDir, d) }
		steps = append(steps, step{d.Package, " (dependency of " + d.RequiredBy + ")", run})
	}
	for _, p := range pkgs {
		run := func() error { return update.InstallPackage(c.cdb, c.eng, p) }
		steps = append(steps, step{p, "", run})
	}

	// every step installs into a new folder, so undoing the run is removing the folders it created
	var created []string
	for _, st := range steps {
		id := st.pkg.Author + "/" + st.pkg.Name

		dest, err := update.PackageDir(c.eng, st.pkg)
		if err == nil {
			existed := pathExists(dest)
			err = st.run()
			if !existed && pathExists(dest) {
				created = append(created, dest)
			}
		}

		if err != nil {
			fmt.Fprintf(c.stderr, "luma: %s: install failed: %v\n", id, err)
			if !c.removeAll(created) {
				return c.err("install failed, could not remove everything this run installed")
			}
			return c.err("install failed, nothing was changed")
		}

		fmt.Fprintf(c.stdout, "%s: installed %d%s\n", id, st.pkg.Release, st.note)
	}

	return exitOK
}

// removeAll removes dirs, reporting any failure itself. ok is false if one could not be removed.
func (c *ctx) removeAll(dirs []string) (ok bool) {
	ok = true
	for _, dir := range dirs {
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintf(c.stderr, "luma: removing %s: %v\n", dir, err)
			ok = false
		}
	}
	return ok
}

// installedContent is what is on disk, a missing folder counts as empty.
type installedContent struct {
	mods  []content.Mod
	names map[string]map[string]bool // by package type, then mod name, game id or texture pack name
}

// scanInstalled scans the mods, games and texture packs, reporting any error itself. ok is false on failure.
func (c *ctx) scanInstalled() (in installedContent, ok bool) {
	if c.eng.ModsDir == "" {
		c.err("no Luanti install detected, pass --dir")
		return in, false
	}

	in.names = map[string]map[string]bool{"mod": {}, "game": {}, "txp": {}}

	mods, err := content.ScanMods(c.eng.ModsDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		c.err("scanning mods: %v", err)
		return in, false
	}
	in.mods = mods
	for _, m := range mods {
		in.names["mod"][m.Name] = true
	}

	games, err := content.ScanGames(c.eng.GamesDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		c.err("scanning games: %v", err)
		return in, false
	}
	for _, g := range games {
		in.names["game"][g.ID] = true
	}

	texturepacks, err := content.ScanTextures(c.eng.TexturesDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		c.err("scanning texture packs: %v", err)
		return in, false
	}
	for _, tp := range texturepacks {
		in.names["txp"][tp.Name] = true
	}

	return in, true
}

// planInstall finds the package for each arg and checks it can be installed, reporting every problem.
// ok is false if any arg failed.
func (c *ctx) planInstall(index map[string]contentdb.Package, installed installedContent, args []string) (pkgs []contentdb.Package, ok bool) {
	ok = true
	seen := make(map[string]bool)
	dests := make(map[string]string)

	for _, arg := range args {
		p, found := c.findPackage(index, arg)
		if !found {
			ok = false
			continue
		}

		id := p.Author + "/" + p.Name
		if seen[strings.ToLower(id)] {
			continue
		}
		seen[strings.ToLower(id)] = true

		dest, err := update.PackageDir(c.eng, p)
		if err != nil {
			c.err("%s: %v", id, err)
			ok = false
			continue
		}

		if installed.names[p.Type][p.Name] || pathExists(dest) {
			c.err("%q is already installed, use 'luma %s update'", arg, typeNoun(p.Type))
			ok = false
			continue
		}

		if other, taken := dests[dest]; taken {
			c.err("%s and %s both install to %s", other, id, dest)
			ok = false
			continue
		}
		dests[dest] = id

		pkgs = append(pkgs, p)
	}

	return pkgs, ok
}

// findPackage returns the one package arg names, reporting why if there isn't one.
func (c *ctx) findPackage(index map[string]contentdb.Package, arg string) (contentdb.Package, bool) {
	found := update.FindPackages(index, arg)

	switch {
	case len(found) == 1:
		return found[0], true

	case len(found) > 1:
		c.err("%q is ambiguous, use \"author/name\" format", arg)

	case strings.Contains(arg, "/"):
		c.notInIndex(arg)

	default:
		if s, ok := update.SuggestPackage(index, arg); ok {
			c.err("%q not found on ContentDB, did you mean %s/%s, %q?", arg, s.Author, s.Name, s.Title)
		} else {
			c.err("%q not found on ContentDB", arg)
		}
	}

	return contentdb.Package{}, false
}

// notInIndex reports why the "author/name" id is missing from the package index:
// the package doesn't exist, or has no release for this engine.
func (c *ctx) notInIndex(id string) {
	author, name, _ := strings.Cut(id, "/")
	if author == "" || name == "" || strings.Contains(name, "/") {
		c.err("%q not found on ContentDB", id)
		return
	}

	_, err := c.cdb.PackageDetails(author, name)
	switch {
	case err == nil:
		version := c.eng.Version
		if version == "" {
			version = "this version"
		}
		c.err("%q has no release compatible with Luanti %s", id, version)
	case errors.Is(err, contentdb.ErrNotFound):
		c.err("%q not found on ContentDB", id)
	default:
		c.err("looking up %s: %v", id, err)
	}
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// typeNoun returns the command noun for a ContentDB package type.
func typeNoun(pkgType string) string {
	switch pkgType {
	case "game":
		return "games"
	case "txp":
		return "texturepacks"
	default:
		return "mods"
	}
}

func typeName(pkgType string) string {
	if pkgType == "txp" {
		return "texture pack"
	}
	return pkgType
}

func printPackages(w io.Writer, pkgs []contentdb.Package) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tTYPE\tRELEASE")

	for _, p := range pkgs {
		fmt.Fprintf(tw, "%s/%s\t%s\t%d\n", p.Author, p.Name, typeName(p.Type), p.Release)
	}

	tw.Flush()
}
