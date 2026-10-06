package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/luanti-org/luma/internal/content"
	"github.com/luanti-org/luma/internal/contentdb"
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

	printUpdates(c.stdout, updates)

	return exitOK
}

func modsUpdate(c *ctx, args []string) int {
	fs := c.newFlagSet()
	dryRun := fs.BoolP("dry-run", "n", false, "show what would be updated without changing anything")
	excludeList := fs.StringP("exclude", "x", "", "comma-separated mod names to skip")
	yes := fs.BoolP("yes", "y", false, "install new dependencies without asking")
	noDeps := fs.Bool("no-deps", false, "don't install new dependencies of the updated mods")
	gameID := fs.String("game", "", "count this game's mods as installed when resolving dependencies")
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

	// an excluded name may be a new dependency rather than an installed mod
	var depExcludes []string
	for _, n := range excluded {
		if _, ok := installed[n]; !ok {
			depExcludes = append(depExcludes, n)
		}
	}

	// checked before any network use so a typo fails fast and a mistyped --exclude can't update what it meant to skip
	var unknown []string
	for _, n := range names {
		if _, ok := installed[n]; !ok {
			unknown = append(unknown, n)
		}
	}
	if *noDeps {
		unknown = append(unknown, depExcludes...)
	}
	if len(unknown) > 0 {
		return c.err("not installed: %s", strings.Join(dedupe(unknown), ", "))
	}

	provided := content.ProvidedModNames(mods)
	if *gameID != "" {
		gameMods, ok := c.scanGameMods(*gameID)
		if !ok {
			return exitError
		}
		maps.Copy(provided, content.ProvidedModNames(gameMods))
	}

	updates, err := update.CheckAllModUpdates(mods, c.cdb, c.eng)
	if err != nil {
		return c.err("checking for updates: %v", err)
	}

	plan := planUpdates(c, installed, updates, names, excluded)

	var deps update.DepPlan
	if !*noDeps && len(plan) > 0 {
		deps, err = c.resolveDeps(plan, provided, excluded)
		if err != nil {
			return c.err("%v", err)
		}
	}

	if unused := unusedExcludes(depExcludes, deps); len(unused) > 0 {
		return c.err("not installed and not a new dependency: %s", strings.Join(unused, ", "))
	}

	if len(plan) == 0 {
		fmt.Fprintln(c.stdout, "Nothing to update.")
		return exitOK
	}

	for _, m := range deps.NotFound {
		switch {
		case !m.Excluded:
			fmt.Fprintf(c.stderr, "luma: could not find dependency %s for %s\n", m.Dep, m.RequiredBy)
		case m.Package != "":
			fmt.Fprintf(c.stderr, "luma: skipping excluded dependency %s (%s) for %s\n", m.Dep, m.Package, m.RequiredBy)
		default:
			fmt.Fprintf(c.stderr, "luma: skipping excluded dependency %s for %s\n", m.Dep, m.RequiredBy)
		}
	}

	if *dryRun {
		fmt.Fprintln(c.stdout, "Would update:")
		printUpdates(c.stdout, plan)
		if len(deps.Install) > 0 {
			fmt.Fprintln(c.stdout, "\nWould install as dependencies:")
			printDepInstalls(c.stdout, deps.Install)
		}
		return exitOK
	}

	if len(deps.Install) > 0 && !*yes {
		if !c.interactive {
			return c.err("new dependencies required (%s), rerun with -y or --no-deps", depNames(deps.Install))
		}
		if !c.confirmDeps(plan, deps.Install) {
			return c.err("aborted, nothing was changed")
		}
	}

	// dependencies go first, an updated mod missing one would not load
	depsFailed := 0
	for _, d := range deps.Install {
		id := d.Package.Author + "/" + d.Package.Name
		if err := c.installDep(d); err != nil {
			depsFailed++
			fmt.Fprintf(c.stderr, "luma: %s: install failed: %v\n", id, err)
			continue
		}
		fmt.Fprintf(c.stdout, "%s: installed %d (dependency of %s)\n", id, d.Package.Release, d.RequiredBy)
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

	if depsFailed > 0 {
		c.err("%d of %d dependency installs failed", depsFailed, len(deps.Install))
	}
	if failed > 0 {
		c.err("%d of %d updates failed", failed, len(plan))
	}
	if depsFailed > 0 || failed > 0 {
		return exitError
	}

	return exitOK
}

// resolveDeps plans the new dependencies of the mods in plan.
func (c *ctx) resolveDeps(plan []update.ModUpdate, provided map[string]bool, excluded []string) (update.DepPlan, error) {
	index, err := update.FetchPackageIndex(c.cdb, c.eng)
	if err != nil {
		return update.DepPlan{}, fmt.Errorf("fetching package list: %w", err)
	}

	roots := make([]string, len(plan))
	for i, u := range plan {
		roots[i] = u.Mod.Author + "/" + u.Mod.Name
	}

	skip := make(map[string]bool, len(excluded))
	for _, n := range excluded {
		skip[n] = true
	}

	return update.ResolveDeps(c.cdb, roots, provided, index, skip)
}

// unusedExcludes returns the names in depExcludes that no dependency in deps was excluded by.
func unusedExcludes(depExcludes []string, deps update.DepPlan) []string {
	used := make(map[string]bool)
	for _, m := range deps.NotFound {
		if !m.Excluded {
			continue
		}
		used[m.Dep] = true
		if _, name, ok := strings.Cut(m.Package, "/"); ok {
			used[name] = true
		}
	}

	var unused []string
	for _, n := range depExcludes {
		if !used[n] {
			unused = append(unused, n)
		}
	}
	return unused
}

var packageNameRe = regexp.MustCompile(`^[a-z0-9_]+$`)

func (c *ctx) installDep(d update.DepInstall) error {
	// the name comes from the network and becomes a folder name
	if !packageNameRe.MatchString(d.Package.Name) {
		return fmt.Errorf("invalid package name %q", d.Package.Name)
	}

	dest := filepath.Join(c.eng.ModsDir, d.Package.Name)

	// InstallMod replaces its target, which here would be a folder luma didn't put there
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("folder %s already exists", dest)
	}

	return update.InstallMod(c.cdb, d.Package.Author, d.Package.Name, d.Package.Release, dest, contentdb.ReasonDependency)
}

// scanGameMods scans the mods of the installed game id, reporting any error itself. ok is false on failure.
func (c *ctx) scanGameMods(id string) (mods []content.Mod, ok bool) {
	games, err := content.ScanGames(c.eng.GamesDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		c.err("scanning games: %v", err)
		return nil, false
	}

	for _, g := range games {
		if g.ID != id {
			continue
		}

		mods, err := content.ScanMods(filepath.Join(g.Path, "mods"))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			c.err("scanning mods of game %s: %v", id, err)
			return nil, false
		}
		return mods, true
	}

	c.err("game not installed: %s", id)
	return nil, false
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

// confirmDeps shows the plan on stderr and asks whether to go ahead, defaulting to no.
func (c *ctx) confirmDeps(plan []update.ModUpdate, installs []update.DepInstall) bool {
	fmt.Fprintln(c.stderr, "Will update:")
	printUpdates(c.stderr, plan)
	fmt.Fprintln(c.stderr, "\nWill also install as dependencies:")
	printDepInstalls(c.stderr, installs)
	fmt.Fprint(c.stderr, "\nContinue? [y/N] ")

	answer, _ := bufio.NewReader(c.stdin).ReadString('\n')
	answer = strings.ToLower(strings.TrimSpace(answer))

	return answer == "y" || answer == "yes"
}

func depNames(installs []update.DepInstall) string {
	names := make([]string, len(installs))
	for i, d := range installs {
		names[i] = d.Package.Author + "/" + d.Package.Name
	}
	return strings.Join(names, ", ")
}

func printDepInstalls(w io.Writer, installs []update.DepInstall) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tRELEASE\tFOR\tNEEDED BY")

	for _, d := range installs {
		fmt.Fprintf(tw, "%s/%s\t%d\t%s\t%s\n", d.Package.Author, d.Package.Name, d.Package.Release, d.Dep, d.RequiredBy)
	}

	tw.Flush()
}

func printUpdates(w io.Writer, updates []update.ModUpdate) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
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
