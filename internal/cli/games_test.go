package cli

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luanti-org/luma/internal/content"
	"github.com/luanti-org/luma/internal/contentdb"
	"github.com/luanti-org/luma/internal/engine"
)

// setupGames installs alpha_game and beta at release 1 and gamma at 5; the fake CDB has alpha_game 2, beta_game 3, gamma 5.
func setupGames(t *testing.T) (engine.Info, *fakeCDB, *contentdb.Client) {
	t.Helper()

	dir := t.TempDir()
	for id, release := range map[string]string{"alpha_game": "1", "beta": "1", "gamma": "5"} {
		writeFile(t, filepath.Join(dir, id, "game.conf"), "title = The "+id+"\nauthor = jane\nrelease = "+release+"\n")
	}

	f := &fakeCDB{updatesJSON: `{"jane/alpha_game": 2, "jane/beta_game": 3, "jane/gamma": 5}`}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)

	return engine.Info{GamesDir: dir}, f, contentdb.New(srv.URL)
}

func installedGameReleases(t *testing.T, dir string) map[string]int {
	t.Helper()

	games, err := content.ScanGames(dir)
	if err != nil {
		t.Fatal(err)
	}

	out := make(map[string]int, len(games))
	for _, g := range games {
		out[g.ID] = g.Release
	}
	return out
}

func TestGamesListNoInstall(t *testing.T) {
	code, _, stderr := run(t, engine.Info{}, "games", "list")
	if code != exitError || !strings.Contains(stderr, "pass --dir") {
		t.Errorf("code = %d, stderr = %q", code, stderr)
	}
}

func TestGamesListMissingDir(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "games")

	code, stdout, stderr := run(t, engine.Info{GamesDir: missing, ViaFlatpak: true}, "games", "list", "--names")
	if code != exitOK || stdout != "" {
		t.Errorf("flatpak: code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}

	code, _, stderr = run(t, engine.Info{GamesDir: missing}, "games", "list")
	if code != exitError || !strings.Contains(stderr, "scanning games") {
		t.Errorf("dir: code = %d, stderr = %q", code, stderr)
	}
}

func TestGamesList(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "alpha_game", "game.conf"), "title = Alpha Quest\nauthor = someone\nrelease = 42\n")
	writeFile(t, filepath.Join(dir, "beta", "init.lua"), "-- no conf")

	code, stdout, stderr := run(t, engine.Info{GamesDir: dir}, "games", "list")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	want := []string{"ID AUTHOR RELEASE TITLE", "alpha_game someone 42 Alpha Quest", "beta - - beta"}
	if len(lines) != len(want) {
		t.Fatalf("stdout =\n%s", stdout)
	}
	for i := range want {
		if got := strings.Join(strings.Fields(lines[i]), " "); got != want[i] {
			t.Errorf("line %d = %q, want %q", i, got, want[i])
		}
	}
}

func TestGamesListNames(t *testing.T) {
	eng, _, _ := setupGames(t)

	code, stdout, _ := run(t, eng, "games", "list", "--names")
	if code != exitOK || stdout != "alpha_game\nbeta\ngamma\n" {
		t.Errorf("code = %d, stdout = %q", code, stdout)
	}
}

func TestGamesOutdated(t *testing.T) {
	eng, _, cdb := setupGames(t)

	code, stdout, stderr := runWithCDB(t, eng, cdb, "games", "outdated")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	want := []string{"ID CURRENT LATEST", "alpha_game 1 2", "beta 1 3"}
	if len(lines) != len(want) {
		t.Fatalf("stdout =\n%s", stdout)
	}
	for i := range want {
		if got := strings.Join(strings.Fields(lines[i]), " "); got != want[i] {
			t.Errorf("line %d = %q, want %q", i, got, want[i])
		}
	}
}

func TestGamesOutdatedNames(t *testing.T) {
	eng, _, cdb := setupGames(t)

	code, stdout, _ := runWithCDB(t, eng, cdb, "games", "outdated", "--names")
	if code != exitOK || stdout != "alpha_game\nbeta\n" {
		t.Errorf("code = %d, stdout = %q", code, stdout)
	}
}

func TestGamesOutdatedAllUpToDate(t *testing.T) {
	eng, f, cdb := setupGames(t)
	f.updatesJSON = `{"jane/gamma": 5}`

	code, stdout, _ := runWithCDB(t, eng, cdb, "games", "outdated")
	if code != exitOK || !strings.Contains(stdout, "up to date") {
		t.Errorf("code = %d, stdout = %q", code, stdout)
	}
}

func TestGamesOutdatedFetchError(t *testing.T) {
	eng, f, cdb := setupGames(t)
	f.updatesFail = true

	code, _, stderr := runWithCDB(t, eng, cdb, "games", "outdated")
	if code != exitError || !strings.Contains(stderr, "checking for updates") {
		t.Errorf("code = %d, stderr = %q", code, stderr)
	}
}

func TestGamesUpdateAll(t *testing.T) {
	eng, f, cdb := setupGames(t)
	stale := filepath.Join(eng.GamesDir, "beta", "mods", "old", "init.lua")
	writeFile(t, stale, "-- old")

	code, stdout, stderr := runWithCDB(t, eng, cdb, "games", "update")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	// the download uses the ContentDB package name, not the folder name
	if got := f.downloaded(); got != "alpha_game,beta_game" {
		t.Errorf("downloaded = %q, want alpha_game,beta_game", got)
	}
	if !strings.Contains(stdout, "alpha_game: updated 1 -> 2") || !strings.Contains(stdout, "beta: updated 1 -> 3") {
		t.Errorf("stdout = %q", stdout)
	}

	rel := installedGameReleases(t, eng.GamesDir)
	if len(rel) != 3 || rel["alpha_game"] != 2 || rel["beta"] != 3 || rel["gamma"] != 5 {
		t.Errorf("releases after update = %v", rel)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale file from the old release still present, err = %v", err)
	}
}

func TestGamesUpdateNamed(t *testing.T) {
	eng, f, cdb := setupGames(t)

	code, stdout, stderr := runWithCDB(t, eng, cdb, "games", "update", "beta", "gamma")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if got := f.downloaded(); got != "beta_game" {
		t.Errorf("downloaded = %q, want beta_game", got)
	}
	if !strings.Contains(stdout, "gamma: no update available") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestGamesUpdateNamedUnversioned(t *testing.T) {
	eng, f, cdb := setupGames(t)
	writeFile(t, filepath.Join(eng.GamesDir, "alpha_game", "game.conf"), "title = Alpha\nauthor = jane\n")

	code, stdout, stderr := runWithCDB(t, eng, cdb, "games", "update", "alpha_game")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "alpha_game: can't check for updates, no author or release in game.conf") {
		t.Errorf("stdout = %q", stdout)
	}
	if got := f.downloaded(); got != "" {
		t.Errorf("downloaded = %q, want nothing", got)
	}
}

func TestGamesUpdateExclude(t *testing.T) {
	eng, f, cdb := setupGames(t)

	code, _, stderr := runWithCDB(t, eng, cdb, "games", "update", "--exclude", "alpha_game")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if got := f.downloaded(); got != "beta_game" {
		t.Errorf("downloaded = %q, want beta_game", got)
	}
}

func TestGamesUpdateUnknownNames(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"positional", []string{"beta", "nope"}},
		{"exclude", []string{"--exclude", "beta,nope"}},
		// ids are folder names, the engine's short form is not accepted
		{"short id", []string{"alpha"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng, f, cdb := setupGames(t)

			code, _, stderr := runWithCDB(t, eng, cdb, append([]string{"games", "update"}, tt.args...)...)
			if code != exitError || !strings.Contains(stderr, "not installed: ") || strings.Contains(stderr, "beta") {
				t.Errorf("code = %d, stderr = %q", code, stderr)
			}
			if got := f.downloaded(); got != "" {
				t.Errorf("downloaded = %q, want nothing", got)
			}
		})
	}
}

func TestGamesUpdateDryRun(t *testing.T) {
	eng, f, cdb := setupGames(t)

	code, stdout, _ := runWithCDB(t, eng, cdb, "games", "update", "-n")
	if code != exitOK || !strings.Contains(stdout, "Would update:") || !strings.Contains(stdout, "alpha_game") {
		t.Errorf("code = %d, stdout = %q", code, stdout)
	}
	if got := f.downloaded(); got != "" {
		t.Errorf("downloaded = %q, want nothing", got)
	}
}

func TestGamesUpdateNothingToDo(t *testing.T) {
	eng, f, cdb := setupGames(t)
	f.updatesJSON = `{"jane/gamma": 5}`

	code, stdout, _ := runWithCDB(t, eng, cdb, "games", "update")
	if code != exitOK || !strings.Contains(stdout, "Nothing to update.") {
		t.Errorf("code = %d, stdout = %q", code, stdout)
	}
}

func TestGamesUpdateContinuesAfterFailure(t *testing.T) {
	eng, f, cdb := setupGames(t)
	f.failFor = map[string]bool{"alpha_game": true}

	code, _, stderr := runWithCDB(t, eng, cdb, "games", "update")
	if code != exitError {
		t.Errorf("code = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "alpha_game: update failed") || !strings.Contains(stderr, "1 of 2 updates failed") {
		t.Errorf("stderr = %q", stderr)
	}

	rel := installedGameReleases(t, eng.GamesDir)
	if rel["alpha_game"] != 1 || rel["beta"] != 3 {
		t.Errorf("releases after update = %v, want alpha_game untouched at 1 and beta at 3", rel)
	}
}
