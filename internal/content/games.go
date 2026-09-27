package content

import (
	"os"
	"path/filepath"

	"github.com/luanti-org/luma/internal/util"
)

// Game describes one game folder found under a games directory.
//
// Unlike a mod, a game's identifier is always its folder name
// (gameid) -- game.conf has no equivalent of mod.conf's `name`
// override. `title` is the required human-readable name (with a
// deprecated `name` synonym in older game.conf files).
type Game struct {
	ID          string // folder name, e.g. "minetest_game"
	Title       string
	Description string
	Author      string
	Release     int // from game.conf's `release`; 0 if absent

	Dir  string
	Path string

	// ConfOK is true only if game.conf exists and has a usable
	// title (via `title`, or the deprecated `name` synonym). If
	// neither is present, Title falls back to the folder name.
	ConfOK bool
}

// ScanGames scans the immediate subdirectories of dir for games.
func ScanGames(dir string) ([]Game, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var games []Game

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		gameDir := entry.Name()
		games = append(games, scanGameDir(gameDir, filepath.Join(dir, gameDir)))
	}

	return games, nil
}

func scanGameDir(dir, path string) Game {
	g := Game{
		ID:    dir,
		Title: dir, // fallback, overwritten below if game.conf has a title
		Dir:   dir,
		Path:  path,
	}

	data, err := os.ReadFile(filepath.Join(path, "game.conf"))
	if err != nil {
		return g
	}

	conf := util.ParseConfFile(data)

	title := conf["title"]
	if title == "" {
		title = conf["name"] // deprecated synonym for title
	}
	if title != "" {
		g.Title = title
		g.ConfOK = true
	}

	g.Description = conf["description"]
	g.Author = conf["author"]
	g.Release = util.ParseIntField(conf, "release")

	return g
}
