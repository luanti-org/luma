package content

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanMods(t *testing.T) {
	dir := t.TempDir()

	// well-formed mod
	writeFile(t, filepath.Join(dir, "goodmod", "mod.conf"),
		"name = goodmod\ntitle = Good Mod\nauthor = someone\nrelease = 42\ndepends = default, other\n")

	// mod.conf exists but has no name field
	writeFile(t, filepath.Join(dir, "noname", "mod.conf"),
		"title = No Name Mod\nauthor = someone\n")

	// no mod.conf at all
	writeFile(t, filepath.Join(dir, "bare_mod", "init.lua"), "-- nothing")

	// modpack: its own metadata is parsed, but it's not scanned for nested mods
	writeFile(t, filepath.Join(dir, "somepack", "modpack.conf"),
		"name = somepack\ntitle = Some Pack\nauthor = someone\nrelease = 7\n")
	writeFile(t, filepath.Join(dir, "somepack", "innermod", "mod.conf"), "name = innermod\n")

	mods, err := ScanMods(dir)
	if err != nil {
		t.Fatal(err)
	}

	byName := make(map[string]Mod)
	for _, m := range mods {
		byName[m.Dir] = m
	}

	if len(mods) != 4 {
		t.Fatalf("expected 4 entries, got %d: %+v", len(mods), mods)
	}

	good := byName["goodmod"]
	if !good.ConfOK || good.Name != "goodmod" || good.Title != "Good Mod" || good.Release != 42 {
		t.Errorf("goodmod: unexpected result %+v", good)
	}
	if len(good.Depends) != 2 || good.Depends[0] != "default" || good.Depends[1] != "other" {
		t.Errorf("goodmod: unexpected depends %v", good.Depends)
	}

	noname := byName["noname"]
	if noname.ConfOK || noname.Name != "noname" {
		t.Errorf("noname: expected fallback to folder name, got %+v", noname)
	}

	bare := byName["bare_mod"]
	if bare.ConfOK || bare.Name != "bare_mod" {
		t.Errorf("bare_mod: expected fallback to folder name, got %+v", bare)
	}

	pack := byName["somepack"]
	if !pack.IsModpack || !pack.ConfOK || pack.Title != "Some Pack" || pack.Author != "someone" || pack.Release != 7 {
		t.Errorf("somepack: unexpected result %+v", pack)
	}
}
