package update

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

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
	Package    string // "author/name" that was excluded, empty if excluded by Dep
}

// DepPlan is the outcome of ResolveDeps.
type DepPlan struct {
	Install  []DepInstall
	NotFound []MissingDep
}

// OptionalDep is an optional dependency of RequiredBy and the package that would provide it.
type OptionalDep struct {
	Dep        string
	RequiredBy string // "author/name"
	Installed  bool
	Available  bool // Package is set
	GameOnly   bool // no Package, but a game provides it
	Package    contentdb.Package
}

// FetchPackageIndex fetches every package compatible with engineInfo, keyed for ResolveDeps.
func FetchPackageIndex(client *contentdb.Client, engineInfo engine.Info) (map[string]contentdb.Package, error) {
	packages, err := client.Search(contentdb.SearchOptions{
		ProtocolVersion: engineInfo.Protocol,
		EngineVersion:   engineInfo.Version,
	})
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
	r := &depResolver{
		client:    client,
		installed: installed,
		packages:  packages,
		excluded:  excluded,
		planned:   make(map[string]bool),
		seen:      make(map[string]bool),
		raw:       make(map[string][]contentdb.Dependency),
	}

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

// ListOptionalDeps lists the optional dependencies of the "author/name" package id, in ContentDB order.
// installed holds mod names on disk, packages is the index from FetchPackageIndex.
func ListOptionalDeps(client *contentdb.Client, id string, installed map[string]bool,
	packages map[string]contentdb.Package) ([]OptionalDep, error) {
	author, name, ok := strings.Cut(id, "/")
	if !ok {
		return nil, fmt.Errorf("invalid package id %q", id)
	}

	resp, err := client.Dependencies(author, name, false)
	if err != nil {
		return nil, fmt.Errorf("fetching dependencies of %s: %w", id, err)
	}

	r := &depResolver{packages: packages}
	var out []OptionalDep

	for respID, deps := range resp {
		if packageKey(respID) != packageKey(id) {
			continue
		}

		for _, dep := range deps {
			if !dep.IsOptional {
				continue
			}

			o := OptionalDep{Dep: dep.Name, RequiredBy: id, Installed: installed[dep.Name]}
			o.Package, o.Available = r.candidate(dep, true)
			o.GameOnly = !o.Available && providedByGame(dep, packages)
			out = append(out, o)
		}
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

// PlanOptionalDeps plans installing picks along with their hard dependencies, see ResolveDeps.
// Picks that are installed or have no package are left out, excluded applies only to their dependencies.
func PlanOptionalDeps(client *contentdb.Client, picks []OptionalDep, installed map[string]bool,
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

	dest := filepath.Join(modsDir, d.Package.Name)

	// InstallMod replaces its target, which here would be a folder luma didn't put there
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("folder %s already exists", dest)
	}

	return InstallMod(client, d.Package.Author, d.Package.Name, d.Package.Release, dest, contentdb.ReasonDependency)
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
					r.miss(MissingDep{Dep: dep.Name, RequiredBy: id})
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
