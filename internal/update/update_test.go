package update

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/luanti-org/luma/internal/content"
	"github.com/luanti-org/luma/internal/contentdb"
	"github.com/luanti-org/luma/internal/engine"
	"github.com/luanti-org/luma/internal/util"
)

func newTestServer(t *testing.T, body string) *contentdb.Client {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	return contentdb.New(srv.URL)
}

func TestCheckAllModUpdatesFindsOutdatedMod(t *testing.T) {
	client := newTestServer(t, `{"jane/mymod": 42, "john/other": 7}`)

	mods := []content.Mod{
		{Name: "mymod", Author: "jane", Release: 40, ConfOK: true},
	}

	updates, err := CheckAllModUpdates(mods, client, engine.Info{})
	if err != nil {
		t.Fatalf("CheckAllModUpdates: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("updates = %+v, want 1 entry", updates)
	}
	if updates[0].Mod.Name != "mymod" || updates[0].LatestRelease != 42 {
		t.Errorf("updates[0] = %+v, want mymod at release 42", updates[0])
	}
}

func TestCheckAllModUpdatesSkipsUpToDateMod(t *testing.T) {
	client := newTestServer(t, `{"jane/mymod": 42}`)

	mods := []content.Mod{
		{Name: "mymod", Author: "jane", Release: 42, ConfOK: true},
	}

	updates, err := CheckAllModUpdates(mods, client, engine.Info{})
	if err != nil {
		t.Fatalf("CheckAllModUpdates: %v", err)
	}
	if len(updates) != 0 {
		t.Errorf("updates = %+v, want none", updates)
	}
}

func TestCheckAllModUpdatesSkipsUnmatchedMod(t *testing.T) {
	client := newTestServer(t, `{"jane/mymod": 42}`)

	mods := []content.Mod{
		{Name: "othermod", Author: "carol", Release: 1, ConfOK: true},
	}

	updates, err := CheckAllModUpdates(mods, client, engine.Info{})
	if err != nil {
		t.Fatalf("CheckAllModUpdates: %v", err)
	}
	if len(updates) != 0 {
		t.Errorf("updates = %+v, want none", updates)
	}
}

func TestCheckAllModUpdatesSkipsNoAuthor(t *testing.T) {
	client := newTestServer(t, `{"jane/mymod": 42}`)

	mods := []content.Mod{
		{Name: "noauthor", Author: "", Release: 1},
	}

	updates, err := CheckAllModUpdates(mods, client, engine.Info{})
	if err != nil {
		t.Fatalf("CheckAllModUpdates: %v", err)
	}
	if len(updates) != 0 {
		t.Errorf("updates = %+v, want none (no author to key the lookup on)", updates)
	}
}

func TestCheckAllModUpdatesChecksModpacks(t *testing.T) {
	client := newTestServer(t, `{"john/pack": 5}`)

	mods := []content.Mod{
		{Name: "pack", Author: "john", Release: 1, IsModpack: true, ConfOK: true},
	}

	updates, err := CheckAllModUpdates(mods, client, engine.Info{})
	if err != nil {
		t.Fatalf("CheckAllModUpdates: %v", err)
	}
	if len(updates) != 1 || updates[0].LatestRelease != 5 {
		t.Errorf("updates = %+v, want 1 entry at release 5 (modpacks are checkable too)", updates)
	}
}

func TestCheckAllModUpdatesChecksFolderNameFallback(t *testing.T) {
	client := newTestServer(t, `{"jane/mymod": 42}`)

	// mod.conf had an author but no valid name, so Name fell back to the
	// folder name (ConfOK false) -- that's still a usable lookup key.
	mods := []content.Mod{
		{Name: "mymod", Author: "jane", Release: 1, ConfOK: false},
	}

	updates, err := CheckAllModUpdates(mods, client, engine.Info{})
	if err != nil {
		t.Fatalf("CheckAllModUpdates: %v", err)
	}
	if len(updates) != 1 || updates[0].LatestRelease != 42 {
		t.Errorf("updates = %+v, want 1 entry at release 42", updates)
	}
}

func TestCheckAllModUpdatesSendsEngineInfo(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	client := contentdb.New(srv.URL)

	mods := []content.Mod{{Name: "mymod", Author: "jane", Release: 1, ConfOK: true}}

	if _, err := CheckAllModUpdates(mods, client, engine.Info{Version: "5.9.0", Protocol: 48}); err != nil {
		t.Fatalf("CheckAllModUpdates: %v", err)
	}

	want := url.Values{"type": {"mod"}, "protocol_version": {"48"}, "engine_version": {"5.9.0"}}
	if got.Encode() != want.Encode() {
		t.Errorf("query = %q, want %q", got.Encode(), want.Encode())
	}
}

func TestCheckAllModUpdatesUnknownInstalledReleaseCountsAsOutdated(t *testing.T) {
	client := newTestServer(t, `{"jane/mymod": 42}`)

	mods := []content.Mod{
		{Name: "mymod", Author: "jane", Release: 0, ConfOK: true},
	}

	updates, err := CheckAllModUpdates(mods, client, engine.Info{})
	if err != nil {
		t.Fatalf("CheckAllModUpdates: %v", err)
	}
	if len(updates) != 1 || updates[0].LatestRelease != 42 {
		t.Errorf("updates = %+v, want 1 entry at release 42", updates)
	}
}

func TestUpdateModDownloadsAndInstallsLatestRelease(t *testing.T) {
	zipData := buildZip(t, "jane-mymod-abc123", map[string]string{
		"mod.conf": "name = mymod\ndepends = default\n",
		"init.lua": "-- hi\n",
	})

	var requestedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.Write(zipData)
	}))
	t.Cleanup(srv.Close)
	client := contentdb.New(srv.URL)

	destDir := t.TempDir()
	u := ModUpdate{
		Mod:           content.Mod{Name: "mymod", Author: "jane", Path: destDir},
		LatestRelease: 42,
	}

	if err := UpdateMod(client, u); err != nil {
		t.Fatalf("UpdateMod: %v", err)
	}

	wantPath := "/packages/jane/mymod/releases/42/download/"
	if requestedPath != wantPath {
		t.Errorf("requested path = %q, want %q", requestedPath, wantPath)
	}

	data, err := os.ReadFile(filepath.Join(destDir, "mod.conf"))
	if err != nil {
		t.Fatalf("reading mod.conf: %v", err)
	}
	if want := "name = mymod\ndepends = default\nauthor = jane\nrelease = 42\n"; string(data) != want {
		t.Errorf("mod.conf = %q, want %q", data, want)
	}
}

func TestUpdateModWritesModpackConf(t *testing.T) {
	zipData := buildZip(t, "john-pack-abc123", map[string]string{
		"modpack.conf": "name = pack\n",
		"sub/init.lua": "-- hi\n",
	})
	client, _ := newDownloadTestServer(t, zipData)

	destDir := t.TempDir()
	u := ModUpdate{
		Mod:           content.Mod{Name: "pack", Author: "john", Path: destDir, IsModpack: true},
		LatestRelease: 5,
	}

	if err := UpdateMod(client, u); err != nil {
		t.Fatalf("UpdateMod: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(destDir, "modpack.conf"))
	if err != nil {
		t.Fatalf("reading modpack.conf: %v", err)
	}
	if want := "name = pack\nauthor = john\nrelease = 5\n"; string(data) != want {
		t.Errorf("modpack.conf = %q, want %q", data, want)
	}
	if util.FileExists(filepath.Join(destDir, "mod.conf")) {
		t.Error("mod.conf should not be created for a modpack")
	}
}

func TestCheckAllModUpdatesPropagatesFetchError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	client := contentdb.New(srv.URL)

	mods := []content.Mod{{Name: "mymod", Author: "jane", Release: 1, ConfOK: true}}

	if _, err := CheckAllModUpdates(mods, client, engine.Info{}); err == nil {
		t.Fatal("CheckAllModUpdates: expected an error, got nil")
	}
}
