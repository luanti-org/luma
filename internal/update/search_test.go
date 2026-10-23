package update

import (
	"slices"
	"testing"

	"github.com/luanti-org/luma/internal/contentdb"
)

func TestMatchPackages(t *testing.T) {
	packages := []contentdb.Package{
		{Author: "jane", Name: "mobs", Title: "Mobs Redo", ShortDescription: "Adds a mob API", Type: "mod"},
		{Author: "bob", Name: "farming", Title: "Farming", ShortDescription: "Crops and food", Type: "mod"},
		{Author: "jane", Name: "voxelgarden", Title: "Voxelgarden", ShortDescription: "A classic game with mobs", Type: "game"},
		{Author: "ann", Name: "soothing", Title: "Soothing 32", ShortDescription: "Calm textures", Type: "txp"},
	}

	tests := []struct {
		name    string
		query   string
		pkgType string
		want    []string
	}{
		{"empty query keeps all in order", "", "", []string{"mobs", "farming", "voxelgarden", "soothing"}},
		{"empty query filters type", "  ", "mod", []string{"mobs", "farming"}},
		{"title case-insensitive", "FARM", "", []string{"farming"}},
		{"author/name", "jane/voxel", "", []string{"voxelgarden"}},
		{"short description", "calm", "", []string{"soothing"}},
		{"word in several fields keeps order", "mob", "", []string{"mobs", "voxelgarden"}},
		{"every word must match", "mob classic", "", []string{"voxelgarden"}},
		{"type filter", "mob", "game", []string{"voxelgarden"}},
		{"no match", "nothing", "", nil},
		{"words match different fields", "farming crops", "", []string{"farming"}},
		{"word does not span fields", "redojane", "", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, p := range MatchPackages(packages, tt.query, tt.pkgType) {
				got = append(got, p.Name)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("MatchPackages(%q, %q) = %v, want %v", tt.query, tt.pkgType, got, tt.want)
			}
		})
	}
}

func TestInstalledReleasesTagAndOrder(t *testing.T) {
	installed := InstalledReleases{
		installedKey("mod", "jane", "zeta"):   3,
		installedKey("mod", "jane", "alpha"):  5,
		installedKey("mod", "Bob", "beta"):    1,
		installedKey("game", "jane", "gamma"): 2,
	}
	packages := []contentdb.Package{
		{Author: "amy", Name: "popular", Title: "Popular", Type: "mod", Release: 1},
		{Author: "jane", Name: "zeta", Title: "Zeta", Type: "mod", Release: 3},
		{Author: "jane", Name: "gamma", Title: "Gamma", Type: "mod", Release: 9},
		{Author: "jane", Name: "alpha", Title: "Alpha", Type: "mod", Release: 5},
		{Author: "bob", Name: "beta", Title: "Beta", Type: "mod", Release: 4},
		{Author: "amy", Name: "less", Title: "Less", Type: "mod", Release: 1},
	}

	var got []string
	for _, p := range installed.InstalledFirst(packages) {
		got = append(got, installed.Tag(p)+p.Name)
	}
	// gamma is only installed as a game, author is case-insensitive
	want := []string{"[U]beta", "[I]alpha", "[I]zeta", "popular", "gamma", "less"}
	if !slices.Equal(got, want) {
		t.Errorf("InstalledFirst = %v, want %v", got, want)
	}
}
