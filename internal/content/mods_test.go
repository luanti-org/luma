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

	// modpack: its own metadata is parsed, and its members are scanned
	writeFile(t, filepath.Join(dir, "somepack", "modpack.conf"),
		"name = somepack\ntitle = Some Pack\ndescription = A test pack\nauthor = someone\nrelease = 7\n")
	writeFile(t, filepath.Join(dir, "somepack", "innermod", "mod.conf"), "name = innermod\n")
	writeFile(t, filepath.Join(dir, "somepack", "innermod", "init.lua"), "-- nothing")

	// pack member with no mod.conf at all
	writeFile(t, filepath.Join(dir, "somepack", "baremember", "init.lua"), "-- nothing")

	// non-mod folder inside the pack: no init.lua, must be excluded
	writeFile(t, filepath.Join(dir, "somepack", "doc", "readme.txt"), "not a mod")

	// nested modpack: its mods are flattened into the outer pack's members
	writeFile(t, filepath.Join(dir, "somepack", "subpack", "modpack.conf"), "name = subpack\n")
	writeFile(t, filepath.Join(dir, "somepack", "subpack", "deepmod", "init.lua"), "-- nothing")

	// modpack.conf with no name field
	writeFile(t, filepath.Join(dir, "nonamepack", "modpack.conf"), "title = No Name Pack\n")

	// legacy modpack: only modpack.txt, no conf to read
	writeFile(t, filepath.Join(dir, "txtpack", "modpack.txt"), "")
	writeFile(t, filepath.Join(dir, "txtpack", "txtmember", "init.lua"), "-- nothing")

	mods, err := ScanMods(dir)
	if err != nil {
		t.Fatal(err)
	}

	byName := make(map[string]Mod)
	for _, m := range mods {
		byName[m.Dir] = m
	}

	if len(mods) != 6 {
		t.Fatalf("expected 6 entries, got %d: %+v", len(mods), mods)
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
	if !bare.HasInit || good.HasInit {
		t.Errorf("HasInit: expected bare_mod true and goodmod false, got %v and %v", bare.HasInit, good.HasInit)
	}

	pack := byName["somepack"]
	if !pack.IsModpack || !pack.ConfOK || pack.Name != "somepack" ||
		pack.Title != "Some Pack" || pack.Description != "A test pack" ||
		pack.Author != "someone" || pack.Release != 7 {
		t.Errorf("somepack: unexpected result %+v", pack)
	}
	if pack.Path != filepath.Join(dir, "somepack") {
		t.Errorf("somepack: unexpected path %q", pack.Path)
	}

	// members: only the pack's own mod subfolders (init.lua required), with absolute paths
	if len(pack.ModpackMods) != 3 {
		t.Fatalf("somepack: expected 3 members, got %d: %+v", len(pack.ModpackMods), pack.ModpackMods)
	}
	members := make(map[string]Mod)
	for _, m := range pack.ModpackMods {
		members[m.Dir] = m
	}

	if _, ok := members["doc"]; ok {
		t.Errorf("somepack/doc: non-mod folder should be excluded, got %+v", members["doc"])
	}

	inner := members["innermod"]
	if !inner.ConfOK || inner.Name != "innermod" {
		t.Errorf("somepack/innermod: unexpected result %+v", inner)
	}
	if inner.Path != filepath.Join(dir, "somepack", "innermod") {
		t.Errorf("somepack/innermod: unexpected path %q", inner.Path)
	}

	bareMember := members["baremember"]
	if bareMember.ConfOK || bareMember.Name != "baremember" {
		t.Errorf("somepack/baremember: expected fallback to folder name, got %+v", bareMember)
	}
	if bareMember.Path != filepath.Join(dir, "somepack", "baremember") {
		t.Errorf("somepack/baremember: unexpected path %q", bareMember.Path)
	}

	if _, ok := members["subpack"]; ok {
		t.Errorf("somepack/subpack: nested modpack should not be a member itself, got %+v", members["subpack"])
	}

	deep := members["deepmod"]
	if deep.Name != "deepmod" || deep.Path != filepath.Join(dir, "somepack", "subpack", "deepmod") {
		t.Errorf("somepack/subpack/deepmod: unexpected result %+v", deep)
	}

	txtPack := byName["txtpack"]
	if !txtPack.IsModpack || txtPack.ConfOK || txtPack.Name != "txtpack" {
		t.Errorf("txtpack: expected a modpack named after its folder, got %+v", txtPack)
	}
	if len(txtPack.ModpackMods) != 1 || txtPack.ModpackMods[0].Name != "txtmember" {
		t.Errorf("txtpack: unexpected members %+v", txtPack.ModpackMods)
	}

	nonamePack := byName["nonamepack"]
	if !nonamePack.IsModpack || nonamePack.ConfOK || nonamePack.Name != "nonamepack" ||
		nonamePack.Title != "No Name Pack" {
		t.Errorf("nonamepack: expected fallback to folder name, got %+v", nonamePack)
	}
}

func TestProvidedModNames(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "plain", "init.lua"), "-- nothing")
	writeFile(t, filepath.Join(dir, "renamed", "mod.conf"), "name = realname\n")
	writeFile(t, filepath.Join(dir, "renamed", "init.lua"), "-- nothing")

	// no init.lua: not loadable, must not count
	writeFile(t, filepath.Join(dir, "stray", "readme.txt"), "not a mod")

	// the pack's own name is not a mod name, only its members are
	writeFile(t, filepath.Join(dir, "pack", "modpack.conf"), "name = pack\n")
	writeFile(t, filepath.Join(dir, "pack", "member", "init.lua"), "-- nothing")
	writeFile(t, filepath.Join(dir, "pack", "sub", "modpack.txt"), "")
	writeFile(t, filepath.Join(dir, "pack", "sub", "deep", "init.lua"), "-- nothing")

	mods, err := ScanMods(dir)
	if err != nil {
		t.Fatal(err)
	}

	got := ProvidedModNames(mods)

	want := []string{"plain", "realname", "member", "deep"}
	if len(got) != len(want) {
		t.Errorf("expected %d names, got %v", len(want), got)
	}
	for _, n := range want {
		if !got[n] {
			t.Errorf("expected %q in %v", n, got)
		}
	}
}
