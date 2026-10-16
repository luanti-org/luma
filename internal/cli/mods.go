package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
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

	provided, ok := c.providedModNames(mods, *gameID)
	if !ok {
		return exitError
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

	c.printNotFound(deps.NotFound)

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
	depsFailed := c.installDeps(deps.Install)

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

func modsListOptional(c *ctx, args []string) int {
	fs := c.newFlagSet()
	namesOnly := fs.Bool("names", false, "print only the optional dependencies that can be installed, one per line")
	gameID := fs.String("game", "", "count this game's mods as installed")
	if code, done := c.parseFlagsWithArgs(fs, args); done {
		return code
	}
	if fs.NArg() != 1 {
		return c.usageErr("%s: expected one mod name", fs.Name())
	}

	name := fs.Arg(0)
	opts, _, _, ok := c.optionalDeps(name, *gameID)
	if !ok {
		return exitError
	}

	if *namesOnly {
		for _, o := range opts {
			if o.Available && !o.Installed {
				fmt.Fprintln(c.stdout, o.Dep)
			}
		}
		return exitOK
	}

	if len(opts) == 0 {
		fmt.Fprintf(c.stdout, "%s has no optional dependencies.\n", name)
		return exitOK
	}

	tw := tabwriter.NewWriter(c.stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "DEP\tSTATUS\tPACKAGE\tRELEASE")

	for _, o := range opts {
		switch {
		case o.Installed:
			fmt.Fprintf(tw, "%s\tinstalled\t-\t-\n", o.Dep)
		case o.Available:
			fmt.Fprintf(tw, "%s\tavailable\t%s/%s\t%d\n", o.Dep, o.Package.Author, o.Package.Name, o.Package.Release)
		case o.GameOnly:
			fmt.Fprintf(tw, "%s\tfrom game\t-\t-\n", o.Dep)
		default:
			fmt.Fprintf(tw, "%s\tnot found\t-\t-\n", o.Dep)
		}
	}

	tw.Flush()

	return exitOK
}

func modsInstallOptional(c *ctx, args []string) int {
	fs := c.newFlagSet()
	dryRun := fs.BoolP("dry-run", "n", false, "show what would be installed without changing anything")
	yes := fs.BoolP("yes", "y", false, "install without asking")
	gameID := fs.String("game", "", "count this game's mods as installed")
	if code, done := c.parseFlagsWithArgs(fs, args); done {
		return code
	}
	if fs.NArg() == 0 {
		return c.usageErr("%s: expected a mod name", fs.Name())
	}

	name, deps := fs.Arg(0), dedupe(fs.Args()[1:])
	opts, provided, index, ok := c.optionalDeps(name, *gameID)
	if !ok {
		return exitError
	}

	picks, ok := pickOptional(c, name, opts, deps)
	if !ok {
		return exitError
	}
	if len(picks) == 0 {
		fmt.Fprintln(c.stdout, "Nothing to install.")
		return exitOK
	}

	plan, err := update.PlanOptionalDeps(c.cdb, picks, provided, index, nil)
	if err != nil {
		return c.err("%v", err)
	}

	c.printNotFound(plan.NotFound)

	if *dryRun {
		fmt.Fprintln(c.stdout, "Would install:")
		printDepInstalls(c.stdout, plan.Install)
		return exitOK
	}

	if !*yes {
		if !c.interactive {
			return c.err("would install %s, rerun with -y", depNames(plan.Install))
		}
		fmt.Fprintln(c.stderr, "Will install:")
		printDepInstalls(c.stderr, plan.Install)
		if !c.askContinue() {
			return c.err("aborted, nothing was changed")
		}
	}

	if failed := c.installDeps(plan.Install); failed > 0 {
		return c.err("%d of %d installs failed", failed, len(plan.Install))
	}

	return exitOK
}

// pickOptional picks the opts named in deps, or all installable ones when deps is empty.
// Names that aren't optional dependencies of mod are an error, ok is false then.
func pickOptional(c *ctx, mod string, opts []update.OptionalDep, deps []string) (picks []update.OptionalDep, ok bool) {
	if len(deps) == 0 {
		for _, o := range opts {
			if o.Available && !o.Installed {
				picks = append(picks, o)
			}
		}
		return picks, true
	}

	byDep := make(map[string]update.OptionalDep, len(opts))
	for _, o := range opts {
		byDep[o.Dep] = o
	}

	var unknown []string
	for _, d := range deps {
		if _, ok := byDep[d]; !ok {
			unknown = append(unknown, d)
		}
	}
	if len(unknown) > 0 {
		c.err("not optional dependencies of %s: %s", mod, strings.Join(unknown, ", "))
		return nil, false
	}

	for _, d := range deps {
		switch o := byDep[d]; {
		case o.Installed:
			fmt.Fprintf(c.stdout, "%s: already installed\n", d)
		case o.GameOnly:
			fmt.Fprintf(c.stdout, "%s: only provided by games, see --game\n", d)
		case !o.Available:
			fmt.Fprintf(c.stdout, "%s: no compatible package on ContentDB\n", d)
		default:
			picks = append(picks, o)
		}
	}

	return picks, true
}

// optionalDeps lists the optional dependencies of the installed mod name, with the provided names and package index
// used to find them. It reports any error itself, ok is false on failure.
func (c *ctx) optionalDeps(name, gameID string) (opts []update.OptionalDep, provided map[string]bool,
	index map[string]contentdb.Package, ok bool) {
	mods, ok := c.scanMods()
	if !ok {
		return nil, nil, nil, false
	}

	i := slices.IndexFunc(mods, func(m content.Mod) bool { return m.Name == name })
	if i < 0 {
		c.err("not installed: %s", name)
		return nil, nil, nil, false
	}
	m := mods[i]
	if m.Author == "" {
		c.err("%s: can't look up dependencies, no author in %s", name, confName(m))
		return nil, nil, nil, false
	}

	provided, ok = c.providedModNames(mods, gameID)
	if !ok {
		return nil, nil, nil, false
	}

	index, err := c.packageIndex()
	if err != nil {
		c.err("%v", err)
		return nil, nil, nil, false
	}

	opts, err = update.ListOptionalDeps(c.cdb, m.Author+"/"+m.Name, provided, index)
	if err != nil {
		c.err("%v", err)
		return nil, nil, nil, false
	}

	return opts, provided, index, true
}

// installDeps installs each of installs, reporting the outcome of each, and returns how many failed.
func (c *ctx) installDeps(installs []update.DepInstall) (failed int) {
	for _, d := range installs {
		id := d.Package.Author + "/" + d.Package.Name
		if err := update.InstallDep(c.cdb, c.eng.ModsDir, d); err != nil {
			failed++
			fmt.Fprintf(c.stderr, "luma: %s: install failed: %v\n", id, err)
			continue
		}
		fmt.Fprintf(c.stdout, "%s: installed %d (dependency of %s)\n", id, d.Package.Release, d.RequiredBy)
	}
	return failed
}

func (c *ctx) printNotFound(missing []update.MissingDep) {
	for _, m := range missing {
		switch {
		case !m.Excluded:
			fmt.Fprintf(c.stderr, "luma: could not find dependency %s for %s\n", m.Dep, m.RequiredBy)
		case m.Package != "":
			fmt.Fprintf(c.stderr, "luma: skipping excluded dependency %s (%s) for %s\n", m.Dep, m.Package, m.RequiredBy)
		default:
			fmt.Fprintf(c.stderr, "luma: skipping excluded dependency %s for %s\n", m.Dep, m.RequiredBy)
		}
	}
}

// providedModNames returns the mod names mods provide, plus those of game gameID if set. ok is false on failure.
func (c *ctx) providedModNames(mods []content.Mod, gameID string) (provided map[string]bool, ok bool) {
	provided = content.ProvidedModNames(mods)
	if gameID == "" {
		return provided, true
	}

	gameMods, ok := c.scanGameMods(gameID)
	if !ok {
		return nil, false
	}
	maps.Copy(provided, content.ProvidedModNames(gameMods))

	return provided, true
}

func (c *ctx) packageIndex() (map[string]contentdb.Package, error) {
	index, err := update.FetchPackageIndex(c.cdb, c.eng)
	if err != nil {
		return nil, fmt.Errorf("fetching package list: %w", err)
	}
	return index, nil
}

// resolveDeps plans the new dependencies of the mods in plan.
func (c *ctx) resolveDeps(plan []update.ModUpdate, provided map[string]bool, excluded []string) (update.DepPlan, error) {
	index, err := c.packageIndex()
	if err != nil {
		return update.DepPlan{}, err
	}

	skip := make(map[string]bool, len(excluded))
	for _, n := range excluded {
		skip[n] = true
	}

	return update.ResolveModDeps(c.cdb, plan, provided, index, skip)
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
	if m.Release <= 0 && update.IsGitCheckout(m.Path) {
		return "git checkout with no release, skipped"
	}
	if m.Author != "" {
		return "no update available"
	}
	return "can't check for updates, no author in " + confName(m)
}

func confName(m content.Mod) string {
	if m.IsModpack {
		return "modpack.conf"
	}
	return "mod.conf"
}

// confirmDeps shows the plan on stderr and asks whether to go ahead, defaulting to no.
func (c *ctx) confirmDeps(plan []update.ModUpdate, installs []update.DepInstall) bool {
	fmt.Fprintln(c.stderr, "Will update:")
	printUpdates(c.stderr, plan)
	fmt.Fprintln(c.stderr, "\nWill also install as dependencies:")
	printDepInstalls(c.stderr, installs)

	return c.askContinue()
}

// askContinue asks on stderr whether to go ahead, defaulting to no.
func (c *ctx) askContinue() bool {
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
