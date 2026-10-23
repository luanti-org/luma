package update

import (
	"strings"

	"github.com/luanti-org/luma/internal/contentdb"
)

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
