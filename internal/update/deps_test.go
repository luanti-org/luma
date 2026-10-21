package update

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/luanti-org/luma/internal/content"
	"github.com/luanti-org/luma/internal/contentdb"
	"github.com/luanti-org/luma/internal/engine"
)

// newDepsServer serves each body at /api/packages/<id>/dependencies/ and records the ids requested.
// The body for "providers" is served at /api/dependencies/, recorded as "providers".
func newDepsServer(t *testing.T, bodies map[string]string) (*contentdb.Client, *[]string) {
	t.Helper()
	t.Setenv("LUMA_CACHE_DIR", t.TempDir())

	var requested []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/dependencies/" {
			requested = append(requested, "providers")
			body, ok := bodies["providers"]
			if !ok {
				body = `{"page_count": 1, "items": []}`
			}
			w.Write([]byte(body))
			return
		}
		for id, body := range bodies {
			if r.URL.Path == "/api/packages/"+id+"/dependencies/" {
				requested = append(requested, id)
				w.Write([]byte(body))
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	return contentdb.New(srv.URL), &requested
}

func index(pkgs ...contentdb.Package) map[string]contentdb.Package {
	out := make(map[string]contentdb.Package, len(pkgs))
	for _, p := range pkgs {
		out[packageKey(p.Author+"/"+p.Name)] = p
	}
	return out
}

func mod(author, name string, release int) contentdb.Package {
	return contentdb.Package{Author: author, Name: name, Type: "mod", Release: release}
}

func installIDs(plan DepPlan) []string {
	var ids []string
	for _, d := range plan.Install {
		ids = append(ids, d.Package.Author+"/"+d.Package.Name)
	}
	return ids
}

func TestResolveDepsPicksExactNameAndRecurses(t *testing.T) {
	client, _ := newDepsServer(t, map[string]string{
		"jane/root": `{"jane/root": [
			{"name": "lib", "is_optional": false, "packages": ["gm/somegame", "fred/libfork", "amy/lib"]},
			{"name": "extra", "is_optional": true, "packages": ["amy/extra"]}
		]}`,
		"amy/lib": `{"amy/lib": [
			{"name": "base", "is_optional": false, "packages": ["gm/somegame", "zed/basepack"]}
		]}`,
		"zed/basepack": `{"zed/basepack": []}`,
	})

	packages := index(
		contentdb.Package{Author: "gm", Name: "somegame", Type: "game", Release: 1},
		mod("fred", "libfork", 2), mod("amy", "lib", 3), mod("amy", "extra", 4), mod("zed", "basepack", 5),
	)

	plan, err := ResolveDeps(client, []string{"jane/root"}, map[string]bool{}, packages, nil)
	if err != nil {
		t.Fatalf("ResolveDeps: %v", err)
	}

	if got, want := installIDs(plan), []string{"amy/lib", "zed/basepack"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("install = %v, want %v", got, want)
	}
	if len(plan.NotFound) != 0 {
		t.Errorf("not found = %+v, want none", plan.NotFound)
	}

	lib := plan.Install[0]
	if lib.Dep != "lib" || lib.RequiredBy != "jane/root" || lib.Package.Release != 3 {
		t.Errorf("install[0] = %+v, want lib release 3 required by jane/root", lib)
	}
	base := plan.Install[1]
	if base.Dep != "base" || base.RequiredBy != "amy/lib" {
		t.Errorf("install[1] = %+v, want base required by amy/lib", base)
	}
}

func TestResolveDepsSkipsInstalled(t *testing.T) {
	client, requested := newDepsServer(t, map[string]string{
		"jane/root": `{"jane/root": [
			{"name": "lib", "is_optional": false, "packages": ["amy/lib"]}
		]}`,
	})

	plan, err := ResolveDeps(client, []string{"jane/root"}, map[string]bool{"lib": true}, index(mod("amy", "lib", 3)), nil)
	if err != nil {
		t.Fatalf("ResolveDeps: %v", err)
	}
	if len(plan.Install) != 0 || len(plan.NotFound) != 0 {
		t.Errorf("plan = %+v, want empty", plan)
	}
	if len(*requested) != 1 {
		t.Errorf("requested = %v, want only the root", *requested)
	}
}

func TestResolveDepsPrefersPackageAlreadyInPlan(t *testing.T) {
	// pack_extra comes first and has no exact match, but amy/pack is chosen for "pack" and provides it too
	client, _ := newDepsServer(t, map[string]string{
		"jane/root": `{"jane/root": [
			{"name": "pack_extra", "is_optional": false, "packages": ["fred/forkpack", "amy/pack"]},
			{"name": "pack", "is_optional": false, "packages": ["fred/forkpack", "amy/pack"]}
		]}`,
		"amy/pack": `{"amy/pack": []}`,
	})

	packages := index(mod("fred", "forkpack", 1), mod("amy", "pack", 2))

	plan, err := ResolveDeps(client, []string{"jane/root"}, map[string]bool{}, packages, nil)
	if err != nil {
		t.Fatalf("ResolveDeps: %v", err)
	}
	if got, want := installIDs(plan), []string{"amy/pack"}; !reflect.DeepEqual(got, want) {
		t.Errorf("install = %v, want %v", got, want)
	}
}

func TestResolveDepsRootProvidesDepOfAnotherRoot(t *testing.T) {
	client, _ := newDepsServer(t, map[string]string{
		"jane/root":    `{"jane/root": [{"name": "lib", "is_optional": false, "packages": ["amy/lib", "Fred/LibFork"]}]}`,
		"fred/libfork": `{"fred/libfork": []}`,
	})

	packages := index(mod("amy", "lib", 3), mod("Fred", "LibFork", 2))

	plan, err := ResolveDeps(client, []string{"jane/root", "fred/libfork"}, map[string]bool{}, packages, nil)
	if err != nil {
		t.Fatalf("ResolveDeps: %v", err)
	}
	if len(plan.Install) != 0 || len(plan.NotFound) != 0 {
		t.Errorf("plan = %+v, want empty", plan)
	}
}

func TestResolveDepsReportsMissingAndExcluded(t *testing.T) {
	client, _ := newDepsServer(t, map[string]string{
		"jane/root": `{"jane/root": [
			{"name": "gameonly", "is_optional": false, "packages": ["gm/somegame"]},
			{"name": "toonew", "is_optional": false, "packages": ["amy/toonew"]},
			{"name": "nobody", "is_optional": false, "packages": []},
			{"name": "skipdep", "is_optional": false, "packages": ["amy/skipdep"]},
			{"name": "viapkg", "is_optional": false, "packages": ["amy/skippkg"]}
		]}`,
	})

	// amy/toonew is left out, as a package with no compatible release would be
	packages := index(
		contentdb.Package{Author: "gm", Name: "somegame", Type: "game", Release: 1},
		mod("amy", "skipdep", 1), mod("amy", "skippkg", 1),
	)
	excluded := map[string]bool{"skipdep": true, "skippkg": true}

	plan, err := ResolveDeps(client, []string{"jane/root"}, map[string]bool{}, packages, excluded)
	if err != nil {
		t.Fatalf("ResolveDeps: %v", err)
	}
	if len(plan.Install) != 0 {
		t.Errorf("install = %v, want none", installIDs(plan))
	}

	got := make(map[string]bool)
	for _, m := range plan.NotFound {
		if m.RequiredBy != "jane/root" {
			t.Errorf("%s: required by %q, want jane/root", m.Dep, m.RequiredBy)
		}
		got[m.Dep] = m.Excluded

		if wantPkg := map[string]string{"viapkg": "amy/skippkg"}[m.Dep]; m.Package != wantPkg {
			t.Errorf("%s: package = %q, want %q", m.Dep, m.Package, wantPkg)
		}
		if m.GameOnly != (m.Dep == "gameonly") {
			t.Errorf("%s: game only = %v", m.Dep, m.GameOnly)
		}
	}

	want := map[string]bool{"gameonly": false, "toonew": false, "nobody": false, "skipdep": true, "viapkg": true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("not found (dep -> excluded) = %v, want %v", got, want)
	}
}

func TestResolveDepsReusesNestedResponse(t *testing.T) {
	// ContentDB includes the first candidate's own deps in the root's response
	client, requested := newDepsServer(t, map[string]string{
		"jane/root": `{
			"jane/root": [{"name": "lib", "is_optional": false, "packages": ["amy/lib"]}],
			"amy/lib": [{"name": "base", "is_optional": false, "packages": ["zed/base"]}]
		}`,
		"zed/base": `{"zed/base": []}`,
	})

	plan, err := ResolveDeps(client, []string{"jane/root"}, map[string]bool{}, index(mod("amy", "lib", 3), mod("zed", "base", 5)), nil)
	if err != nil {
		t.Fatalf("ResolveDeps: %v", err)
	}
	if got, want := installIDs(plan), []string{"amy/lib", "zed/base"}; !reflect.DeepEqual(got, want) {
		t.Errorf("install = %v, want %v", got, want)
	}
	if want := []string{"jane/root", "zed/base"}; !reflect.DeepEqual(*requested, want) {
		t.Errorf("requested = %v, want %v", *requested, want)
	}
}

func TestResolveDepsHandlesCycle(t *testing.T) {
	client, _ := newDepsServer(t, map[string]string{
		"jane/root": `{"jane/root": [{"name": "lib", "is_optional": false, "packages": ["amy/lib"]}]}`,
		"amy/lib":   `{"amy/lib": [{"name": "root", "is_optional": false, "packages": ["jane/root"]}]}`,
	})

	plan, err := ResolveDeps(client, []string{"jane/root"}, map[string]bool{}, index(mod("amy", "lib", 3), mod("jane", "root", 1)), nil)
	if err != nil {
		t.Fatalf("ResolveDeps: %v", err)
	}
	if got, want := installIDs(plan), []string{"amy/lib"}; !reflect.DeepEqual(got, want) {
		t.Errorf("install = %v, want %v", got, want)
	}
}

func TestResolveDepsFetchError(t *testing.T) {
	client, _ := newDepsServer(t, map[string]string{})

	if _, err := ResolveDeps(client, []string{"jane/root"}, map[string]bool{}, nil, nil); err == nil {
		t.Error("expected an error when the dependencies request fails")
	}
}

func TestFetchPackageIndex(t *testing.T) {
	var query string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		w.Write([]byte(`[{"author": "Amy", "name": "lib", "type": "mod", "release": 3}]`))
	}))
	t.Cleanup(srv.Close)

	packages, err := FetchPackageIndex(contentdb.New(srv.URL), engine.Info{Protocol: 48, Version: "5.13.0"})
	if err != nil {
		t.Fatalf("FetchPackageIndex: %v", err)
	}

	if p := packages["amy/lib"]; p.Author != "Amy" || p.Release != 3 {
		t.Errorf("packages = %+v, want Amy/lib release 3 under a lowercase key", packages)
	}
	if query != "engine_version=5.13.0&protocol_version=48" {
		t.Errorf("query = %q, want both version params", query)
	}
}

func TestResolveModDepsUsesUpdatesAsRoots(t *testing.T) {
	client, _ := newDepsServer(t, map[string]string{
		"jane/root": `{"jane/root": [{"name": "lib", "is_optional": false, "packages": ["amy/lib"]}]}`,
		"amy/lib":   `{"amy/lib": []}`,
	})

	updates := []ModUpdate{{Mod: content.Mod{Name: "root", Author: "jane"}, LatestRelease: 2}}

	plan, err := ResolveModDeps(client, updates, map[string]bool{}, index(mod("amy", "lib", 3)), nil)
	if err != nil {
		t.Fatalf("ResolveModDeps: %v", err)
	}
	if got, want := installIDs(plan), []string{"amy/lib"}; !reflect.DeepEqual(got, want) {
		t.Errorf("install = %v, want %v", got, want)
	}
}

func TestListDeps(t *testing.T) {
	client, requested := newDepsServer(t, map[string]string{
		"jane/root": `{
			"Jane/Root": [
				{"name": "lib", "is_optional": false, "packages": ["amy/lib"]},
				{"name": "extra", "is_optional": true, "packages": ["gm/somegame", "fred/extrafork", "amy/extra"]},
				{"name": "have", "is_optional": true, "packages": ["amy/have"]},
				{"name": "gameonly", "is_optional": true, "packages": ["gm/somegame"]},
				{"name": "forked", "is_optional": true, "packages": ["fred/extrafork"]},
				{"name": "nowhere", "is_optional": true, "packages": []}
			],
			"amy/lib": [{"name": "deep", "is_optional": true, "packages": ["amy/deep"]}]
		}`,
	})

	packages := index(
		contentdb.Package{Author: "gm", Name: "somegame", Type: "game", Release: 1},
		mod("fred", "extrafork", 2), mod("amy", "extra", 3), mod("amy", "have", 4), mod("amy", "deep", 5),
	)

	got, err := ListDeps(client, "jane/root", map[string]bool{"have": true}, packages)
	if err != nil {
		t.Fatalf("ListDeps: %v", err)
	}

	want := []ModDep{
		{Dep: "lib", RequiredBy: "jane/root"},
		{Dep: "extra", RequiredBy: "jane/root", Optional: true, Available: true, Package: mod("amy", "extra", 3)},
		{Dep: "have", RequiredBy: "jane/root", Optional: true, Installed: true, Available: true, Package: mod("amy", "have", 4)},
		{Dep: "gameonly", RequiredBy: "jane/root", Optional: true, GameOnly: true},
		{Dep: "forked", RequiredBy: "jane/root", Optional: true, Available: true, Package: mod("fred", "extrafork", 2)},
		{Dep: "nowhere", RequiredBy: "jane/root", Optional: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("deps = %+v, want %+v", got, want)
	}
	if len(*requested) != 1 {
		t.Errorf("requested = %v, want only the package itself", *requested)
	}

	if _, err := ListDeps(client, "noslash", nil, nil); err == nil {
		t.Error("expected an error for an id without an author")
	}
	if _, err := ListDeps(client, "jane/unknown", nil, nil); err == nil {
		t.Error("expected an error when the dependencies request fails")
	}
}

func TestListLocalDeps(t *testing.T) {
	client, requested := newDepsServer(t, map[string]string{
		"providers": `{"page_count": 1, "items": [
			{"author": "jeija", "name": "mesecons", "provides": ["mesecons", "mesecons_mvps"]},
			{"author": "carol", "name": "extras", "provides": ["both"]},
			{"author": "dan", "name": "more", "provides": ["both"]},
			{"author": "gm", "name": "somegame", "provides": ["fromgame", "mesecons_mvps"]}
		]}`,
	})

	packages := index(
		contentdb.Package{Author: "gm", Name: "somegame", Type: "game", Release: 1},
		mod("amy", "lib", 2), mod("jeija", "mesecons", 3), mod("carol", "extras", 4), mod("dan", "more", 5),
	)
	m := content.Mod{Name: "solo", Depends: []string{"have", "lib", "mesecons_mvps", "fromgame"},
		OptionalDepends: []string{"both", "nowhere"}}

	got, err := ListLocalDeps(client, m, map[string]bool{"have": true}, packages)
	if err != nil {
		t.Fatalf("ListLocalDeps: %v", err)
	}

	want := []ModDep{
		{Dep: "have", RequiredBy: "solo", Installed: true},
		{Dep: "lib", RequiredBy: "solo", Available: true, Package: mod("amy", "lib", 2)},
		{Dep: "mesecons_mvps", RequiredBy: "solo", Available: true, Package: mod("jeija", "mesecons", 3)},
		{Dep: "fromgame", RequiredBy: "solo", GameOnly: true},
		{Dep: "both", RequiredBy: "solo", Optional: true, Ambiguous: true},
		{Dep: "nowhere", RequiredBy: "solo", Optional: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("deps = %+v, want %+v", got, want)
	}
	if slices.ContainsFunc(*requested, func(id string) bool { return id != "providers" }) {
		t.Errorf("requested = %v, want only the providers", *requested)
	}
}

func TestListLocalDepsOptionalNoRefetch(t *testing.T) {
	client, requested := newDepsServer(t, nil)
	m := content.Mod{Name: "solo", OptionalDepends: []string{"nowhere"}}

	if _, err := ListLocalDeps(client, m, nil, nil); err != nil {
		t.Fatalf("ListLocalDeps: %v", err)
	}
	ageCache(t, time.Hour)

	if _, err := ListLocalDeps(client, m, nil, nil); err != nil {
		t.Fatalf("second ListLocalDeps: %v", err)
	}
	if n := countOf(*requested, "providers"); n != 1 {
		t.Errorf("providers fetched %d times, want 1 as a missing optional dep doesn't refetch", n)
	}

	m.Depends = []string{"nowhere_hard"}
	if _, err := ListLocalDeps(client, m, nil, nil); err != nil {
		t.Fatalf("third ListLocalDeps: %v", err)
	}
	if n := countOf(*requested, "providers"); n != 2 {
		t.Errorf("providers fetched %d times in all, want 2 with one refetch for the missing hard dep", n)
	}
}

func TestListLocalDepsNoProviderLookup(t *testing.T) {
	client, requested := newDepsServer(t, nil)
	m := content.Mod{Name: "solo", Depends: []string{"have", "lib"}}

	got, err := ListLocalDeps(client, m, map[string]bool{"have": true}, index(mod("amy", "lib", 2)))
	if err != nil {
		t.Fatalf("ListLocalDeps: %v", err)
	}
	if len(got) != 2 || !got[1].Available {
		t.Errorf("deps = %+v", got)
	}
	if len(*requested) != 0 {
		t.Errorf("requested = %v, want nothing when every dep is installed or named like a package", *requested)
	}
}

func TestPlanDeps(t *testing.T) {
	client, _ := newDepsServer(t, map[string]string{
		"amy/extra": `{"amy/extra": [
			{"name": "base", "is_optional": false, "packages": ["zed/base"]},
			{"name": "pack", "is_optional": false, "packages": ["amy/pack"]},
			{"name": "deep", "is_optional": true, "packages": ["amy/deep"]},
			{"name": "gone", "is_optional": false, "packages": []}
		]}`,
		"amy/pack": `{"amy/pack": []}`,
		"zed/base": `{"zed/base": []}`,
	})

	packages := index(mod("amy", "extra", 3), mod("amy", "pack", 4), mod("zed", "base", 5), mod("amy", "deep", 6))

	picks := []ModDep{
		{Dep: "extra", RequiredBy: "jane/root", Available: true, Package: mod("amy", "extra", 3)},
		{Dep: "pack", RequiredBy: "jane/root", Available: true, Package: mod("amy", "pack", 4)},
		{Dep: "pack_part", RequiredBy: "jane/root", Available: true, Package: mod("amy", "pack", 4)},
		{Dep: "have", RequiredBy: "jane/root", Installed: true, Available: true, Package: mod("amy", "have", 7)},
		{Dep: "gameonly", RequiredBy: "jane/root"},
	}

	plan, err := PlanDeps(client, picks, map[string]bool{"have": true}, packages, nil)
	if err != nil {
		t.Fatalf("PlanDeps: %v", err)
	}

	if got, want := installIDs(plan), []string{"amy/extra", "amy/pack", "zed/base"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("install = %v, want %v", got, want)
	}
	if extra := plan.Install[0]; extra.Dep != "extra" || extra.RequiredBy != "jane/root" {
		t.Errorf("install[0] = %+v, want extra required by jane/root", extra)
	}
	if base := plan.Install[2]; base.Dep != "base" || base.RequiredBy != "amy/extra" {
		t.Errorf("install[2] = %+v, want base required by amy/extra", base)
	}

	if want := []MissingDep{{Dep: "gone", RequiredBy: "amy/extra"}}; !reflect.DeepEqual(plan.NotFound, want) {
		t.Errorf("not found = %+v, want %+v", plan.NotFound, want)
	}
}

func TestInstallDep(t *testing.T) {
	zipData := buildZip(t, "amy-lib-abc123", map[string]string{"mod.conf": "name = lib\n", "init.lua": "-- hi\n"})

	var requests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path+"?reason="+r.URL.Query().Get("reason"))
		w.Write(zipData)
	}))
	t.Cleanup(srv.Close)
	client := contentdb.New(srv.URL)

	modsDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(modsDir, "taken"), 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	if err := InstallDep(client, modsDir, DepInstall{Package: mod("amy", "lib", 3)}); err != nil {
		t.Fatalf("InstallDep: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(modsDir, "lib", "mod.conf"))
	if err != nil {
		t.Fatalf("reading mod.conf: %v", err)
	}
	if want := "name = lib\nauthor = amy\nrelease = 3\n"; string(data) != want {
		t.Errorf("mod.conf = %q, want %q", data, want)
	}
	if want := []string{"/packages/amy/lib/releases/3/download/?reason=" + contentdb.ReasonDependency}; !reflect.DeepEqual(requests, want) {
		t.Errorf("requests = %v, want %v", requests, want)
	}

	for _, name := range []string{"taken", "lib.d", "../up"} {
		if err := InstallDep(client, modsDir, DepInstall{Package: mod("amy", name, 1)}); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if len(requests) != 1 {
		t.Errorf("requests = %v, want none for the refused installs", requests)
	}
}

func TestResolveInstallDeps(t *testing.T) {
	client, requested := newDepsServer(t, map[string]string{
		// the published release, which needs "dropped" and doesn't know "newdep" yet
		"jane/mymod": `{"jane/mymod": [
			{"name": "inpack", "is_optional": false, "packages": ["amy/bigpack"]},
			{"name": "dropped", "is_optional": false, "packages": ["amy/dropped"]}
		]}`,
		"amy/bigpack": `{"amy/bigpack": []}`,
		"bob/newdep":  `{"bob/newdep": [{"name": "deeper", "is_optional": false, "packages": ["bob/deeper"]}]}`,
		"bob/deeper":  `{"bob/deeper": []}`,
	})

	packages := index(
		mod("jane", "mymod", 1), mod("amy", "bigpack", 2), mod("amy", "dropped", 3),
		mod("bob", "newdep", 4), mod("bob", "deeper", 5),
		mod("bob", "dupe", 6), mod("sam", "dupe", 7),
		contentdb.Package{Author: "gm", Name: "nowhere", Type: "game", Release: 1},
	)

	locals := []LocalPackage{
		{Type: "mod", Name: "mymod", Author: "jane", Mod: content.Mod{
			Name: "mymod", HasInit: true, Depends: []string{"inpack", "newdep", "dupe", "have", "other", "nowhere"},
		}},
		// its author doesn't name a package in the index, so it isn't looked up
		{Type: "mod", Name: "other", Author: "zed", Mod: content.Mod{Name: "other", HasInit: true, Depends: []string{"newdep"}}},
		{Type: "game", Name: "agame"},
	}

	plan, err := ResolveInstallDeps(client, nil, locals, map[string]bool{"have": true}, packages)
	if err != nil {
		t.Fatalf("ResolveInstallDeps: %v", err)
	}

	if got, want := installIDs(plan), []string{"bob/newdep", "amy/bigpack", "bob/deeper"}; !reflect.DeepEqual(got, want) {
		t.Errorf("install = %v, want %v", got, want)
	}
	if plan.Install[0].RequiredBy != "mymod" {
		t.Errorf("required by = %q, want the local mod name", plan.Install[0].RequiredBy)
	}

	got := make(map[string]bool)
	for _, m := range plan.NotFound {
		got[m.Dep] = m.Ambiguous
	}
	if want := map[string]bool{"dupe": true, "nowhere": false}; !reflect.DeepEqual(got, want) {
		t.Errorf("not found (dep -> ambiguous) = %v, want %v", got, want)
	}

	if slices.Contains(*requested, "zed/other") {
		t.Errorf("requested = %v, want no lookup for zed/other", *requested)
	}
}

func TestResolveInstallDepsWithRoots(t *testing.T) {
	client, _ := newDepsServer(t, map[string]string{
		"jane/thing": `{"jane/thing": [{"name": "lib", "is_optional": false, "packages": ["amy/lib"]}]}`,
		"amy/lib":    `{"amy/lib": []}`,
	})
	packages := index(mod("jane", "thing", 1), mod("amy", "lib", 2))

	// both need lib, which is installed once
	locals := []LocalPackage{{Type: "mod", Name: "mymod", Mod: content.Mod{Name: "mymod", HasInit: true, Depends: []string{"lib", "thing"}}}}

	plan, err := ResolveInstallDeps(client, []string{"jane/thing"}, locals, map[string]bool{}, packages)
	if err != nil {
		t.Fatalf("ResolveInstallDeps: %v", err)
	}
	if got, want := installIDs(plan), []string{"amy/lib"}; !reflect.DeepEqual(got, want) {
		t.Errorf("install = %v, want %v", got, want)
	}
	if len(plan.NotFound) != 0 {
		t.Errorf("not found = %+v", plan.NotFound)
	}
}

func TestResolveInstallDepsProviders(t *testing.T) {
	client, requested := newDepsServer(t, map[string]string{
		"providers": `{"page_count": 1, "items": [
			{"author": "jeija", "name": "mesecons", "provides": ["mesecons", "mesecons_mvps"]},
			{"author": "amy", "name": "twice", "provides": ["twice"]},
			{"author": "carol", "name": "extras", "provides": ["twice", "both"]},
			{"author": "dan", "name": "more", "provides": ["both"]},
			{"author": "bob", "name": "dupe", "provides": ["dupe"]},
			{"author": "gone", "name": "stale", "provides": ["stale"]},
			{"author": "gm", "name": "somegame", "provides": ["mesecons_mvps", "fromgame"]}
		]}`,
		"jeija/mesecons": `{"jeija/mesecons": []}`,
		"amy/twice":      `{"amy/twice": []}`,
		"bob/dupe":       `{"bob/dupe": []}`,
	})

	packages := index(
		mod("jeija", "mesecons", 1), mod("amy", "twice", 2), mod("carol", "extras", 3),
		mod("dan", "more", 4), mod("bob", "dupe", 5), mod("sam", "dupe", 6),
		contentdb.Package{Author: "gm", Name: "somegame", Type: "game", Release: 7},
	)
	locals := []LocalPackage{{Type: "mod", Name: "mymod", Mod: content.Mod{
		Name: "mymod", HasInit: true, Depends: []string{"mesecons_mvps", "twice", "both", "dupe", "stale", "nothing", "fromgame"},
	}}}

	plan, err := ResolveInstallDeps(client, nil, locals, map[string]bool{}, packages)
	if err != nil {
		t.Fatalf("ResolveInstallDeps: %v", err)
	}

	got := installIDs(plan)
	slices.Sort(got)
	// one provider, the one named like the dep, and the one of two same-named packages that provides it
	if want := []string{"amy/twice", "bob/dupe", "jeija/mesecons"}; !reflect.DeepEqual(got, want) {
		t.Errorf("install = %v, want %v", got, want)
	}

	missing := make(map[string]MissingDep)
	for _, m := range plan.NotFound {
		missing[m.Dep] = MissingDep{Ambiguous: m.Ambiguous, GameOnly: m.GameOnly}
	}
	// stale's provider isn't in the index, and a game providing mesecons_mvps doesn't make it ambiguous
	want := map[string]MissingDep{"both": {Ambiguous: true}, "stale": {}, "nothing": {}, "fromgame": {GameOnly: true}}
	if !reflect.DeepEqual(missing, want) {
		t.Errorf("not found = %+v, want %+v", missing, want)
	}

	// still missing deps don't refetch a copy under 10 minutes old
	if n := countOf(*requested, "providers"); n != 1 {
		t.Errorf("providers fetched %d times, want 1", n)
	}

	ageCache(t, time.Hour)

	if _, err := ResolveInstallDeps(client, nil, locals, map[string]bool{}, packages); err != nil {
		t.Fatalf("second ResolveInstallDeps: %v", err)
	}
	if n := countOf(*requested, "providers"); n != 2 {
		t.Errorf("providers fetched %d times in all, want 2 with one refetch for the missing deps", n)
	}
}

func TestResolveInstallDepsNoProviderLookup(t *testing.T) {
	client, requested := newDepsServer(t, map[string]string{"amy/lib": `{"amy/lib": []}`})
	packages := index(mod("amy", "lib", 1))
	locals := []LocalPackage{{Type: "mod", Name: "mymod", Mod: content.Mod{
		Name: "mymod", HasInit: true, Depends: []string{"lib", "have"}, OptionalDepends: []string{"nothing"},
	}}}

	if _, err := ResolveInstallDeps(client, nil, locals, map[string]bool{"have": true}, packages); err != nil {
		t.Fatalf("ResolveInstallDeps: %v", err)
	}
	if slices.Contains(*requested, "providers") {
		t.Errorf("requested = %v, want no provider lookup when only optional deps are missing", *requested)
	}
}

// ageCache makes every file in the test's cache folder age old.
func ageCache(t *testing.T, age time.Duration) {
	t.Helper()
	cacheDir := os.Getenv("LUMA_CACHE_DIR")
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-age)
	for _, e := range entries {
		if err := os.Chtimes(filepath.Join(cacheDir, e.Name()), old, old); err != nil {
			t.Fatal(err)
		}
	}
}

func countOf(s []string, v string) int {
	n := 0
	for _, x := range s {
		if x == v {
			n++
		}
	}
	return n
}
