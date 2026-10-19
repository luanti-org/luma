package update

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func writeLocalFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()

	for name, data := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func writeLocalZip(t *testing.T, name, topLevelDir string, files map[string]string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, buildZip(t, topLevelDir, files), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOpenLocalZipWithNestedMod(t *testing.T) {
	path := writeLocalZip(t, "upload.zip", "mymod-main", map[string]string{
		"init.lua": "",
		"mod.conf": "name = mymod\nauthor = jane\ndepends = lib\n",
	})

	p, err := OpenLocal(path, "")
	if err != nil {
		t.Fatal(err)
	}

	if p.Type != "mod" || p.Folder != "mymod" || p.Name != "mymod" || p.Author != "jane" {
		t.Errorf("got %+v", p)
	}
	if hard, _ := p.Mod.Deps(); !slices.Equal(hard, []string{"lib"}) {
		t.Errorf("deps = %v", hard)
	}
	if !strings.HasSuffix(p.Dir, "mymod-main") {
		t.Errorf("root = %s, want the nested folder", p.Dir)
	}

	tmp := p.tmpDir
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Errorf("temp folder left behind: %v", err)
	}
}

func TestOpenLocalNames(t *testing.T) {
	tests := []struct {
		name       string
		zipName    string // a folder is opened if empty
		topDir     string
		files      map[string]string
		folderFlag string
		wantType   string
		wantFolder string
		wantName   string
		wantErr    string
	}{
		{
			name: "game from its folder", zipName: "x.zip", topDir: "mineclonia",
			files:    map[string]string{"game.conf": "title = Mineclonia\nname = Ignored\n"},
			wantType: "game", wantFolder: "mineclonia", wantName: "mineclonia",
		},
		{
			name: "invalid folder name", zipName: "x.zip", topDir: "mineclonia-main",
			files:   map[string]string{"game.conf": "title = Mineclonia\n"},
			wantErr: `"mineclonia-main" is not a valid name, use --name`,
		},
		{
			name: "--name fixes it", zipName: "x.zip", topDir: "mineclonia-main", folderFlag: "mineclonia",
			files:    map[string]string{"game.conf": "title = Mineclonia\n"},
			wantType: "game", wantFolder: "mineclonia", wantName: "mineclonia",
		},
		{
			name: "invalid --name", zipName: "x.zip", topDir: "mineclonia", folderFlag: "Bad Name",
			files:   map[string]string{"game.conf": "title = Mineclonia\n"},
			wantErr: `"Bad Name" is not a valid name`,
		},
		{
			name: "zip name when files are at the top", zipName: "submission.zip",
			files:    map[string]string{"texture_pack.conf": "title = Pixels\n"},
			wantType: "txp", wantFolder: "submission", wantName: "submission",
		},
		{
			name: "texture pack conf name", zipName: "x.zip", topDir: "pixels-1.0",
			files:    map[string]string{"texture_pack.conf": "name = pixels\n"},
			wantType: "txp", wantFolder: "pixels", wantName: "pixels",
		},
		{
			name: "--name keeps the conf name", zipName: "x.zip", topDir: "mymod", folderFlag: "other",
			files:    map[string]string{"init.lua": "", "mod.conf": "name = mymod\n"},
			wantType: "mod", wantFolder: "other", wantName: "mymod",
		},
		{
			name: "folder with a nested mod and no conf", topDir: "nested",
			files:    map[string]string{"init.lua": ""},
			wantType: "mod", wantFolder: "nested", wantName: "nested",
		},
		{
			name: "modpack", topDir: "pack",
			files: map[string]string{
				"modpack.conf": "name = bigpack\n",
				"a/init.lua":   "", "a/mod.conf": "name = a\ndepends = b, lib\n",
				"b/init.lua": "", "b/mod.conf": "name = b\n",
			},
			wantType: "mod", wantFolder: "bigpack", wantName: "bigpack",
		},
		{
			name: "no manifest", zipName: "x.zip", topDir: "stuff",
			files:   map[string]string{"readme.txt": ""},
			wantErr: "no game.conf, texture_pack.conf, modpack.conf or init.lua found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var path string
			if tt.zipName != "" {
				path = writeLocalZip(t, tt.zipName, tt.topDir, tt.files)
			} else {
				path = t.TempDir()
				writeLocalFiles(t, filepath.Join(path, tt.topDir), tt.files)
			}

			p, err := OpenLocal(path, tt.folderFlag)
			defer p.Close()

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}

			if p.Type != tt.wantType || p.Folder != tt.wantFolder || p.Name != tt.wantName {
				t.Errorf("type, folder, name = %q, %q, %q, want %q, %q, %q",
					p.Type, p.Folder, p.Name, tt.wantType, tt.wantFolder, tt.wantName)
			}
		})
	}
}

func TestOpenLocalModpackDeps(t *testing.T) {
	dir := t.TempDir()
	writeLocalFiles(t, dir, map[string]string{
		"modpack.conf": "name = bigpack\n",
		"a/init.lua":   "", "a/mod.conf": "name = a\ndepends = b, lib\n",
		"b/init.lua": "", "b/mod.conf": "name = b\n",
	})

	p, err := OpenLocal(dir, "")
	if err != nil {
		t.Fatal(err)
	}

	if hard, _ := p.Mod.Deps(); !slices.Equal(hard, []string{"lib"}) {
		t.Errorf("deps = %v, want only those the pack doesn't contain", hard)
	}
}

func TestOpenLocalNotAZip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.txt")
	writeLocalFiles(t, filepath.Dir(path), map[string]string{"notes.txt": "hello"})

	if _, err := OpenLocal(path, ""); err == nil || !strings.Contains(err.Error(), "not a zip file or folder") {
		t.Errorf("err = %v", err)
	}
}

func TestOpenLocalZipLimits(t *testing.T) {
	entries, size := maxZipEntries, maxUnzippedSize
	t.Cleanup(func() { maxZipEntries, maxUnzippedSize = entries, size })

	maxZipEntries = 2
	path := writeLocalZip(t, "x.zip", "m", map[string]string{"init.lua": "", "a.lua": "", "b.lua": ""})
	if _, err := OpenLocal(path, ""); err == nil || !strings.Contains(err.Error(), "more than the limit of 2") {
		t.Errorf("entries: err = %v", err)
	}

	maxZipEntries = entries
	maxUnzippedSize = 10
	path = writeLocalZip(t, "x.zip", "m", map[string]string{"init.lua": "-- 0123456789"})
	if _, err := OpenLocal(path, ""); err == nil || !strings.Contains(err.Error(), "zip unpacks to more than the limit") {
		t.Errorf("size: err = %v", err)
	}
}

func TestLocalInstall(t *testing.T) {
	src := t.TempDir()
	writeLocalFiles(t, src, map[string]string{"init.lua": "x", "mod.conf": "name = mymod\n", "textures/a.png": "png"})

	p, err := OpenLocal(src, "")
	if err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "mods", "mymod")
	if err := p.Install(dest); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(dest, "textures", "a.png")); err != nil || string(data) != "png" {
		t.Errorf("copied file = %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(src, "init.lua")); err != nil {
		t.Errorf("source folder changed: %v", err)
	}

	if err := p.Install(dest); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("second install: err = %v", err)
	}
}

func TestLocalInstallRejectsSymlinks(t *testing.T) {
	src := t.TempDir()
	writeLocalFiles(t, src, map[string]string{"init.lua": "x"})
	if err := os.Symlink("/etc/hostname", filepath.Join(src, "link.txt")); err != nil {
		t.Skip("symlinks unsupported:", err)
	}

	p, err := OpenLocal(src, "mymod")
	if err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "mymod")
	if err := p.Install(dest); err == nil || !strings.Contains(err.Error(), "symlinks are not supported") {
		t.Errorf("err = %v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Errorf("partial copy left behind: %v", err)
	}
}
