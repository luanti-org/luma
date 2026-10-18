package cli

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luanti-org/luma/internal/contentdb"
	"github.com/luanti-org/luma/internal/engine"
	"github.com/luanti-org/luma/internal/util"
)

const installPackagesJSON = `[
	{"author": "jane", "name": "thing", "title": "Thing", "type": "mod", "release": 7},
	{"author": "amy", "name": "lib", "title": "Lib", "type": "mod", "release": 9},
	{"author": "jane", "name": "quest", "title": "Quest Land (formerly Questy)", "type": "game", "release": 3},
	{"author": "jane", "name": "pixels", "title": "Pixels", "type": "txp", "release": 4},
	{"author": "bob", "name": "dupe", "title": "Dupe", "type": "mod", "release": 1},
	{"author": "sam", "name": "dupe", "title": "Dupe", "type": "mod", "release": 2}
]`

// setupInstall returns an install with no content folders yet and a fake CDB serving installPackagesJSON,
// where jane/thing depends on lib.
func setupInstall(t *testing.T) (engine.Info, *fakeCDB, *contentdb.Client) {
	t.Helper()

	dir := t.TempDir()
	eng := engine.Info{
		ModsDir:     filepath.Join(dir, "mods"),
		GamesDir:    filepath.Join(dir, "games"),
		TexturesDir: filepath.Join(dir, "textures"),
		Version:     "5.17.0",
	}

	f := &fakeCDB{
		packagesJSON: installPackagesJSON,
		depsJSON: map[string]string{
			"jane/thing": `{"jane/thing": [{"name": "lib", "is_optional": false, "packages": ["amy/lib"]}]}`,
		},
	}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)

	return eng, f, contentdb.New(srv.URL)
}

func readConf(t *testing.T, path string) map[string]string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return util.ParseConfFile(data)
}

func TestInstallModWithDeps(t *testing.T) {
	eng, f, cdb := setupInstall(t)

	code, stdout, stderr := runWithCDB(t, eng, cdb, "install", "-y", "thing")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if got := f.downloaded(); got != "lib,thing" {
		t.Errorf("downloads = %q, want the dependency first", got)
	}
	if !strings.Contains(stdout, "jane/thing: installed 7") || !strings.Contains(stdout, "amy/lib: installed 9 (dependency of jane/thing)") {
		t.Errorf("stdout = %q", stdout)
	}

	conf := readConf(t, filepath.Join(eng.ModsDir, "thing", "mod.conf"))
	if conf["author"] != "jane" || conf["release"] != "7" {
		t.Errorf("mod.conf = %v", conf)
	}
}

func TestInstallGameAndTexturepack(t *testing.T) {
	eng, f, cdb := setupInstall(t)

	code, _, stderr := runWithCDB(t, eng, cdb, "install", "jane/quest", "PIXELS")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if got := f.downloaded(); got != "quest,pixels" {
		t.Errorf("downloads = %q", got)
	}

	conf := readConf(t, filepath.Join(eng.GamesDir, "quest", "game.conf"))
	if conf["author"] != "jane" || conf["release"] != "3" || conf["name"] != "" {
		t.Errorf("game.conf = %v", conf)
	}

	conf = readConf(t, filepath.Join(eng.TexturesDir, "pixels", "texture_pack.conf"))
	if conf["name"] != "pixels" || conf["author"] != "jane" || conf["release"] != "4" {
		t.Errorf("texture_pack.conf = %v", conf)
	}
}

func TestInstallLookupErrors(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want string
	}{
		{"ambiguous", "dupe", `"dupe" is ambiguous, use "author/name" format`},
		{"suggested by title", "questland", `"questland" not found on ContentDB, did you mean jane/quest, "Quest Land (formerly Questy)"?`},
		{"not found", "nothing", `"nothing" not found on ContentDB`},
		{"id not found", "jane/nothing", `"jane/nothing" not found on ContentDB`},
		{"no compatible release", "jane/old", `"jane/old" has no release compatible with Luanti 5.17.0`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng, f, cdb := setupInstall(t)
			f.details = map[string]bool{"jane/old": true}

			// the valid package is not installed either, a run installs all or nothing
			code, _, stderr := runWithCDB(t, eng, cdb, "install", "-y", "jane/quest", tt.arg)
			if code != exitError {
				t.Errorf("code = %d, want %d", code, exitError)
			}
			if !strings.Contains(stderr, tt.want) {
				t.Errorf("stderr = %q, want it to contain %q", stderr, tt.want)
			}
			if got := f.downloaded(); got != "" {
				t.Errorf("downloads = %q, want none", got)
			}
		})
	}
}

func TestInstallAlreadyInstalled(t *testing.T) {
	eng, f, cdb := setupInstall(t)
	writeFile(t, filepath.Join(eng.ModsDir, "thing", "mod.conf"), "name = thing\n")
	writeFile(t, filepath.Join(eng.TexturesDir, "my_pixels", "texture_pack.conf"), "name = pixels\n")

	code, _, stderr := runWithCDB(t, eng, cdb, "install", "-y", "thing", "jane/pixels", "jane/quest")
	if code != exitError {
		t.Errorf("code = %d, want %d", code, exitError)
	}
	for _, want := range []string{
		`"thing" is already installed, use 'luma mods update'`,
		`"jane/pixels" is already installed, use 'luma texturepacks update'`,
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr = %q, want it to contain %q", stderr, want)
		}
	}
	if got := f.downloaded(); got != "" {
		t.Errorf("downloads = %q, want none", got)
	}
}

func TestInstallSameFolder(t *testing.T) {
	eng, f, cdb := setupInstall(t)

	code, _, stderr := runWithCDB(t, eng, cdb, "install", "bob/dupe", "sam/dupe")
	if code != exitError || !strings.Contains(stderr, "bob/dupe and sam/dupe both install to") {
		t.Errorf("code = %d, stderr = %q", code, stderr)
	}
	if got := f.downloaded(); got != "" {
		t.Errorf("downloads = %q, want none", got)
	}
}

func TestInstallDepsNeedYes(t *testing.T) {
	eng, f, cdb := setupInstall(t)

	code, _, stderr := runWithCDB(t, eng, cdb, "install", "thing")
	if code != exitError || !strings.Contains(stderr, "dependencies required (amy/lib), rerun with -y or --no-deps") {
		t.Errorf("code = %d, stderr = %q", code, stderr)
	}
	if got := f.downloaded(); got != "" {
		t.Errorf("downloads = %q, want none", got)
	}

	code, _, stderr = runWithCDB(t, eng, cdb, "install", "--no-deps", "thing")
	if code != exitOK {
		t.Fatalf("--no-deps: code = %d, stderr = %q", code, stderr)
	}
	if got := f.downloaded(); got != "thing" {
		t.Errorf("--no-deps: downloads = %q", got)
	}
}

func TestInstallFailureRollsBack(t *testing.T) {
	eng, f, cdb := setupInstall(t)
	writeFile(t, filepath.Join(eng.ModsDir, "other", "mod.conf"), "name = other\n")
	f.failFor = map[string]bool{"pixels": true}

	code, _, stderr := runWithCDB(t, eng, cdb, "install", "-y", "thing", "jane/quest", "pixels")
	if code != exitError || !strings.Contains(stderr, "install failed, nothing was changed") {
		t.Errorf("code = %d, stderr = %q", code, stderr)
	}
	if got := f.downloaded(); got != "lib,thing,quest,pixels" {
		t.Errorf("downloads = %q", got)
	}

	for _, dir := range []string{
		filepath.Join(eng.ModsDir, "lib"),
		filepath.Join(eng.ModsDir, "thing"),
		filepath.Join(eng.GamesDir, "quest"),
		filepath.Join(eng.TexturesDir, "pixels"),
	} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("%s left behind: %v", dir, err)
		}
	}
	if _, err := os.Stat(filepath.Join(eng.ModsDir, "other")); err != nil {
		t.Errorf("mod installed before the run was removed: %v", err)
	}
}

func TestInstallDryRun(t *testing.T) {
	eng, f, cdb := setupInstall(t)

	code, stdout, stderr := runWithCDB(t, eng, cdb, "install", "-n", "thing", "pixels")
	if code != exitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	for _, want := range []string{"Would install:", "jane/thing", "texture pack", "Would install as dependencies:", "amy/lib"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout = %q, want it to contain %q", stdout, want)
		}
	}
	if got := f.downloaded(); got != "" {
		t.Errorf("downloads = %q, want none", got)
	}
	if _, err := os.Stat(eng.ModsDir); !os.IsNotExist(err) {
		t.Errorf("mods dir created on a dry run: %v", err)
	}
}

func TestInstallUsage(t *testing.T) {
	code, _, stderr := run(t, engine.Info{}, "install")
	if code != exitUsage || !strings.Contains(stderr, "expected at least one package") {
		t.Errorf("code = %d, stderr = %q", code, stderr)
	}
}
