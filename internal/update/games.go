package update

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/luanti-org/luma/internal/content"
	"github.com/luanti-org/luma/internal/contentdb"
	"github.com/luanti-org/luma/internal/engine"
	"github.com/luanti-org/luma/internal/util"
)

// GameUpdate pairs a locally installed game with the latest release CDB knows about for it.
type GameUpdate struct {
	Game          content.Game
	Author        string // ContentDB author, differs from Game.Author only for an unversioned Minetest Game
	Package       string // ContentDB package name, the game id with or without "_game"
	LatestRelease int
}

// CheckAllGameUpdates compares games against ContentDB's /api/updates/ and
// returns the ones with a newer release available, following the Luanti client.
// engineInfo provides version for compatible releases.
// Games with no author or no release are skipped, except an unversioned Minetest Game
func CheckAllGameUpdates(games []content.Game, client *contentdb.Client, engineInfo engine.Info) ([]GameUpdate, error) {
	latest, err := client.Updates(contentdb.UpdatesOptions{
		Types:           []string{"game"},
		ProtocolVersion: engineInfo.Protocol,
		EngineVersion:   engineInfo.Version,
	})
	if err != nil {
		return nil, err
	}

	latest = lowerKeys(latest)

	var updates []GameUpdate
	for _, game := range games {
		if u, ok := matchGameUpdate(game, latest); ok {
			updates = append(updates, u)
		}
	}

	return updates, nil
}

// UpdateGame downloads and installs the latest release for u, replacing the game's existing directory in place.
func UpdateGame(client *contentdb.Client, u GameUpdate) error {
	url := client.ReleaseDownloadURL(u.Author, u.Package, u.LatestRelease, contentdb.ReasonUpdate)
	if err := DownloadPackage(client, url, u.Game.Path); err != nil {
		return err
	}

	// no `name` here, in game.conf it is a deprecated synonym for title
	return util.SetConfFields(filepath.Join(u.Game.Path, "game.conf"), map[string]string{
		"author":  u.Author,
		"release": strconv.Itoa(u.LatestRelease),
	})
}

// minetestGameAuthor owns Minetest Game on ContentDB.
const minetestGameAuthor = "Luanti"

// matchGameUpdate reports whether game has a newer release in latest, whose keys must be lowercase
func matchGameUpdate(game content.Game, latest map[string]int) (GameUpdate, bool) {
	id := normalizeGameID(game.ID)

	author := game.Author
	if author == "" || game.Release <= 0 {
		if !isUnversionedMinetestGame(game) {
			return GameUpdate{}, false
		}
		author = minetestGameAuthor
	}

	// the exact folder name goes first, a mod of the same author can be named like the short form
	names := []string{game.ID, id}
	if id == game.ID {
		names[1] = id + "_game"
	}

	for _, name := range names {
		release, ok := latest[strings.ToLower(author)+"/"+name]
		if !ok {
			continue
		}
		if release <= game.Release {
			return GameUpdate{}, false
		}
		return GameUpdate{Game: game, Author: author, Package: name, LatestRelease: release}, true
	}

	return GameUpdate{}, false
}

// isUnversionedMinetestGame reports whether game is the Minetest Game that engines up to 5.8 bundled without a release.
func isUnversionedMinetestGame(game content.Game) bool {
	if normalizeGameID(game.ID) != "minetest" || game.Release != 0 {
		return false
	}

	// a git checkout is the user's own, an update would wipe it
	_, err := os.Stat(filepath.Join(game.Path, ".git"))
	return os.IsNotExist(err)
}

// normalizeGameID strips the "_game" suffix the engine ignores in game folder names.
func normalizeGameID(id string) string {
	if short := strings.TrimSuffix(id, "_game"); short != "" {
		return short
	}
	return id
}
