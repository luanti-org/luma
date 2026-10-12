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

func checkTexturepacks(t *testing.T, body string, texturepacks ...content.Texturepack) []TexturepackUpdate {
	t.Helper()

	updates, err := CheckAllTexturepackUpdates(texturepacks, newTestServer(t, body), engine.Info{})
	if err != nil {
		t.Fatalf("CheckAllTexturepackUpdates: %v", err)
	}

	return updates
}

func TestCheckAllTexturepackUpdatesFindsOutdatedTexturepack(t *testing.T) {
	updates := checkTexturepacks(t, `{"Jane/soothing32": 42}`,
		content.Texturepack{Name: "soothing32", Dir: "soothing32", Author: "jane", Release: 40})

	if len(updates) != 1 {
		t.Fatalf("got %d updates, want 1: %+v", len(updates), updates)
	}
	if u := updates[0]; u.Texturepack.Name != "soothing32" || u.LatestRelease != 42 {
		t.Errorf("update = %+v, want soothing32 at release 42", u)
	}
}

func TestCheckAllTexturepackUpdatesSkipsUpToDateOrNewerTexturepack(t *testing.T) {
	updates := checkTexturepacks(t, `{"jane/soothing32": 42}`,
		content.Texturepack{Name: "soothing32", Author: "jane", Release: 42},
		content.Texturepack{Name: "soothing32", Author: "jane", Release: 43})

	if len(updates) != 0 {
		t.Errorf("updates = %+v, want none", updates)
	}
}

func TestCheckAllTexturepackUpdatesSkipsNoAuthorOrNoRelease(t *testing.T) {
	updates := checkTexturepacks(t, `{"jane/soothing32": 42, "/nobody": 5}`,
		content.Texturepack{Name: "soothing32", Author: "jane", Release: 0},
		content.Texturepack{Name: "nobody", Author: "", Release: 1})

	if len(updates) != 0 {
		t.Errorf("updates = %+v, want none", updates)
	}
}

func TestCheckAllTexturepackUpdatesMatchesConfNameNotFolder(t *testing.T) {
	updates := checkTexturepacks(t, `{"jane/soothing32": 42, "jane/renamed": 99}`,
		content.Texturepack{Name: "soothing32", Dir: "renamed", Author: "jane", Release: 40})

	if len(updates) != 1 || updates[0].LatestRelease != 42 {
		t.Errorf("updates = %+v, want 1 entry at release 42", updates)
	}
}

func TestCheckAllTexturepackUpdatesSendsEngineInfo(t *testing.T) {
	var query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query().Encode()
		w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)

	_, err := CheckAllTexturepackUpdates(nil, contentdb.New(srv.URL), engine.Info{Version: "5.9.0", Protocol: 46})
	if err != nil {
		t.Fatalf("CheckAllTexturepackUpdates: %v", err)
	}
	if want := "engine_version=5.9.0&protocol_version=46&type=txp"; query != want {
		t.Errorf("query = %q, want %q", query, want)
	}
}

func TestCheckAllTexturepackUpdatesPropagatesFetchError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	texturepacks := []content.Texturepack{{Name: "foo", Author: "jane", Release: 1}}
	if _, err := CheckAllTexturepackUpdates(texturepacks, contentdb.New(srv.URL), engine.Info{}); err == nil {
		t.Error("expected an error, got nil")
	}
}

func TestUpdateTexturepackDownloadsAndInstallsLatestRelease(t *testing.T) {
	zipData := buildZip(t, "jane-soothing32-abc123", map[string]string{
		"texture_pack.conf": "title = Soothing 32\nrelease = 1\n",
		"default_dirt.png":  "",
	})

	var requestedPath, requestedReason string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		requestedReason = r.URL.Query().Get("reason")
		w.Write(zipData)
	}))
	t.Cleanup(srv.Close)

	destDir := filepath.Join(t.TempDir(), "renamed")
	stale := filepath.Join(destDir, "default_stone.png")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	u := TexturepackUpdate{
		Texturepack:   content.Texturepack{Name: "soothing32", Dir: "renamed", Author: "jane", Release: 1, Path: destDir},
		LatestRelease: 42,
	}

	if err := UpdateTexturepack(contentdb.New(srv.URL), u); err != nil {
		t.Fatalf("UpdateTexturepack: %v", err)
	}

	if want := "/packages/jane/soothing32/releases/42/download/"; requestedPath != want {
		t.Errorf("requested path = %q, want %q", requestedPath, want)
	}
	if requestedReason != contentdb.ReasonUpdate {
		t.Errorf("reason = %q, want %q", requestedReason, contentdb.ReasonUpdate)
	}

	data, err := os.ReadFile(filepath.Join(destDir, "texture_pack.conf"))
	if err != nil {
		t.Fatalf("reading texture_pack.conf: %v", err)
	}
	if want := "title = Soothing 32\nrelease = 42\nauthor = jane\nname = soothing32\n"; string(data) != want {
		t.Errorf("texture_pack.conf = %q, want %q", data, want)
	}

	if _, err := os.Stat(filepath.Join(destDir, "default_dirt.png")); err != nil {
		t.Errorf("new texture pack content missing: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale file from the old release still present, err = %v", err)
	}
}
