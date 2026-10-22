package update

import (
	"fmt"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/luanti-org/luma/internal/cache"
	"github.com/luanti-org/luma/internal/content"
	"github.com/luanti-org/luma/internal/contentdb"
	"github.com/luanti-org/luma/internal/engine"
)

// DepInstall is a package to install because RequiredBy needs the mod Dep.
type DepInstall struct {
	Package    contentdb.Package
	Dep        string
	RequiredBy string // "author/name"
}

// MissingDep is a hard dependency that will not be installed.
type MissingDep struct {
	Dep        string
	RequiredBy string // "author/name"
	Excluded   bool   // skipped on request rather than not found
	Ambiguous  bool   // several packages are named Dep or provide it, and none was picked
	GameOnly   bool   // not found, but a game provides it
	Package    string // "author/name" that was excluded, empty if excluded by Dep
}

// DepPlan is the outcome of ResolveDeps.
type DepPlan struct {
	Install  []DepInstall
	NotFound []MissingDep
}

// ModDep is a dependency of RequiredBy and the package that would provide it.
type ModDep struct {
	Dep        string
	RequiredBy string // "author/name", or the mod name from ListLocalDeps
	Optional   bool
	Installed  bool
	Available  bool // Package is set
	GameOnly   bool // no Package, but a game provides it
	Ambiguous  bool // no Package, as several packages provide it and none was picked
	Package    contentdb.Package
}

// PackageListMaxAge is how old a cached package list may be.
const PackageListMaxAge = 10 * time.Minute

// FetchPackages fetches every package compatible with engineInfo, in ContentDB's order, most popular first.
// A cached copy younger than maxAge is returned instead, a fresh fetch replaces it.
func FetchPackages(client *contentdb.Client, engineInfo engine.Info, maxAge time.Duration) ([]contentdb.Package, error) {
	// bump the version when the cached shape or contents change
	key := fmt.Sprintf("packages/v1 %s protocol=%d engine=%s", client.BaseURL, engineInfo.Protocol, engineInfo.Version)
	var packages []contentdb.Package
	if cache.GetJSON(key, maxAge, &packages) {
		return packages, nil
	}

	packages, err := client.Search(contentdb.SearchOptions{
		ProtocolVersion: engineInfo.Protocol,
		EngineVersion:   engineInfo.Version,
	})
	if err != nil {
		return nil, err
	}

	cache.SaveJSON(key, packages)
	return packages, nil
}

// FetchPackageIndex fetches the packages from FetchPackages, keyed for ResolveDeps.
func FetchPackageIndex(client *contentdb.Client, engineInfo engine.Info) (map[string]contentdb.Package, error) {
	packages, err := FetchPackages(client, engineInfo, PackageListMaxAge)
	if err != nil {
		return nil, err
	}

	index := make(map[string]contentdb.Package, len(packages))
	for _, p := range packages {
		index[packageKey(p.Author+"/"+p.Name)] = p
	}

	return index, nil
}

// ResolveDeps plans the packages needed to meet the hard dependencies of roots, recursively.
// roots are "author/name" ids, installed holds mod names on disk, excluded holds mod or package names to skip.
func ResolveDeps(client *contentdb.Client, roots []string, installed map[string]bool,
	packages map[string]contentdb.Package, excluded map[string]bool) (DepPlan, error) {
	r := newDepResolver(client, installed, packages, excluded)
	return r.resolveRoots(roots)
}

// ResolveInstallDeps plans the hard dependencies of the ContentDB roots and the local mods, see ResolveDeps.
// A local mod's dependencies come from its own conf files. ContentDB only picks their packages,
// from the package its conf's author and name point to if any, else the one package named like the dependency.
func ResolveInstallDeps(client *contentdb.Client, roots []string, locals []LocalPackage,
	installed map[string]bool, packages map[string]contentdb.Package) (DepPlan, error) {
	all := maps.Clone(installed)
	for _, p := range locals {
		if p.Type == "mod" {
			maps.Copy(all, content.ProvidedModNames([]content.Mod{p.Mod}))
		}
	}

	localDeps := make([][]contentdb.Dependency, len(locals))
	var unresolved []*contentdb.Dependency
	for i, p := range locals {
		if p.Type != "mod" {
			continue
		}

		deps, err := localDependencies(client, p, packages)
		if err != nil {
			return DepPlan{}, err
		}
		localDeps[i] = deps

		for j := range deps {
			if deps[j].Packages == nil && !deps[j].IsOptional && !all[deps[j].Name] {
				unresolved = append(unresolved, &deps[j])
			}
		}
	}

	ambiguous, err := addProviders(client, unresolved, packages)
	if err != nil {
		return DepPlan{}, err
	}

	r := newDepResolver(client, all, packages, nil)
	var localRoots []string

	for i, p := range locals {
		if p.Type != "mod" {
			continue
		}

		// a local root has no "/", so its key can't match a ContentDB package
		r.raw[packageKey(p.Name)] = localDeps[i]
		localRoots = append(localRoots, p.Name)

		for _, d := range localDeps[i] {
			if d.Packages == nil && ambiguous[d.Name] && !r.seen[d.Name] {
				r.miss(MissingDep{Dep: d.Name, RequiredBy: p.Name, Ambiguous: true})
			}
		}
	}

	return r.resolveRoots(slices.Concat(roots, localRoots))
}

// providerMaxAges are how old the cached mod providers may be, the second is tried once if a hard dep is still missing.
var providerMaxAges = []time.Duration{24 * time.Hour, 10 * time.Minute}

// addProviders fills in the packages of deps from the mod names every ContentDB mod provides,
// and returns the names that several packages provide with none picked.
func addProviders(client *contentdb.Client, deps []*contentdb.Dependency,
	packages map[string]contentdb.Package) (map[string]bool, error) {
	ambiguous := make(map[string]bool)
	for i, maxAge := range providerMaxAges {
		missing := slices.DeleteFunc(slices.Clone(deps), func(d *contentdb.Dependency) bool {
			return d.Packages != nil || ambiguous[d.Name] || (i > 0 && d.IsOptional)
		})
		if len(missing) == 0 {
			break
		}

		providers, err := client.ModProviders(maxAge)
		if err != nil {
			return nil, fmt.Errorf("fetching mod providers: %w", err)
		}

		for _, d := range missing {
			id, several, games := pickProvider(d.Name, providers[d.Name], packages)
			switch {
			case id != "":
				d.Packages = []string{id}
			case several:
				ambiguous[d.Name] = true
			case len(games) > 0:
				// never picked, but marks the dep as game-only rather than not found
				d.Packages = games
			}
		}
	}

	for _, d := range deps {
		if d.Packages == nil && len(modsNamed(packages, d.Name)) > 1 {
			ambiguous[d.Name] = true
		}
	}

	return ambiguous, nil
}

// pickProvider returns the one non-game provider of dep in the index, or the one named like dep among several.
// several is true when more than one is left to choose from, games are the game providers in the index.
func pickProvider(dep string, ids []string, packages map[string]contentdb.Package) (id string, several bool, games []string) {
	var usable, named []string
	for _, id := range ids {
		// a stale provider may have gone, or have no compatible release
		pkg, ok := packages[packageKey(id)]
		if !ok {
			continue
		}
		if pkg.Type == "game" {
			games = append(games, id)
			continue
		}
		usable = append(usable, id)
		if pkg.Name == dep {
			named = append(named, id)
		}
	}

	switch {
	case len(usable) == 1:
		return usable[0], false, games
	case len(named) == 1:
		return named[0], false, games
	}
	return "", len(usable) > 1, games
}

// modsNamed returns the mod packages named name, which a dependency of that name likely means.
func modsNamed(packages map[string]contentdb.Package, name string) []contentdb.Package {
	var out []contentdb.Package
	for _, pkg := range FindPackages(packages, name) {
		if pkg.Type == "mod" {
			out = append(out, pkg)
		}
	}
	return out
}

// localDependencies returns the dependencies in p's conf files with the packages that provide them.
func localDependencies(client *contentdb.Client, p LocalPackage, packages map[string]contentdb.Package) ([]contentdb.Dependency, error) {
	providers := make(map[string][]string)

	id := p.Author + "/" + p.Name
	if _, ok := packages[packageKey(id)]; ok && p.Author != "" {
		resp, err := client.Dependencies(p.Author, p.Name, false)
		if err != nil {
			return nil, fmt.Errorf("fetching dependencies of %s: %w", id, err)
		}
		for respID, deps := range resp {
			if packageKey(respID) != packageKey(id) {
				continue
			}
			for _, d := range deps {
				providers[d.Name] = d.Packages
			}
		}
	}

	hard, optional := p.Mod.Deps()
	out := make([]contentdb.Dependency, len(hard)+len(optional))

	for i, name := range slices.Concat(hard, optional) {
		out[i] = contentdb.Dependency{Name: name, IsOptional: i >= len(hard), Packages: providers[name]}
		if out[i].Packages != nil {
			continue
		}
		if found := modsNamed(packages, name); len(found) == 1 {
			out[i].Packages = []string{found[0].Author + "/" + found[0].Name}
		}
	}

	return out, nil
}

func newDepResolver(client *contentdb.Client, installed map[string]bool,
	packages map[string]contentdb.Package, excluded map[string]bool) *depResolver {
	return &depResolver{
		client:    client,
		installed: installed,
		packages:  packages,
		excluded:  excluded,
		planned:   make(map[string]bool),
		seen:      make(map[string]bool),
		raw:       make(map[string][]contentdb.Dependency),
	}
}

func (r *depResolver) resolveRoots(roots []string) (DepPlan, error) {
	for _, root := range roots {
		r.planned[packageKey(root)] = true
	}

	for _, root := range roots {
		if err := r.resolve(root); err != nil {
			return DepPlan{}, err
		}
	}

	return r.plan, nil
}

// ResolveModDeps plans the new dependencies of the mods in updates, see ResolveDeps.
func ResolveModDeps(client *contentdb.Client, updates []ModUpdate, installed map[string]bool,
	packages map[string]contentdb.Package, excluded map[string]bool) (DepPlan, error) {
	roots := make([]string, len(updates))
	for i, u := range updates {
		roots[i] = u.Mod.Author + "/" + u.Mod.Name
	}

	return ResolveDeps(client, roots, installed, packages, excluded)
}

// ListDeps lists the hard and optional dependencies of the "author/name" package id, in ContentDB order.
// installed holds mod names on disk, packages is the index from FetchPackageIndex.
func ListDeps(client *contentdb.Client, id string, installed map[string]bool,
	packages map[string]contentdb.Package) ([]ModDep, error) {
	author, name, ok := strings.Cut(id, "/")
	if !ok {
		return nil, fmt.Errorf("invalid package id %q", id)
	}

	resp, err := client.Dependencies(author, name, false)
	if err != nil {
		return nil, fmt.Errorf("fetching dependencies of %s: %w", id, err)
	}

	r := &depResolver{packages: packages}
	var out []ModDep

	for respID, deps := range resp {
		if packageKey(respID) != packageKey(id) {
			continue
		}

		for _, dep := range deps {
			o := ModDep{Dep: dep.Name, RequiredBy: id, Optional: dep.IsOptional, Installed: installed[dep.Name]}
			o.Package, o.Available = r.candidate(dep, true)
			o.GameOnly = !o.Available && providedByGame(dep, packages)
			out = append(out, o)
		}
	}

	return out, nil
}

// ListLocalDeps lists the hard and optional dependencies in m's conf files, or its members' for a modpack,
// for mods ContentDB can't look up. Their packages are found as for a local mod in ResolveInstallDeps.
func ListLocalDeps(client *contentdb.Client, m content.Mod, installed map[string]bool,
	packages map[string]contentdb.Package) ([]ModDep, error) {
	deps, err := localDependencies(client, LocalPackage{Type: "mod", Name: m.Name, Author: m.Author, Mod: m}, packages)
	if err != nil {
		return nil, err
	}

	var unresolved []*contentdb.Dependency
	for i := range deps {
		if deps[i].Packages == nil && !installed[deps[i].Name] {
			unresolved = append(unresolved, &deps[i])
		}
	}

	ambiguous, err := addProviders(client, unresolved, packages)
	if err != nil {
		return nil, err
	}

	r := &depResolver{packages: packages}
	out := make([]ModDep, len(deps))

	for i, dep := range deps {
		o := ModDep{Dep: dep.Name, RequiredBy: m.Name, Optional: dep.IsOptional, Installed: installed[dep.Name]}
		o.Package, o.Available = r.candidate(dep, true)
		o.GameOnly = !o.Available && providedByGame(dep, packages)
		o.Ambiguous = !o.Available && ambiguous[dep.Name]
		out[i] = o
	}

	return out, nil
}

func providedByGame(dep contentdb.Dependency, packages map[string]contentdb.Package) bool {
	for _, id := range dep.Packages {
		if packages[packageKey(id)].Type == "game" {
			return true
		}
	}
	return false
}

// PlanDeps plans installing picks along with their hard dependencies, see ResolveDeps.
// Picks that are installed or have no package are left out, excluded applies only to their dependencies.
func PlanDeps(client *contentdb.Client, picks []ModDep, installed map[string]bool,
	packages map[string]contentdb.Package, excluded map[string]bool) (DepPlan, error) {
	var installs []DepInstall
	var roots []string
	picked := make(map[string]bool)

	for _, o := range picks {
		id := o.Package.Author + "/" + o.Package.Name
		// one package can provide several of the picked names
		if o.Installed || !o.Available || picked[packageKey(id)] {
			continue
		}

		picked[packageKey(id)] = true
		roots = append(roots, id)
		installs = append(installs, DepInstall{Package: o.Package, Dep: o.Dep, RequiredBy: o.RequiredBy})
	}

	plan, err := ResolveDeps(client, roots, installed, packages, excluded)
	if err != nil {
		return DepPlan{}, err
	}

	plan.Install = append(installs, plan.Install...)
	return plan, nil
}

var packageNameRe = regexp.MustCompile(`^[a-z0-9_]+$`)

// InstallDep installs d into a new folder under modsDir, never replacing an existing one.
func InstallDep(client *contentdb.Client, modsDir string, d DepInstall) error {
	// the name comes from the network and becomes a folder name
	if !packageNameRe.MatchString(d.Package.Name) {
		return fmt.Errorf("invalid package name %q", d.Package.Name)
	}

	return installNew(client, d.Package, filepath.Join(modsDir, d.Package.Name), contentdb.ReasonDependency)
}

type depResolver struct {
	client    *contentdb.Client
	installed map[string]bool
	packages  map[string]contentdb.Package
	excluded  map[string]bool

	planned map[string]bool // package keys being installed or updated in this run
	seen    map[string]bool // dep names already decided
	raw     map[string][]contentdb.Dependency
	plan    DepPlan
}

func (r *depResolver) resolve(id string) error {
	deps, err := r.rawDeps(id)
	if err != nil {
		return err
	}

	var added []string

	// exact name matches are picked first so the result doesn't depend on dep order
	for _, fallback := range []bool{false, true} {
		for _, dep := range deps {
			if dep.IsOptional || r.seen[dep.Name] {
				continue
			}

			if r.installed[dep.Name] || r.plannedProvides(dep) {
				r.seen[dep.Name] = true
				continue
			}

			if r.excluded[dep.Name] {
				r.miss(MissingDep{Dep: dep.Name, RequiredBy: id, Excluded: true})
				continue
			}

			pkg, ok := r.candidate(dep, fallback)
			if !ok {
				if fallback {
					r.miss(MissingDep{Dep: dep.Name, RequiredBy: id, GameOnly: providedByGame(dep, r.packages)})
				}
				continue
			}

			depID := pkg.Author + "/" + pkg.Name
			if r.excluded[pkg.Name] {
				r.miss(MissingDep{Dep: dep.Name, RequiredBy: id, Excluded: true, Package: depID})
				continue
			}

			r.seen[dep.Name] = true
			r.planned[packageKey(depID)] = true
			r.plan.Install = append(r.plan.Install, DepInstall{Package: pkg, Dep: dep.Name, RequiredBy: id})
			added = append(added, depID)
		}
	}

	for _, depID := range added {
		if err := r.resolve(depID); err != nil {
			return err
		}
	}

	return nil
}

func (r *depResolver) miss(m MissingDep) {
	r.seen[m.Dep] = true
	r.plan.NotFound = append(r.plan.NotFound, m)
}

// plannedProvides reports whether a package already in this run provides dep.
func (r *depResolver) plannedProvides(dep contentdb.Dependency) bool {
	for _, id := range dep.Packages {
		key := packageKey(id)
		if r.planned[key] && r.packages[key].Type != "game" {
			return true
		}
	}
	return false
}

// candidate picks the non-game package named like dep, or with fallback the first non-game one.
func (r *depResolver) candidate(dep contentdb.Dependency, fallback bool) (contentdb.Package, bool) {
	var first *contentdb.Package

	for _, id := range dep.Packages {
		// packages lacking a compatible release are not in the index
		pkg, ok := r.packages[packageKey(id)]
		if !ok || pkg.Type == "game" {
			continue
		}

		if pkg.Name == dep.Name {
			return pkg, true
		}
		if first == nil {
			first = &pkg
		}
	}

	if fallback && first != nil {
		return *first, true
	}

	return contentdb.Package{}, false
}

// rawDeps returns the dependencies of id, reusing any earlier response that covered it.
func (r *depResolver) rawDeps(id string) ([]contentdb.Dependency, error) {
	key := packageKey(id)
	if deps, ok := r.raw[key]; ok {
		return deps, nil
	}

	author, name, ok := strings.Cut(id, "/")
	if !ok {
		return nil, fmt.Errorf("invalid package id %q", id)
	}

	resp, err := r.client.Dependencies(author, name, true)
	if err != nil {
		return nil, fmt.Errorf("fetching dependencies of %s: %w", id, err)
	}

	for respID, deps := range resp {
		if _, ok := r.raw[packageKey(respID)]; !ok {
			r.raw[packageKey(respID)] = deps
		}
	}

	// keeps a package absent from its own response from being fetched again
	if _, ok := r.raw[key]; !ok {
		r.raw[key] = nil
	}

	return r.raw[key], nil
}

// packageKey normalises an "author/name" id, which ContentDB treats case-insensitively.
func packageKey(id string) string {
	return strings.ToLower(id)
}
