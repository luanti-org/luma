package contentdb

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func newTestServer(t *testing.T) *Client {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/packages/alice/mymod/download/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/uploads/mymod.zip", http.StatusFound)
	})
	mux.HandleFunc("/packages/alice/mymod/releases/42/download/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("release-42"))
	})
	mux.HandleFunc("/uploads/mymod.zip", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("zip-bytes"))
	})
	mux.HandleFunc("/api/packages/alice/mymod/releases/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[
			{"id": 42, "title": "1.1", "url": "/uploads/mymod.zip", "size": 9, "min_minetest_version": {"name": "5.4.0"}},
			{"id": 41, "title": "1.0", "url": "/uploads/old.zip", "size": 7, "min_minetest_version": null}
		]`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return New(srv.URL)
}

func TestDownloadFollowsRedirect(t *testing.T) {
	c := newTestServer(t)

	var buf bytes.Buffer
	if err := c.Download(c.DownloadURL("alice", "mymod"), &buf); err != nil {
		t.Fatalf("Download: %v", err)
	}
	if got := buf.String(); got != "zip-bytes" {
		t.Errorf("body = %q, want %q", got, "zip-bytes")
	}
}

func TestDownloadRelease(t *testing.T) {
	c := newTestServer(t)

	var buf bytes.Buffer
	if err := c.Download(c.ReleaseDownloadURL("alice", "mymod", 42), &buf); err != nil {
		t.Fatalf("Download: %v", err)
	}
	if got := buf.String(); got != "release-42" {
		t.Errorf("body = %q, want %q", got, "release-42")
	}
}

// dripServer sends chunks of "x" with gap between them, then holds the connection open if hang is set
func dripServer(t *testing.T, chunks int, gap time.Duration, hang bool) *Client {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for i := 0; i < chunks; i++ {
			w.Write([]byte("x"))
			w.(http.Flusher).Flush()
			time.Sleep(gap)
		}
		if hang {
			<-r.Context().Done()
		}
	}))
	t.Cleanup(srv.Close)

	return New(srv.URL)
}

func TestDownloadOutlastsTimeout(t *testing.T) {
	c := dripServer(t, 6, 50*time.Millisecond, false)
	c.HTTPClient = &http.Client{Timeout: 200 * time.Millisecond}

	var buf bytes.Buffer
	if err := c.Download(c.BaseURL+"/", &buf); err != nil {
		t.Fatalf("Download: %v", err)
	}
	if got := buf.String(); got != "xxxxxx" {
		t.Errorf("body = %q, want %q", got, "xxxxxx")
	}
}

func TestDownloadStalled(t *testing.T) {
	c := dripServer(t, 1, 0, true)
	c.HTTPClient = &http.Client{Timeout: 100 * time.Millisecond}

	var buf bytes.Buffer
	err := c.Download(c.BaseURL+"/", &buf)
	if err == nil || !strings.Contains(err.Error(), "download stalled") {
		t.Fatalf("err = %v, want a stall error", err)
	}
}

func TestDownloadNotFound(t *testing.T) {
	c := newTestServer(t)

	var buf bytes.Buffer
	err := c.Download(c.DownloadURL("bob", "missing"), &buf)
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error %q does not mention status 404", err)
	}
	if buf.Len() != 0 {
		t.Errorf("wrote %d bytes on error, want 0", buf.Len())
	}
}

func TestReleases(t *testing.T) {
	c := newTestServer(t)

	releases, err := c.Releases("alice", "mymod")
	if err != nil {
		t.Fatalf("Releases: %v", err)
	}
	if len(releases) != 2 {
		t.Fatalf("got %d releases, want 2", len(releases))
	}
	if releases[0].ID != 42 || releases[0].Title != "1.1" {
		t.Errorf("releases[0] = %+v, want id 42 title 1.1", releases[0])
	}
	if v := releases[0].MinLuantiVersion; v == nil || v.Name != "5.4.0" {
		t.Errorf("releases[0].MinLuantiVersion = %+v, want 5.4.0", v)
	}
	if releases[1].MinLuantiVersion != nil {
		t.Errorf("releases[1].MinLuantiVersion = %+v, want nil", releases[1].MinLuantiVersion)
	}
}

func TestSearchQuery(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		w.Write([]byte(`[{"author": "alice", "name": "mymod", "type": "mod"}]`))
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	pkgs, err := c.Search(SearchOptions{Type: "mod", Query: "tree", Limit: 5, Sort: "downloads", Order: "desc"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(pkgs) != 1 || pkgs[0].Name != "mymod" {
		t.Errorf("pkgs = %+v, want one package named mymod", pkgs)
	}

	want := url.Values{"type": {"mod"}, "q": {"tree"}, "limit": {"5"}, "sort": {"downloads"}, "order": {"desc"}}
	if got.Encode() != want.Encode() {
		t.Errorf("query = %q, want %q", got.Encode(), want.Encode())
	}

	if _, err := c.Search(SearchOptions{}); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("zero options sent query %q, want none", got.Encode())
	}
}

func TestSearchFilterQuery(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		w.Write([]byte(`[]`))
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	_, err := c.Search(SearchOptions{
		Flag:            []string{"nonfree"},
		Hide:            []string{"nonfree", "android_default"},
		License:         []string{"MIT", "LGPL-2.1-or-later"},
		Game:            "Warr1024/nodecore",
		ProtocolVersion: 48,
		EngineVersion:   "5.9.0",
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	want := url.Values{
		"flag":             {"nonfree"},
		"hide":             {"nonfree", "android_default"},
		"license":          {"MIT", "LGPL-2.1-or-later"},
		"game":             {"Warr1024/nodecore"},
		"protocol_version": {"48"},
		"engine_version":   {"5.9.0"},
	}
	if got.Encode() != want.Encode() {
		t.Errorf("query = %q, want %q", got.Encode(), want.Encode())
	}
}

func TestUpdates(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		w.Write([]byte(`{"alice/mymod": 42, "bob/game": 7}`))
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	updates, err := c.Updates(UpdatesOptions{
		Types:           []string{"mod", "game", "txp"},
		ProtocolVersion: 48,
		EngineVersion:   "5.9.0",
	})
	if err != nil {
		t.Fatalf("Updates: %v", err)
	}
	if updates["alice/mymod"] != 42 || updates["bob/game"] != 7 {
		t.Errorf("updates = %v, want alice/mymod=42 bob/game=7", updates)
	}

	want := url.Values{"type": {"mod", "game", "txp"}, "protocol_version": {"48"}, "engine_version": {"5.9.0"}}
	if got.Encode() != want.Encode() {
		t.Errorf("query = %q, want %q", got.Encode(), want.Encode())
	}

	if _, err := c.Updates(UpdatesOptions{}); err != nil {
		t.Fatalf("Updates: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("zero options sent query %q, want none", got.Encode())
	}
}

func TestEngineVersions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/minetest_versions/" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`[
			{"is_dev": false, "name": "5.17", "protocol_version": 53},
			{"is_dev": true, "name": "5.18-dev", "protocol_version": 54}
		]`))
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	versions, err := c.EngineVersions()
	if err != nil {
		t.Fatalf("EngineVersions: %v", err)
	}

	want := []EngineVersion{
		{Name: "5.17", ProtocolVersion: 53},
		{Name: "5.18-dev", ProtocolVersion: 54, IsDev: true},
	}
	if len(versions) != len(want) {
		t.Fatalf("got %d versions, want %d", len(versions), len(want))
	}
	for i := range want {
		if versions[i] != want[i] {
			t.Errorf("versions[%d] = %+v, want %+v", i, versions[i], want[i])
		}
	}
}

func TestPackageDetails(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/packages/alice/mymod/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{
			"author": "alice", "name": "mymod", "title": "My Mod",
			"short_description": "short", "long_description": "long",
			"type": "mod", "release": 42, "license": "MIT",
			"repo": "https://example.com/alice/mymod",
			"tags": ["mobs", "survival"], "provides": ["mymod"],
			"maintainers": ["alice"], "downloads": 100, "score": 12.5,
			"state": "APPROVED", "dev_state": "BETA",
			"thumbnail": "https://example.com/t.png",
			"url": "https://example.com/packages/alice/mymod/download/",
			"supports_all_games": false, "screenshots": [],
			"content_warnings": ["violence", "horror"],
			"game_support": [
				{"confidence": 11, "supports": true, "game": {
					"author": "Luanti", "name": "minetest_game", "title": "Minetest Game",
					"type": "game", "release": 5, "aliases": ["Minetest/minetest_game"]}},
				{"confidence": 3, "supports": false, "game": {
					"author": "bob", "name": "othergame", "type": "game"}}
			]
		}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	d, err := c.PackageDetails("alice", "mymod")
	if err != nil {
		t.Fatalf("PackageDetails: %v", err)
	}
	if d.Author != "alice" || d.Name != "mymod" || d.Release != 42 || d.License != "MIT" {
		t.Errorf("details = %+v, want alice/mymod release 42 MIT", d)
	}
	if d.Repo != "https://example.com/alice/mymod" || d.Downloads != 100 || d.Score != 12.5 {
		t.Errorf("details = %+v, want repo, 100 downloads, score 12.5", d)
	}
	if len(d.Tags) != 2 || d.Tags[0] != "mobs" || len(d.Maintainers) != 1 {
		t.Errorf("tags/maintainers = %v / %v", d.Tags, d.Maintainers)
	}
	if d.URL != "https://example.com/packages/alice/mymod/download/" {
		t.Errorf("URL = %q", d.URL)
	}
	if len(d.ContentWarnings) != 2 || d.ContentWarnings[0] != "violence" || d.ContentWarnings[1] != "horror" {
		t.Errorf("ContentWarnings = %v, want [violence horror]", d.ContentWarnings)
	}
	if len(d.GameSupport) != 2 {
		t.Fatalf("GameSupport = %+v, want 2 entries", d.GameSupport)
	}
	if g := d.GameSupport[0]; !g.Supports || g.Confidence != 11 || g.Game.Author != "Luanti" || g.Game.Name != "minetest_game" || g.Game.Type != "game" {
		t.Errorf("GameSupport[0] = %+v, want supported minetest_game, confidence 11", g)
	}
	if g := d.GameSupport[1]; g.Supports || g.Game.Name != "othergame" {
		t.Errorf("GameSupport[1] = %+v, want unsupported othergame", g)
	}

	if _, err := c.PackageDetails("bob", "missing"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("missing package error = %v, want one mentioning 404", err)
	}
}

func TestDependencies(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		w.Write([]byte(`{
			"alice/mymod": [
				{"name": "core", "is_optional": false, "packages": ["bob/core", "carol/core"]},
				{"name": "extra", "is_optional": true, "packages": ["dave/extra"]}
			],
			"bob/core": []
		}`))
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	deps, err := c.Dependencies("alice", "mymod", false)
	if err != nil {
		t.Fatalf("Dependencies: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("onlyHard=false sent query %q, want none", got.Encode())
	}
	if len(deps) != 2 || len(deps["bob/core"]) != 0 {
		t.Fatalf("deps = %+v, want alice/mymod plus empty bob/core", deps)
	}
	mod := deps["alice/mymod"]
	if len(mod) != 2 {
		t.Fatalf("alice/mymod deps = %+v, want 2", mod)
	}
	if mod[0].Name != "core" || mod[0].IsOptional || len(mod[0].Packages) != 2 || mod[0].Packages[0] != "bob/core" {
		t.Errorf("mod[0] = %+v, want required core with 2 candidates", mod[0])
	}
	if mod[1].Name != "extra" || !mod[1].IsOptional {
		t.Errorf("mod[1] = %+v, want optional extra", mod[1])
	}

	if _, err := c.Dependencies("alice", "mymod", true); err != nil {
		t.Fatalf("Dependencies: %v", err)
	}
	if got.Get("only_hard") != "true" {
		t.Errorf("onlyHard=true sent query %q, want only_hard=true", got.Encode())
	}
}
