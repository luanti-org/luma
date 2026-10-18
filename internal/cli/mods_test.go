package cli

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/luanti-org/luma/internal/content"
	"github.com/luanti-org/luma/internal/contentdb"
	"github.com/luanti-org/luma/internal/engine"
)

// fakeCDB serves /api/updates/, the package list, dependencies and release zips,
// recording which mods were downloaded.
type fakeCDB struct {
	updatesJSON  string
	updatesFail  bool
	failFor      map[string]bool
	packagesJSON string            // empty serves no packages
	depsJSON     map[string]string // by "author/name", missing serves no dependencies
	details      map[string]bool   // "author/name" packages whose details are served, others 404

	mu           sync.Mutex
	downloads    []string
	listRequests int
}

func (f *fakeCDB) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/updates/" {
		if f.updatesFail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write([]byte(f.updatesJSON))
		return
	}

	if r.URL.Path == "/api/packages/" {
		f.mu.Lock()
		f.listRequests++
		f.mu.Unlock()

		if f.packagesJSON == "" {
			w.Write([]byte("[]"))
			return
		}
		w.Write([]byte(f.packagesJSON))
		return
	}

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")

	// /api/packages/<author>/<name>/dependencies/
	if len(parts) == 5 && parts[0] == "api" && parts[4] == "dependencies" {
		if body, ok := f.depsJSON[parts[2]+"/"+parts[3]]; ok {
			w.Write([]byte(body))
			return
		}
		w.Write([]byte("{}"))
		return
	}

	// /api/packages/<author>/<name>/
	if len(parts) == 4 && parts[0] == "api" {
		if !f.details[parts[2]+"/"+parts[3]] {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte(`{"author": "` + parts[2] + `", "name": "` + parts[3] + `"}`))
		return
	}

	// /packages/<author>/<name>/releases/<id>/download/
	if len(parts) != 6 || parts[0] != "packages" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	name := parts[2]

	f.mu.Lock()
	f.downloads = append(f.downloads, name)
	f.mu.Unlock()

	if f.failFor[name] {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Write(buildModZip(name))
}

func (f *fakeCDB) downloaded() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.downloads, ",")
}

func buildModZip(name string) []byte {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for file, data := range map[string]string{"mod.conf": "name = " + name + "\n", "init.lua": "-- hi\n"} {
		fw, _ := w.Create("jane-" + name + "-abc/" + file)
		fw.Write([]byte(data))
	}
	w.Close()
	return buf.Bytes()
}

// setupMods installs alpha and beta at release 1 and gamma at 5; the fake CDB has alpha 2, beta 3, gamma 5.
func setupMods(t *testing.T) (engine.Info, *fakeCDB, *contentdb.Client) {
	t.Helper()

	dir := t.TempDir()
	for name, release := range map[string]string{"alpha": "1", "beta": "1", "gamma": "5"} {
		writeFile(t, filepath.Join(dir, name, "mod.conf"), "name = "+name+"\nauthor = jane\nrelease = "+release+"\n")
	}

	f := &fakeCDB{updatesJSON: `{"jane/alpha": 2, "jane/beta": 3, "jane/gamma": 5}`}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)

	return engine.Info{ModsDir: dir}, f, contentdb.New(srv.URL)
}

func installedReleases(t *testing.T, dir string) map[string]int {
	t.Helper()

	mods, err := content.ScanMods(dir)
	if err != nil {
		t.Fatal(err)
	}

	out := make(map[string]int, len(mods))
	for _, m := range mods {
		out[m.Name] = m.Release
	}
	return out
}

func TestModsListNoInstall(t *testing.T) {
	code, _, stderr := run(t, engine.Info{}, "mods", "list")
	if code != exitError {
		t.Errorf("code = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "pass --dir") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestModsListMissingDir(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "mods")

	code, stdout, stderr := run(t, engine.Info{ModsDir: missing, ViaFlatpak: true}, "mods", "list", "--names")
	if code != exitOK || stdout != "" {
		t.Errorf("flatpak: code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}

	code, _, stderr = run(t, engine.Info{ModsDir: missing}, "mods", "list")
	if code != exitError || !strings.Contains(stderr, "scanning mods") {
		t.Errorf("dir: code = %d, stderr = %q", code, stderr)
	}
}

func TestModsList(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "alpha", "mod.conf"), "name = alpha\nauthor = someone\nrelease = 42\n")
	writeFile(t, filepath.Join(dir, "beta", "mod.conf"), "name = beta\n")

	code, stdout, stderr := run(t, engine.Info{ModsDir: dir}, "mods", "list")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3:\n%s", len(lines), stdout)
	}

	wantFields := [][]string{
		{"NAME", "AUTHOR", "RELEASE"},
		{"alpha", "someone", "42"},
		{"beta", "-", "-"},
	}
	for i, want := range wantFields {
		got := strings.Fields(lines[i])
		if strings.Join(got, " ") != strings.Join(want, " ") {
			t.Errorf("line %d = %q, want fields %q", i, lines[i], want)
		}
	}
}

func TestModsListModpackTag(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pack", "modpack.conf"), "name = pack\n")
	writeFile(t, filepath.Join(dir, "solo", "mod.conf"), "name = solo\n")

	code, stdout, stderr := run(t, engine.Info{ModsDir: dir}, "mods", "list")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "pack [P]") || strings.Contains(stdout, "solo [P]") {
		t.Errorf("stdout = %q", stdout)
	}

	_, stdout, _ = run(t, engine.Info{ModsDir: dir}, "mods", "list", "--names")
	if stdout != "pack\nsolo\n" {
		t.Errorf("--names stdout = %q", stdout)
	}
}

func TestModsListNames(t *testing.T) {
	eng, _, _ := setupMods(t)

	code, stdout, stderr := run(t, eng, "mods", "list", "--names")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if stdout != "alpha\nbeta\ngamma\n" {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestModsOutdated(t *testing.T) {
	eng, _, cdb := setupMods(t)

	code, stdout, stderr := runWithCDB(t, eng, cdb, "mods", "outdated")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	want := []string{"NAME CURRENT LATEST", "alpha 1 2", "beta 1 3"}
	if len(lines) != len(want) {
		t.Fatalf("stdout =\n%s", stdout)
	}
	for i := range want {
		if got := strings.Join(strings.Fields(lines[i]), " "); got != want[i] {
			t.Errorf("line %d = %q, want %q", i, got, want[i])
		}
	}
}

func TestModsOutdatedNames(t *testing.T) {
	eng, _, cdb := setupMods(t)

	code, stdout, _ := runWithCDB(t, eng, cdb, "mods", "outdated", "--names")
	if code != exitOK || stdout != "alpha\nbeta\n" {
		t.Errorf("code = %d, stdout = %q", code, stdout)
	}
}

func TestModsOutdatedAllUpToDate(t *testing.T) {
	eng, f, cdb := setupMods(t)
	f.updatesJSON = `{"jane/gamma": 5}`

	code, stdout, _ := runWithCDB(t, eng, cdb, "mods", "outdated")
	if code != exitOK || !strings.Contains(stdout, "up to date") {
		t.Errorf("code = %d, stdout = %q", code, stdout)
	}
}

func TestModsOutdatedFetchError(t *testing.T) {
	eng, f, cdb := setupMods(t)
	f.updatesFail = true

	code, _, stderr := runWithCDB(t, eng, cdb, "mods", "outdated")
	if code != exitError || !strings.Contains(stderr, "checking for updates") {
		t.Errorf("code = %d, stderr = %q", code, stderr)
	}
}

func TestModsUpdateAll(t *testing.T) {
	eng, f, cdb := setupMods(t)

	code, stdout, stderr := runWithCDB(t, eng, cdb, "mods", "update")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if got := f.downloaded(); got != "alpha,beta" {
		t.Errorf("downloaded = %q, want alpha,beta", got)
	}
	if !strings.Contains(stdout, "alpha: updated 1 -> 2") {
		t.Errorf("stdout = %q", stdout)
	}

	rel := installedReleases(t, eng.ModsDir)
	if rel["alpha"] != 2 || rel["beta"] != 3 || rel["gamma"] != 5 {
		t.Errorf("releases after update = %v", rel)
	}
}

func TestModsUpdateNamed(t *testing.T) {
	eng, f, cdb := setupMods(t)

	code, stdout, stderr := runWithCDB(t, eng, cdb, "mods", "update", "beta", "gamma")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if got := f.downloaded(); got != "beta" {
		t.Errorf("downloaded = %q, want beta", got)
	}
	if !strings.Contains(stdout, "gamma: no update available") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestModsUpdateNamedNoAuthor(t *testing.T) {
	eng, f, cdb := setupMods(t)
	writeFile(t, filepath.Join(eng.ModsDir, "local", "mod.conf"), "name = local\n")
	writeFile(t, filepath.Join(eng.ModsDir, "pack", "modpack.conf"), "name = pack\n")

	code, stdout, stderr := runWithCDB(t, eng, cdb, "mods", "update", "local", "pack")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "local: can't check for updates, no author in mod.conf") ||
		!strings.Contains(stdout, "pack: can't check for updates, no author in modpack.conf") {
		t.Errorf("stdout = %q", stdout)
	}
	if got := f.downloaded(); got != "" {
		t.Errorf("downloaded = %q, want nothing", got)
	}
}

func TestModsUpdateSkipsGitCheckoutWithoutRelease(t *testing.T) {
	eng, f, cdb := setupMods(t)
	writeFile(t, filepath.Join(eng.ModsDir, "alpha", "mod.conf"), "name = alpha\nauthor = jane\n")
	writeFile(t, filepath.Join(eng.ModsDir, "alpha", ".git", "HEAD"), "ref: refs/heads/main\n")
	// has a release, so it came from a ContentDB zip that shipped a .git
	writeFile(t, filepath.Join(eng.ModsDir, "beta", ".git", "HEAD"), "ref: refs/heads/main\n")

	code, stdout, stderr := runWithCDB(t, eng, cdb, "mods", "outdated", "--names")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if strings.Contains(stdout, "alpha") || !strings.Contains(stdout, "beta") {
		t.Errorf("outdated = %q, want beta and not alpha", stdout)
	}

	code, stdout, stderr = runWithCDB(t, eng, cdb, "mods", "update", "alpha", "beta")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "alpha: git checkout with no release, skipped") {
		t.Errorf("stdout = %q", stdout)
	}
	if got := f.downloaded(); got != "beta" {
		t.Errorf("downloaded = %q, want beta", got)
	}
	if _, err := os.Stat(filepath.Join(eng.ModsDir, "alpha", ".git", "HEAD")); err != nil {
		t.Error("the git checkout was replaced")
	}
}

func TestModsUpdateExclude(t *testing.T) {
	eng, f, cdb := setupMods(t)

	code, _, stderr := runWithCDB(t, eng, cdb, "mods", "update", "--exclude", "alpha")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if got := f.downloaded(); got != "beta" {
		t.Errorf("downloaded = %q, want beta", got)
	}
}

func TestModsUpdateNamedAndExcluded(t *testing.T) {
	eng, f, cdb := setupMods(t)

	code, stdout, stderr := runWithCDB(t, eng, cdb, "mods", "update", "alpha", "beta", "-x", "alpha")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if got := f.downloaded(); got != "beta" {
		t.Errorf("downloaded = %q, want beta", got)
	}
	if strings.Contains(stdout, "alpha") {
		t.Errorf("stdout = %q, want no mention of alpha", stdout)
	}
	if rel := installedReleases(t, eng.ModsDir); rel["alpha"] != 1 || rel["beta"] != 3 {
		t.Errorf("releases after update = %v", rel)
	}
}

func TestModsUpdateUnknownNames(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"positional", []string{"alpha", "nope"}},
		{"exclude", []string{"--exclude", "alpha,nope"}},
		{"exclude without deps", []string{"--no-deps", "--exclude", "alpha,nope"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng, f, cdb := setupMods(t)

			code, _, stderr := runWithCDB(t, eng, cdb, append([]string{"mods", "update"}, tt.args...)...)
			if code != exitError || !strings.Contains(stderr, "not installed") || !strings.Contains(stderr, ": nope") {
				t.Errorf("code = %d, stderr = %q", code, stderr)
			}
			if got := f.downloaded(); got != "" {
				t.Errorf("downloaded = %q, want nothing", got)
			}
		})
	}
}

func TestModsUpdateDryRun(t *testing.T) {
	eng, f, cdb := setupMods(t)

	code, stdout, _ := runWithCDB(t, eng, cdb, "mods", "update", "--dry-run")
	if code != exitOK || !strings.Contains(stdout, "Would update:") || !strings.Contains(stdout, "alpha") {
		t.Errorf("code = %d, stdout = %q", code, stdout)
	}
	if got := f.downloaded(); got != "" {
		t.Errorf("downloaded = %q, want nothing", got)
	}
}

func TestModsUpdateNothingToDo(t *testing.T) {
	eng, f, cdb := setupMods(t)
	f.updatesJSON = `{"jane/gamma": 5}`

	code, stdout, _ := runWithCDB(t, eng, cdb, "mods", "update")
	if code != exitOK || !strings.Contains(stdout, "Nothing to update.") {
		t.Errorf("code = %d, stdout = %q", code, stdout)
	}
}

func TestModsUpdateContinuesAfterFailure(t *testing.T) {
	eng, f, cdb := setupMods(t)
	f.failFor = map[string]bool{"alpha": true}

	code, _, stderr := runWithCDB(t, eng, cdb, "mods", "update")
	if code != exitError {
		t.Errorf("code = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "alpha: update failed") || !strings.Contains(stderr, "1 of 2 updates failed") {
		t.Errorf("stderr = %q", stderr)
	}
	if rel := installedReleases(t, eng.ModsDir); rel["beta"] != 3 {
		t.Errorf("beta release = %d, want 3", rel["beta"])
	}
}

func TestModsUpdateFlagAfterName(t *testing.T) {
	eng, f, cdb := setupMods(t)

	code, stdout, stderr := runWithCDB(t, eng, cdb, "mods", "update", "alpha", "-n")
	if code != exitOK || !strings.Contains(stdout, "Would update:") {
		t.Errorf("code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
	if got := f.downloaded(); got != "" {
		t.Errorf("downloaded = %q, want nothing", got)
	}
}

func TestModsUpdateCombinedShortFlags(t *testing.T) {
	eng, f, cdb := setupMods(t)

	code, stdout, stderr := runWithCDB(t, eng, cdb, "mods", "update", "-nx", "alpha")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "beta") || strings.Contains(stdout, "alpha") {
		t.Errorf("stdout = %q, want only beta planned", stdout)
	}
	if got := f.downloaded(); got != "" {
		t.Errorf("downloaded = %q, want nothing", got)
	}
}

// setupDeps is setupMods where alpha's new release needs "lib", which amy/lib provides.
func setupDeps(t *testing.T) (engine.Info, *fakeCDB, *contentdb.Client) {
	t.Helper()

	eng, f, cdb := setupMods(t)
	f.packagesJSON = `[{"author": "amy", "name": "lib", "type": "mod", "release": 9}]`
	f.depsJSON = map[string]string{
		"jane/alpha": `{"jane/alpha": [{"name": "lib", "is_optional": false, "packages": ["amy/lib"]}]}`,
	}

	return eng, f, cdb
}

func TestModsUpdateInstallsNewDependency(t *testing.T) {
	eng, f, cdb := setupDeps(t)

	code, stdout, stderr := runWithCDB(t, eng, cdb, "mods", "update", "-y")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if got := f.downloaded(); got != "lib,alpha,beta" {
		t.Errorf("downloaded = %q, want lib,alpha,beta", got)
	}
	if !strings.Contains(stdout, "amy/lib: installed 9 (dependency of jane/alpha)") {
		t.Errorf("stdout = %q", stdout)
	}

	conf, err := os.ReadFile(filepath.Join(eng.ModsDir, "lib", "mod.conf"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"name = lib", "author = amy", "release = 9"} {
		if !strings.Contains(string(conf), want) {
			t.Errorf("lib mod.conf = %q, want %q in it", conf, want)
		}
	}
}

func TestModsUpdateDryRunShowsDependencies(t *testing.T) {
	eng, f, cdb := setupDeps(t)

	code, stdout, stderr := runWithCDB(t, eng, cdb, "mods", "update", "-n")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "Would install as dependencies:") || !strings.Contains(stdout, "amy/lib") {
		t.Errorf("stdout = %q", stdout)
	}
	if got := f.downloaded(); got != "" {
		t.Errorf("downloaded = %q, want nothing", got)
	}
}

func TestModsUpdateNoDeps(t *testing.T) {
	eng, f, cdb := setupDeps(t)

	code, _, stderr := runWithCDB(t, eng, cdb, "mods", "update", "--no-deps")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if got := f.downloaded(); got != "alpha,beta" {
		t.Errorf("downloaded = %q, want alpha,beta", got)
	}
	if f.listRequests != 0 {
		t.Errorf("package list requested %d times, want 0", f.listRequests)
	}
}

func TestModsUpdateDependencyAlreadyInstalled(t *testing.T) {
	tests := []struct {
		name string
		path []string // under the mods dir
	}{
		{"as a mod", []string{"lib", "init.lua"}},
		{"in a modpack", []string{"pack", "lib", "init.lua"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng, f, cdb := setupDeps(t)
			writeFile(t, filepath.Join(eng.ModsDir, "pack", "modpack.conf"), "name = pack\n")
			writeFile(t, filepath.Join(append([]string{eng.ModsDir}, tt.path...)...), "-- hi\n")

			code, _, stderr := runWithCDB(t, eng, cdb, "mods", "update")
			if code != exitOK {
				t.Fatalf("code = %d, stderr = %q", code, stderr)
			}
			if got := f.downloaded(); got != "alpha,beta" {
				t.Errorf("downloaded = %q, want alpha,beta", got)
			}
		})
	}
}

func TestModsUpdateGameProvidesDependency(t *testing.T) {
	eng, f, cdb := setupDeps(t)
	eng.GamesDir = t.TempDir()
	writeFile(t, filepath.Join(eng.GamesDir, "mygame", "game.conf"), "title = My Game\n")
	writeFile(t, filepath.Join(eng.GamesDir, "mygame", "mods", "lib", "init.lua"), "-- hi\n")

	code, _, stderr := runWithCDB(t, eng, cdb, "mods", "update", "--game", "mygame")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if got := f.downloaded(); got != "alpha,beta" {
		t.Errorf("downloaded = %q, want alpha,beta", got)
	}

	code, _, stderr = runWithCDB(t, eng, cdb, "mods", "update", "--game", "nope")
	if code != exitError || !strings.Contains(stderr, "game not installed: nope") {
		t.Errorf("code = %d, stderr = %q", code, stderr)
	}
}

func TestModsUpdateExcludeDependency(t *testing.T) {
	eng, f, cdb := setupDeps(t)

	code, _, stderr := runWithCDB(t, eng, cdb, "mods", "update", "-x", "lib")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stderr, "skipping excluded dependency lib for jane/alpha") {
		t.Errorf("stderr = %q", stderr)
	}
	if got := f.downloaded(); got != "alpha,beta" {
		t.Errorf("downloaded = %q, want alpha,beta", got)
	}
}

func TestModsUpdateDependencyNotFound(t *testing.T) {
	eng, f, cdb := setupDeps(t)
	f.packagesJSON = ""

	code, _, stderr := runWithCDB(t, eng, cdb, "mods", "update")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stderr, "could not find dependency lib for jane/alpha") {
		t.Errorf("stderr = %q", stderr)
	}
	if got := f.downloaded(); got != "alpha,beta" {
		t.Errorf("downloaded = %q, want alpha,beta", got)
	}
}

func TestModsUpdateDependencyFolderExists(t *testing.T) {
	eng, f, cdb := setupDeps(t)
	// not a loadable mod, so it doesn't satisfy the dependency
	writeFile(t, filepath.Join(eng.ModsDir, "lib", "notes.txt"), "mine")

	code, _, stderr := runWithCDB(t, eng, cdb, "mods", "update", "-y")
	if code != exitError {
		t.Errorf("code = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "amy/lib: install failed") || !strings.Contains(stderr, "1 of 1 dependency installs failed") {
		t.Errorf("stderr = %q", stderr)
	}
	if got := f.downloaded(); got != "alpha,beta" {
		t.Errorf("downloaded = %q, want alpha,beta", got)
	}
	if data, err := os.ReadFile(filepath.Join(eng.ModsDir, "lib", "notes.txt")); err != nil || string(data) != "mine" {
		t.Errorf("existing folder was changed: %q, %v", data, err)
	}
}

func TestModsUpdateDependencyInvalidName(t *testing.T) {
	eng, f, cdb := setupDeps(t)
	f.packagesJSON = `[{"author": "amy", "name": "lib.d", "type": "mod", "release": 9}]`
	f.depsJSON = map[string]string{
		"jane/alpha": `{"jane/alpha": [{"name": "lib", "is_optional": false, "packages": ["amy/lib.d"]}]}`,
	}

	code, _, stderr := runWithCDB(t, eng, cdb, "mods", "update", "-y")
	if code != exitError || !strings.Contains(stderr, "invalid package name") {
		t.Errorf("code = %d, stderr = %q", code, stderr)
	}
	if got := f.downloaded(); got != "alpha,beta" {
		t.Errorf("downloaded = %q, want alpha,beta", got)
	}
	if _, err := os.Stat(filepath.Join(eng.ModsDir, "lib.d")); err == nil {
		t.Error("a folder was created for the invalid name")
	}
}

func TestModsUpdateDependencyInstallFails(t *testing.T) {
	eng, f, cdb := setupDeps(t)
	f.failFor = map[string]bool{"lib": true}

	code, _, stderr := runWithCDB(t, eng, cdb, "mods", "update", "-y")
	if code != exitError || !strings.Contains(stderr, "1 of 1 dependency installs failed") {
		t.Errorf("code = %d, stderr = %q", code, stderr)
	}
	if rel := installedReleases(t, eng.ModsDir); rel["alpha"] != 2 || rel["beta"] != 3 {
		t.Errorf("releases after update = %v, want the updates still applied", rel)
	}
}

// runAnswering runs args as if stdin were a terminal on which the user typed input.
func runAnswering(t *testing.T, eng engine.Info, cdb *contentdb.Client, input string, args ...string) (code int, stdout, stderr string) {
	t.Helper()

	var out, errOut bytes.Buffer
	c := &ctx{eng: eng, cdb: cdb, stdout: &out, stderr: &errOut, stdin: strings.NewReader(input), interactive: true}
	code = c.run(args)

	return code, out.String(), errOut.String()
}

func TestModsUpdateConfirmsNewDependencies(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"y\n", true},
		{"YES\n", true},
		{"n\n", false},
		{"\n", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			eng, f, cdb := setupDeps(t)

			code, stdout, stderr := runAnswering(t, eng, cdb, tt.input, "mods", "update")
			if !strings.Contains(stderr, "Will also install as dependencies:") || !strings.Contains(stderr, "amy/lib") ||
				!strings.Contains(stderr, "Continue? [y/N]") {
				t.Errorf("stderr = %q, want the plan and a prompt", stderr)
			}

			if tt.want {
				if code != exitOK || f.downloaded() != "lib,alpha,beta" {
					t.Errorf("code = %d, downloaded = %q, stderr = %q", code, f.downloaded(), stderr)
				}
				return
			}

			if code != exitError || !strings.Contains(stderr, "aborted") || stdout != "" {
				t.Errorf("code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
			}
			if got := f.downloaded(); got != "" {
				t.Errorf("downloaded = %q, want nothing", got)
			}
		})
	}
}

func TestModsUpdateNoPromptWithoutNewDependencies(t *testing.T) {
	eng, f, cdb := setupMods(t)

	code, _, stderr := runAnswering(t, eng, cdb, "", "mods", "update")
	if code != exitOK || strings.Contains(stderr, "Continue?") {
		t.Errorf("code = %d, stderr = %q", code, stderr)
	}
	if got := f.downloaded(); got != "alpha,beta" {
		t.Errorf("downloaded = %q, want alpha,beta", got)
	}
}

func TestModsUpdateNewDependenciesWithoutTerminal(t *testing.T) {
	eng, f, cdb := setupDeps(t)

	code, _, stderr := runWithCDB(t, eng, cdb, "mods", "update")
	if code != exitError || !strings.Contains(stderr, "new dependencies required (amy/lib), rerun with -y or --no-deps") {
		t.Errorf("code = %d, stderr = %q", code, stderr)
	}
	if got := f.downloaded(); got != "" {
		t.Errorf("downloaded = %q, want nothing", got)
	}
}

func TestModsUpdateDryRunNeverPrompts(t *testing.T) {
	eng, _, cdb := setupDeps(t)

	code, _, stderr := runAnswering(t, eng, cdb, "", "mods", "update", "-n")
	if code != exitOK || strings.Contains(stderr, "Continue?") {
		t.Errorf("code = %d, stderr = %q", code, stderr)
	}
}

// setupModDeps is setupMods where alpha needs lib (amy/lib) and base (only a game provides it), and optionally
// depends on extra (amy/extra, which needs amy/lib), have (installed), gameonly (only a game) and nowhere.
// local has no author and orphan isn't on ContentDB, both need have and gone.
func setupModDeps(t *testing.T) (engine.Info, *fakeCDB, *contentdb.Client) {
	t.Helper()

	eng, f, cdb := setupMods(t)
	writeFile(t, filepath.Join(eng.ModsDir, "have", "init.lua"), "-- hi\n")
	writeFile(t, filepath.Join(eng.ModsDir, "local", "mod.conf"), "name = local\ndepends = have, gone\noptional_depends = opt\n")
	writeFile(t, filepath.Join(eng.ModsDir, "orphan", "mod.conf"), "name = orphan\nauthor = jane\ndepends = have, gone\n")
	f.packagesJSON = `[
		{"author": "jane", "name": "alpha", "type": "mod", "release": 2},
		{"author": "jane", "name": "beta", "type": "mod", "release": 3},
		{"author": "amy", "name": "extra", "type": "mod", "release": 7},
		{"author": "amy", "name": "lib", "type": "mod", "release": 6},
		{"author": "amy", "name": "have", "type": "mod", "release": 8},
		{"author": "gm", "name": "somegame", "type": "game", "release": 9}
	]`
	f.depsJSON = map[string]string{
		"jane/alpha": `{"jane/alpha": [
			{"name": "lib", "is_optional": false, "packages": ["amy/lib"]},
			{"name": "base", "is_optional": false, "packages": ["gm/somegame"]},
			{"name": "extra", "is_optional": true, "packages": ["amy/extra"]},
			{"name": "have", "is_optional": true, "packages": ["amy/have"]},
			{"name": "gameonly", "is_optional": true, "packages": ["gm/somegame"]},
			{"name": "nowhere", "is_optional": true, "packages": []}
		]}`,
		"amy/extra": `{"amy/extra": [{"name": "lib", "is_optional": false, "packages": ["amy/lib"]}]}`,
	}

	return eng, f, cdb
}

// setupGame adds the game mygame to eng, providing the mods named.
func setupGame(t *testing.T, eng *engine.Info, mods ...string) {
	t.Helper()

	eng.GamesDir = t.TempDir()
	writeFile(t, filepath.Join(eng.GamesDir, "mygame", "game.conf"), "title = My Game\n")
	for _, m := range mods {
		writeFile(t, filepath.Join(eng.GamesDir, "mygame", "mods", m, "init.lua"), "-- hi\n")
	}
}

func checkTable(t *testing.T, stdout string, want [][]string) {
	t.Helper()

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != len(want) {
		t.Fatalf("stdout = %q", stdout)
	}
	for i, w := range want {
		if got := strings.Fields(lines[i]); strings.Join(got, " ") != strings.Join(w, " ") {
			t.Errorf("line %d = %q, want %q", i, lines[i], strings.Join(w, " "))
		}
	}
}

func TestModsListDeps(t *testing.T) {
	eng, _, cdb := setupModDeps(t)

	code, stdout, stderr := runWithCDB(t, eng, cdb, "mods", "list-deps", "alpha")
	if code != exitOK || stderr != "" {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	checkTable(t, stdout, [][]string{
		{"DEP", "STATUS", "PACKAGE", "RELEASE"},
		{"lib", "available", "amy/lib", "6"},
		{"base", "from", "game", "-", "-"},
	})
}

func TestModsListDepsOptional(t *testing.T) {
	eng, _, cdb := setupModDeps(t)

	code, stdout, stderr := runWithCDB(t, eng, cdb, "mods", "list-deps", "--optional", "alpha")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	checkTable(t, stdout, [][]string{
		{"DEP", "STATUS", "PACKAGE", "RELEASE"},
		{"extra", "available", "amy/extra", "7"},
		{"have", "installed", "-", "-"},
		{"gameonly", "from", "game", "-", "-"},
		{"nowhere", "not", "found", "-", "-"},
	})
}

func TestModsListDepsNames(t *testing.T) {
	eng, _, cdb := setupModDeps(t)

	code, stdout, stderr := runWithCDB(t, eng, cdb, "mods", "list-deps", "--names", "alpha")
	if code != exitOK || stdout != "lib\nbase\n" {
		t.Errorf("code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}

	code, stdout, stderr = runWithCDB(t, eng, cdb, "mods", "list-deps", "--names", "--optional", "alpha")
	if code != exitOK || stdout != "extra\nhave\ngameonly\nnowhere\n" {
		t.Errorf("code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
}

func TestModsListDepsGameProvides(t *testing.T) {
	eng, _, cdb := setupModDeps(t)
	setupGame(t, &eng, "base")

	code, stdout, stderr := runWithCDB(t, eng, cdb, "mods", "list-deps", "--game", "mygame", "alpha")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	checkTable(t, stdout, [][]string{
		{"DEP", "STATUS", "PACKAGE", "RELEASE"},
		{"lib", "available", "amy/lib", "6"},
		{"base", "installed", "-", "-"},
	})
}

func TestModsListDepsNone(t *testing.T) {
	eng, _, cdb := setupModDeps(t)

	code, stdout, stderr := runWithCDB(t, eng, cdb, "mods", "list-deps", "beta")
	if code != exitOK || stdout != "beta has no required dependencies.\n" {
		t.Errorf("code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}

	code, stdout, stderr = runWithCDB(t, eng, cdb, "mods", "list-deps", "--optional", "beta")
	if code != exitOK || stdout != "beta has no optional dependencies.\n" {
		t.Errorf("code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
}

func TestModsListDepsLocal(t *testing.T) {
	tests := []struct {
		mod, reason string
		requests    int
	}{
		{"local", "no author in mod.conf", 0},
		{"orphan", "no compatible package on ContentDB", 1},
	}

	for _, tt := range tests {
		t.Run(tt.mod, func(t *testing.T) {
			eng, f, cdb := setupModDeps(t)

			code, stdout, stderr := runWithCDB(t, eng, cdb, "mods", "list-deps", tt.mod)
			if code != exitOK || stderr != "luma: "+tt.mod+": "+tt.reason+", listed from mod.conf\n" {
				t.Fatalf("code = %d, stderr = %q", code, stderr)
			}
			checkTable(t, stdout, [][]string{
				{"DEP", "STATUS", "PACKAGE", "RELEASE"},
				{"have", "installed", "-", "-"},
				{"gone", "missing", "-", "-"},
			})
			if f.listRequests != tt.requests {
				t.Errorf("package list requested %d times, want %d", f.listRequests, tt.requests)
			}
		})
	}
}

func TestModsListDepsErrors(t *testing.T) {
	eng, f, cdb := setupModDeps(t)

	tests := []struct {
		args     []string
		code     int
		inStderr string
	}{
		{nil, exitUsage, "expected one mod name"},
		{[]string{"alpha", "beta"}, exitUsage, "expected one mod name"},
		{[]string{"nope"}, exitError, "not installed: nope"},
		{[]string{"--game", "nope", "alpha"}, exitError, "game not installed: nope"},
	}

	for _, tt := range tests {
		args := append([]string{"mods", "list-deps"}, tt.args...)
		code, _, stderr := runWithCDB(t, eng, cdb, args...)
		if code != tt.code || !strings.Contains(stderr, tt.inStderr) {
			t.Errorf("%v: code = %d, stderr = %q, want %d and %q", tt.args, code, stderr, tt.code, tt.inStderr)
		}
	}

	if f.listRequests != 0 {
		t.Errorf("package list requested %d times, want 0", f.listRequests)
	}
}

func TestModsInstallDeps(t *testing.T) {
	eng, f, cdb := setupModDeps(t)

	code, stdout, stderr := runWithCDB(t, eng, cdb, "mods", "install-deps", "-y", "alpha")
	if code != exitError {
		t.Errorf("code = %d, want %d", code, exitError)
	}
	for _, want := range []string{
		"luma: could not find dependency base for jane/alpha, only games provide it, see --game\n",
		"luma: required dependencies still missing: base\n",
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr = %q, want %q in it", stderr, want)
		}
	}
	if !strings.Contains(stdout, "amy/lib: installed 6 (dependency of jane/alpha)") {
		t.Errorf("stdout = %q", stdout)
	}
	if got := f.downloaded(); got != "lib" {
		t.Errorf("downloaded = %q, want lib", got)
	}
}

func TestModsInstallDepsGameProvides(t *testing.T) {
	eng, f, cdb := setupModDeps(t)
	setupGame(t, &eng, "base")

	code, _, stderr := runWithCDB(t, eng, cdb, "mods", "install-deps", "-y", "--game", "mygame", "alpha")
	if code != exitOK || stderr != "" {
		t.Errorf("code = %d, stderr = %q", code, stderr)
	}
	if got := f.downloaded(); got != "lib" {
		t.Errorf("downloaded = %q, want lib", got)
	}
}

func TestModsInstallDepsOptional(t *testing.T) {
	eng, f, cdb := setupModDeps(t)

	code, stdout, stderr := runWithCDB(t, eng, cdb, "mods", "install-deps", "-y", "--optional", "alpha")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if got := f.downloaded(); got != "extra,lib" {
		t.Errorf("downloaded = %q, want extra,lib", got)
	}
	for _, want := range []string{"amy/extra: installed 7 (dependency of jane/alpha)", "amy/lib: installed 6 (dependency of amy/extra)"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout = %q, want %q in it", stdout, want)
		}
	}
	if releases := installedReleases(t, eng.ModsDir); releases["extra"] != 7 || releases["lib"] != 6 {
		t.Errorf("installed = %v", releases)
	}
}

func TestModsInstallDepsNothingToDo(t *testing.T) {
	eng, f, cdb := setupModDeps(t)

	for _, args := range [][]string{{"beta"}, {"--optional", "beta"}} {
		args = append([]string{"mods", "install-deps"}, args...)
		code, stdout, stderr := runWithCDB(t, eng, cdb, args...)
		if code != exitOK || stdout != "Nothing to install.\n" {
			t.Errorf("%v: code = %d, stdout = %q, stderr = %q", args, code, stdout, stderr)
		}
	}

	if got := f.downloaded(); got != "" {
		t.Errorf("downloaded = %q, want nothing", got)
	}
}

func TestModsInstallDepsLocal(t *testing.T) {
	eng, f, cdb := setupModDeps(t)

	code, stdout, stderr := runWithCDB(t, eng, cdb, "mods", "install-deps", "-y", "local")
	want := "luma: local: no author in mod.conf, listed from mod.conf\n" +
		"luma: could not find dependency gone for local\n" +
		"luma: required dependencies still missing: gone\n"
	if code != exitError || stdout != "Nothing to install.\n" || stderr != want {
		t.Errorf("code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}

	code, stdout, _ = runWithCDB(t, eng, cdb, "mods", "install-deps", "--optional", "local")
	if code != exitOK || stdout != "Nothing to install.\n" {
		t.Errorf("--optional: code = %d, stdout = %q", code, stdout)
	}

	if got := f.downloaded(); got != "" {
		t.Errorf("downloaded = %q, want nothing", got)
	}
}

func TestModsInstallDepsUsage(t *testing.T) {
	eng, _, cdb := setupModDeps(t)

	for _, args := range [][]string{nil, {"alpha", "extra"}} {
		args = append([]string{"mods", "install-deps"}, args...)
		code, _, stderr := runWithCDB(t, eng, cdb, args...)
		if code != exitUsage || !strings.Contains(stderr, "expected one mod name") {
			t.Errorf("%v: code = %d, stderr = %q", args, code, stderr)
		}
	}
}

func TestModsInstallDepsDryRun(t *testing.T) {
	eng, f, cdb := setupModDeps(t)

	code, stdout, stderr := runAnswering(t, eng, cdb, "", "mods", "install-deps", "-n", "--optional", "alpha")
	if code != exitOK || strings.Contains(stderr, "Continue?") {
		t.Errorf("code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "Would install:") || !strings.Contains(stdout, "amy/extra") || !strings.Contains(stdout, "amy/lib") {
		t.Errorf("stdout = %q", stdout)
	}

	code, _, stderr = runWithCDB(t, eng, cdb, "mods", "install-deps", "-n", "alpha")
	if code != exitOK || !strings.Contains(stderr, "could not find dependency base") {
		t.Errorf("code = %d, stderr = %q, want missing base reported but exit %d", code, stderr, exitOK)
	}

	if got := f.downloaded(); got != "" {
		t.Errorf("downloaded = %q, want nothing", got)
	}
}

func TestModsInstallDepsConfirms(t *testing.T) {
	for _, tt := range []struct {
		input string
		want  bool
	}{{"y\n", true}, {"\n", false}} {
		t.Run(tt.input, func(t *testing.T) {
			eng, f, cdb := setupModDeps(t)

			code, _, stderr := runAnswering(t, eng, cdb, tt.input, "mods", "install-deps", "--optional", "alpha")
			if !strings.Contains(stderr, "Will install:") || !strings.Contains(stderr, "amy/lib") ||
				!strings.Contains(stderr, "Continue? [y/N]") {
				t.Errorf("stderr = %q, want the plan and a prompt", stderr)
			}

			want, wantCode := "", exitError
			if tt.want {
				want, wantCode = "extra,lib", exitOK
			}
			if got := f.downloaded(); code != wantCode || got != want {
				t.Errorf("code = %d, downloaded = %q, want %d and %q", code, got, wantCode, want)
			}
		})
	}
}

func TestModsInstallDepsWithoutTerminal(t *testing.T) {
	eng, f, cdb := setupModDeps(t)

	code, _, stderr := runWithCDB(t, eng, cdb, "mods", "install-deps", "--optional", "alpha")
	if code != exitError || !strings.Contains(stderr, "would install amy/extra, amy/lib, rerun with -y") {
		t.Errorf("code = %d, stderr = %q", code, stderr)
	}
	if got := f.downloaded(); got != "" {
		t.Errorf("downloaded = %q, want nothing", got)
	}
}

func TestModsInstallDepsFailure(t *testing.T) {
	eng, f, cdb := setupModDeps(t)
	f.failFor = map[string]bool{"extra": true}

	code, stdout, stderr := runWithCDB(t, eng, cdb, "mods", "install-deps", "-y", "--optional", "alpha")
	if code != exitError || !strings.Contains(stderr, "1 of 2 installs failed") {
		t.Errorf("code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "amy/lib: installed 6") {
		t.Errorf("stdout = %q, want lib installed despite extra failing", stdout)
	}
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
