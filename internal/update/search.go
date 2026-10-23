package update

import (
	"slices"
	"strings"

	"github.com/luanti-org/luma/internal/content"
	"github.com/luanti-org/luma/internal/contentdb"
	"github.com/luanti-org/luma/internal/engine"
)

// InstalledReleases holds the release of each installed package that names its author.
type InstalledReleases map[string]int

func installedKey(pkgType, author, name string) string {
	return pkgType + " " + strings.ToLower(author+"/"+name)
}

// ScanInstalledReleases scans the installed mods, games and texture packs.
// Scan errors only cost the installed markers, so they are ignored.
func ScanInstalledReleases(eng engine.Info) InstalledReleases {
	installed := make(InstalledReleases)

	mods, _ := content.ScanMods(eng.ModsDir)
	for _, mod := range mods {
		if mod.Author != "" {
			installed[installedKey("mod", mod.Author, mod.Name)] = mod.Release
		}
	}

	games, _ := content.ScanGames(eng.GamesDir)
	for _, g := range games {
		if g.Author != "" {
			installed[installedKey("game", g.Author, g.ID)] = g.Release
		}
	}

	texturepacks, _ := content.ScanTextures(eng.TexturesDir)
	for _, tp := range texturepacks {
		if tp.Author != "" {
			installed[installedKey("txp", tp.Author, tp.Name)] = tp.Release
		}
	}

	return installed
}

// Release returns the installed release of p, ok is false if p is not installed.
func (r InstalledReleases) Release(p contentdb.Package) (release int, ok bool) {
	release, ok = r[installedKey(p.Type, p.Author, p.Name)]
	return release, ok
}

// Tag returns "[U]" for an installed package with a newer release, "[I]" for one without, else "".
func (r InstalledReleases) Tag(p contentdb.Package) string {
	release, installed := r.Release(p)
	switch {
	case installed && release < p.Release:
		return "[U]"
	case installed:
		return "[I]"
	}
	return ""
}

// InstalledFirst orders packages like the client: installed first, those with an update before the rest, by title,
// then the others in their given order.
func (r InstalledReleases) InstalledFirst(packages []contentdb.Package) []contentdb.Package {
	var installed, rest []contentdb.Package
	for _, p := range packages {
		if r.Tag(p) != "" {
			installed = append(installed, p)
		} else {
			rest = append(rest, p)
		}
	}

	slices.SortStableFunc(installed, func(a, b contentdb.Package) int {
		if au, bu := r.Tag(a) == "[U]", r.Tag(b) == "[U]"; au != bu {
			if au {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Title, b.Title)
	})

	return append(installed, rest...)
}

// MatchPackages returns the packages of pkgType ("" for any) matching every word of query, in input order.
// A word matches a case-insensitive substring of the title, author/name or short description.
func MatchPackages(packages []contentdb.Package, query, pkgType string) []contentdb.Package {
	words := strings.Fields(strings.ToLower(query))

	var matches []contentdb.Package
	for _, p := range packages {
		if pkgType != "" && p.Type != pkgType {
			continue
		}
		text := strings.ToLower(p.Title + "\n" + p.Author + "/" + p.Name + "\n" + p.ShortDescription)
		if matchesAll(text, words) {
			matches = append(matches, p)
		}
	}
	return matches
}

func matchesAll(text string, words []string) bool {
	for _, w := range words {
		if !strings.Contains(text, w) {
			return false
		}
	}
	return true
}
