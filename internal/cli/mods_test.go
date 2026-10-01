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

// fakeCDB serves /api/updates/ and release zips, recording which mods were downloaded.
type fakeCDB struct {
	updatesJSON string
	updatesFail bool
	failFor     map[string]bool

	mu        sync.Mutex
	downloads []string
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

	// /packages/<author>/<name>/releases/<id>/download/
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng, f, cdb := setupMods(t)

			code, _, stderr := runWithCDB(t, eng, cdb, append([]string{"mods", "update"}, tt.args...)...)
			if code != exitError || !strings.Contains(stderr, "not installed: nope") {
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

func writeFile(t *testing.T, path, data string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
