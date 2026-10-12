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

// setupTexturepacks installs alpha and beta at release 1 and gamma at 5; the fake CDB has alpha 2, beta 3, gamma 5.
// beta lives in a folder named beta_folder.
func setupTexturepacks(t *testing.T) (engine.Info, *fakeCDB, *contentdb.Client) {
	t.Helper()

	dir := t.TempDir()
	for folder, conf := range map[string]string{
		"alpha":       "name = alpha\nauthor = jane\nrelease = 1\n",
		"beta_folder": "name = beta\nauthor = jane\nrelease = 1\n",
		"gamma":       "name = gamma\nauthor = jane\nrelease = 5\n",
	} {
		writeFile(t, filepath.Join(dir, folder, "texture_pack.conf"), conf)
	}

	f := &fakeCDB{updatesJSON: `{"jane/alpha": 2, "jane/beta": 3, "jane/gamma": 5}`}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)

	return engine.Info{TexturesDir: dir}, f, contentdb.New(srv.URL)
}

func installedTexturepackReleases(t *testing.T, dir string) map[string]int {
	t.Helper()

	texturepacks, err := content.ScanTextures(dir)
	if err != nil {
		t.Fatal(err)
	}

	out := make(map[string]int, len(texturepacks))
	for _, tp := range texturepacks {
		out[tp.Dir] = tp.Release
	}
	return out
}

func TestTexturepacksListNoInstall(t *testing.T) {
	code, _, stderr := run(t, engine.Info{}, "texturepacks", "list")
	if code != exitError || !strings.Contains(stderr, "pass --dir") {
		t.Errorf("code = %d, stderr = %q", code, stderr)
	}
}

func TestTexturepacksListMissingDir(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "textures")

	code, stdout, stderr := run(t, engine.Info{TexturesDir: missing, ViaFlatpak: true}, "texturepacks", "list", "--names")
	if code != exitOK || stdout != "" {
		t.Errorf("flatpak: code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}

	code, _, stderr = run(t, engine.Info{TexturesDir: missing}, "texturepacks", "list")
	if code != exitError || !strings.Contains(stderr, "scanning texture packs") {
		t.Errorf("dir: code = %d, stderr = %q", code, stderr)
	}
}

func TestTexturepacksList(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "alpha", "texture_pack.conf"), "name = alpha\ntitle = Alpha Pack\nauthor = someone\nrelease = 42\n")
	writeFile(t, filepath.Join(dir, "base", "pack", "logo.png"), "")
	writeFile(t, filepath.Join(dir, "beta", "default_dirt.png"), "")

	code, stdout, stderr := run(t, engine.Info{TexturesDir: dir}, "texturepacks", "list")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	want := []string{"NAME AUTHOR RELEASE TITLE", "alpha someone 42 Alpha Pack", "beta - - -"}
	if len(lines) != len(want) {
		t.Fatalf("stdout =\n%s", stdout)
	}
	for i := range want {
		if got := strings.Join(strings.Fields(lines[i]), " "); got != want[i] {
			t.Errorf("line %d = %q, want %q", i, got, want[i])
		}
	}
}

func TestTexturepacksListNames(t *testing.T) {
	eng, _, _ := setupTexturepacks(t)

	code, stdout, _ := run(t, eng, "texturepacks", "list", "--names")
	if code != exitOK || stdout != "alpha\nbeta\ngamma\n" {
		t.Errorf("code = %d, stdout = %q", code, stdout)
	}
}

func TestTexturepacksOutdated(t *testing.T) {
	eng, _, cdb := setupTexturepacks(t)

	code, stdout, stderr := runWithCDB(t, eng, cdb, "texturepacks", "outdated")
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

func TestTexturepacksOutdatedNames(t *testing.T) {
	eng, _, cdb := setupTexturepacks(t)

	code, stdout, _ := runWithCDB(t, eng, cdb, "texturepacks", "outdated", "--names")
	if code != exitOK || stdout != "alpha\nbeta\n" {
		t.Errorf("code = %d, stdout = %q", code, stdout)
	}
}

func TestTexturepacksOutdatedAllUpToDate(t *testing.T) {
	eng, f, cdb := setupTexturepacks(t)
	f.updatesJSON = `{"jane/gamma": 5}`

	code, stdout, _ := runWithCDB(t, eng, cdb, "texturepacks", "outdated")
	if code != exitOK || !strings.Contains(stdout, "up to date") {
		t.Errorf("code = %d, stdout = %q", code, stdout)
	}
}

func TestTexturepacksOutdatedFetchError(t *testing.T) {
	eng, f, cdb := setupTexturepacks(t)
	f.updatesFail = true

	code, _, stderr := runWithCDB(t, eng, cdb, "texturepacks", "outdated")
	if code != exitError || !strings.Contains(stderr, "checking for updates") {
		t.Errorf("code = %d, stderr = %q", code, stderr)
	}
}

func TestTexturepacksUpdateAll(t *testing.T) {
	eng, f, cdb := setupTexturepacks(t)
	stale := filepath.Join(eng.TexturesDir, "beta_folder", "default_stone.png")
	writeFile(t, stale, "")

	code, stdout, stderr := runWithCDB(t, eng, cdb, "texturepacks", "update")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	// the download uses the name from texture_pack.conf, not the folder name
	if got := f.downloaded(); got != "alpha,beta" {
		t.Errorf("downloaded = %q, want alpha,beta", got)
	}
	if !strings.Contains(stdout, "alpha: updated 1 -> 2") || !strings.Contains(stdout, "beta: updated 1 -> 3") {
		t.Errorf("stdout = %q", stdout)
	}

	rel := installedTexturepackReleases(t, eng.TexturesDir)
	if len(rel) != 3 || rel["alpha"] != 2 || rel["beta_folder"] != 3 || rel["gamma"] != 5 {
		t.Errorf("releases after update = %v", rel)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale file from the old release still present, err = %v", err)
	}
}

func TestTexturepacksUpdateNamed(t *testing.T) {
	eng, f, cdb := setupTexturepacks(t)

	code, stdout, stderr := runWithCDB(t, eng, cdb, "texturepacks", "update", "beta", "gamma")
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

func TestTexturepacksUpdateNamedUnversioned(t *testing.T) {
	eng, f, cdb := setupTexturepacks(t)
	writeFile(t, filepath.Join(eng.TexturesDir, "alpha", "texture_pack.conf"), "name = alpha\nauthor = jane\n")

	code, stdout, stderr := runWithCDB(t, eng, cdb, "texturepacks", "update", "alpha")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "alpha: can't check for updates, no author or release in texture_pack.conf") {
		t.Errorf("stdout = %q", stdout)
	}
	if got := f.downloaded(); got != "" {
		t.Errorf("downloaded = %q, want nothing", got)
	}
}

func TestTexturepacksUpdateExclude(t *testing.T) {
	eng, f, cdb := setupTexturepacks(t)

	code, _, stderr := runWithCDB(t, eng, cdb, "texturepacks", "update", "--exclude", "alpha")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if got := f.downloaded(); got != "beta" {
		t.Errorf("downloaded = %q, want beta", got)
	}
}

func TestTexturepacksUpdateUnknownNames(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"positional", []string{"gamma", "nope"}},
		{"exclude", []string{"--exclude", "gamma,nope"}},
		// packs are named by texture_pack.conf, the folder name is not accepted
		{"folder name", []string{"beta_folder"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng, f, cdb := setupTexturepacks(t)

			code, _, stderr := runWithCDB(t, eng, cdb, append([]string{"texturepacks", "update"}, tt.args...)...)
			if code != exitError || !strings.Contains(stderr, "not installed: ") || strings.Contains(stderr, "gamma") {
				t.Errorf("code = %d, stderr = %q", code, stderr)
			}
			if got := f.downloaded(); got != "" {
				t.Errorf("downloaded = %q, want nothing", got)
			}
		})
	}
}

func TestTexturepacksUpdateDryRun(t *testing.T) {
	eng, f, cdb := setupTexturepacks(t)

	code, stdout, _ := runWithCDB(t, eng, cdb, "texturepacks", "update", "-n")
	if code != exitOK || !strings.Contains(stdout, "Would update:") || !strings.Contains(stdout, "alpha") {
		t.Errorf("code = %d, stdout = %q", code, stdout)
	}
	if got := f.downloaded(); got != "" {
		t.Errorf("downloaded = %q, want nothing", got)
	}
}

func TestTexturepacksUpdateNothingToDo(t *testing.T) {
	eng, f, cdb := setupTexturepacks(t)
	f.updatesJSON = `{"jane/gamma": 5}`

	code, stdout, _ := runWithCDB(t, eng, cdb, "texturepacks", "update")
	if code != exitOK || !strings.Contains(stdout, "Nothing to update.") {
		t.Errorf("code = %d, stdout = %q", code, stdout)
	}
}

func TestTexturepacksUpdateContinuesAfterFailure(t *testing.T) {
	eng, f, cdb := setupTexturepacks(t)
	f.failFor = map[string]bool{"alpha": true}

	code, _, stderr := runWithCDB(t, eng, cdb, "texturepacks", "update")
	if code != exitError {
		t.Errorf("code = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "alpha: update failed") || !strings.Contains(stderr, "1 of 2 updates failed") {
		t.Errorf("stderr = %q", stderr)
	}

	rel := installedTexturepackReleases(t, eng.TexturesDir)
	if rel["alpha"] != 1 || rel["beta_folder"] != 3 {
		t.Errorf("releases after update = %v, want alpha untouched at 1 and beta_folder at 3", rel)
	}
}
