package update

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/luanti-org/luma/internal/content"
	"github.com/luanti-org/luma/internal/contentdb"
	"github.com/luanti-org/luma/internal/engine"
)

func checkGames(t *testing.T, body string, games ...content.Game) []GameUpdate {
	t.Helper()

	updates, err := CheckAllGameUpdates(games, newTestServer(t, body), engine.Info{})
	if err != nil {
		t.Fatalf("CheckAllGameUpdates: %v", err)
	}

	return updates
}

func TestCheckAllGameUpdatesFindsOutdatedGame(t *testing.T) {
	updates := checkGames(t, `{"Wuzzy/mineclone2": 42}`,
		content.Game{ID: "mineclone2", Author: "wuzzy", Release: 40})

	if len(updates) != 1 {
		t.Fatalf("got %d updates, want 1: %+v", len(updates), updates)
	}
	u := updates[0]
	if u.Author != "wuzzy" || u.Package != "mineclone2" || u.LatestRelease != 42 {
		t.Errorf("update = %+v, want wuzzy/mineclone2 at release 42", u)
	}
}

func TestCheckAllGameUpdatesSkipsUpToDateOrNewerGame(t *testing.T) {
	updates := checkGames(t, `{"wuzzy/mineclone2": 42}`,
		content.Game{ID: "mineclone2", Author: "wuzzy", Release: 42},
		content.Game{ID: "mineclone2", Author: "wuzzy", Release: 43})

	if len(updates) != 0 {
		t.Errorf("updates = %+v, want none", updates)
	}
}

func TestCheckAllGameUpdatesSkipsNoAuthorOrNoRelease(t *testing.T) {
	updates := checkGames(t, `{"wuzzy/mineclone2": 42, "/nobody": 5}`,
		content.Game{ID: "mineclone2", Author: "wuzzy", Release: 0, Path: t.TempDir()},
		content.Game{ID: "nobody", Author: "", Release: 1, Path: t.TempDir()})

	if len(updates) != 0 {
		t.Errorf("updates = %+v, want none", updates)
	}
}

func TestCheckAllGameUpdatesMatchesGameSuffix(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		folder      string
		wantPackage string
	}{
		{"folder and package suffixed", `{"jane/foo_game": 5}`, "foo_game", "foo_game"},
		{"only folder suffixed", `{"jane/foo": 5}`, "foo_game", "foo"},
		{"only package suffixed", `{"jane/foo_game": 5}`, "foo", "foo_game"},
		{"unsuffixed folder prefers unsuffixed package", `{"jane/foo": 5, "jane/foo_game": 9}`, "foo", "foo"},
		{"suffixed folder prefers suffixed package", `{"jane/foo": 9, "jane/foo_game": 5}`, "foo_game", "foo_game"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updates := checkGames(t, tt.body, content.Game{ID: tt.folder, Author: "jane", Release: 1})

			if len(updates) != 1 || updates[0].Package != tt.wantPackage {
				t.Errorf("updates = %+v, want 1 entry for package %q", updates, tt.wantPackage)
			}
		})
	}
}

func TestCheckAllGameUpdatesUnversionedMinetestGame(t *testing.T) {
	const body = `{"Luanti/minetest_game": 30000, "Minetest/minetest_game": 30000}`

	t.Run("bundled copy is updatable", func(t *testing.T) {
		updates := checkGames(t, body, content.Game{ID: "minetest_game", Path: t.TempDir()})

		if len(updates) != 1 {
			t.Fatalf("got %d updates, want 1: %+v", len(updates), updates)
		}
		u := updates[0]
		if u.Author != "Luanti" || u.Package != "minetest_game" || u.LatestRelease != 30000 {
			t.Errorf("update = %+v, want Luanti/minetest_game at release 30000", u)
		}
	})

	t.Run("git checkout is left alone", func(t *testing.T) {
		path := t.TempDir()
		if err := os.Mkdir(filepath.Join(path, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}

		if updates := checkGames(t, body, content.Game{ID: "minetest_game", Path: path}); len(updates) != 0 {
			t.Errorf("updates = %+v, want none", updates)
		}
	})

	t.Run("other unversioned games are not", func(t *testing.T) {
		updates := checkGames(t, `{"luanti/other": 5}`, content.Game{ID: "other", Path: t.TempDir()})

		if len(updates) != 0 {
			t.Errorf("updates = %+v, want none", updates)
		}
	})
}

func TestCheckAllGameUpdatesSendsEngineInfo(t *testing.T) {
	var query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query().Encode()
		w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)

	_, err := CheckAllGameUpdates(nil, contentdb.New(srv.URL), engine.Info{Version: "5.9.0", Protocol: 46})
	if err != nil {
		t.Fatalf("CheckAllGameUpdates: %v", err)
	}
	if want := "engine_version=5.9.0&protocol_version=46&type=game"; query != want {
		t.Errorf("query = %q, want %q", query, want)
	}
}

func TestCheckAllGameUpdatesPropagatesFetchError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	games := []content.Game{{ID: "foo", Author: "jane", Release: 1}}
	if _, err := CheckAllGameUpdates(games, contentdb.New(srv.URL), engine.Info{}); err == nil {
		t.Error("expected an error, got nil")
	}
}

func TestUpdateGameDownloadsAndInstallsLatestRelease(t *testing.T) {
	zipData := buildZip(t, "jane-foo_game-abc123", map[string]string{
		"game.conf":         "title = Foo\nname = Old Title\nrelease = 1\n",
		"mods/a/init.lua":   "-- hi\n",
		"mods/a/mod.conf":   "name = a\n",
		"settingtypes.txt":  "",
		"menu/icon.png.txt": "",
	})

	var requestedPath, requestedReason string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		requestedReason = r.URL.Query().Get("reason")
		w.Write(zipData)
	}))
	t.Cleanup(srv.Close)

	destDir := filepath.Join(t.TempDir(), "foo")
	stale := filepath.Join(destDir, "mods", "removed", "init.lua")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("-- old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	u := GameUpdate{
		Game:          content.Game{ID: "foo", Author: "jane", Release: 1, Path: destDir},
		Author:        "jane",
		Package:       "foo_game",
		LatestRelease: 42,
	}

	if err := UpdateGame(contentdb.New(srv.URL), u); err != nil {
		t.Fatalf("UpdateGame: %v", err)
	}

	if want := "/packages/jane/foo_game/releases/42/download/"; requestedPath != want {
		t.Errorf("requested path = %q, want %q", requestedPath, want)
	}
	if requestedReason != contentdb.ReasonUpdate {
		t.Errorf("reason = %q, want %q", requestedReason, contentdb.ReasonUpdate)
	}

	data, err := os.ReadFile(filepath.Join(destDir, "game.conf"))
	if err != nil {
		t.Fatalf("reading game.conf: %v", err)
	}
	if want := "title = Foo\nname = Old Title\nrelease = 42\nauthor = jane\n"; string(data) != want {
		t.Errorf("game.conf = %q, want %q", data, want)
	}

	if _, err := os.Stat(filepath.Join(destDir, "mods", "a", "init.lua")); err != nil {
		t.Errorf("new game content missing: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale file from the old release still present, err = %v", err)
	}
}
