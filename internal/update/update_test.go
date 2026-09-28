package update

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/luanti-org/luma/internal/content"
	"github.com/luanti-org/luma/internal/contentdb"
	"github.com/luanti-org/luma/internal/engine"
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
