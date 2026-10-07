package update

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/luanti-org/luma/internal/content"
	"github.com/luanti-org/luma/internal/contentdb"
	"github.com/luanti-org/luma/internal/engine"
)

// newDepsServer serves each body at /api/packages/<id>/dependencies/ and records the ids requested.
func newDepsServer(t *testing.T, bodies map[string]string) (*contentdb.Client, *[]string) {
	t.Helper()

	var requested []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
