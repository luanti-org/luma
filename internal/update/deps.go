package update

import (
	"fmt"
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
