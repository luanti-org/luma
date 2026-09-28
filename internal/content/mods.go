// Package content scans and parses locally installed Luanti content
// (mods, and eventually games/texture packs).
package content

import (
	"os"
	"path/filepath"

	"github.com/luanti-org/luma/internal/util"
)

// Mod describes one mod folder found under a mods directory.
type Mod struct {
	Name            string
	Title           string
	Description     string
	Author          string
	Release         int // from mod.conf's `release`; 0 if absent
	Depends         []string
	OptionalDepends []string

	Dir       string // folder name, e.g. "everness"
	Path      string // full path to the mod folder
	IsModpack bool   // parsed from modpack.conf instead of mod.conf - nested mods not scanned yet

	// ConfOK is true only if mod.conf/modpack.conf exists and has
	// a usable `name` field.
	// If it's missing, empty, or has no name, Name falls back to
	// the folder name and ConfOK is false.
	ConfOK bool
}

// ScanMods scans the immediate subdirectories of dir for mods.
// Directories containing a modpack.conf are reported with IsModpack true
func ScanMods(dir string) ([]Mod, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var mods []Mod

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		modDir := entry.Name()
		modPath := filepath.Join(dir, modDir)

		if util.FileExists(filepath.Join(modPath, "modpack.conf")) {
			mods = append(mods, scanModpackDir(modDir, modPath))
			continue
		}

		mods = append(mods, scanModDir(modDir, modPath))
	}

	return mods, nil
}

func scanModDir(dir, path string) Mod {
	return scanConfDir(dir, path, "mod.conf")
}

func scanModpackDir(dir, path string) Mod {
	m := scanConfDir(dir, path, "modpack.conf")
	m.IsModpack = true

	return m
}

// scanConfDir reads confFile (mod.conf or modpack.conf) out of path.
func scanConfDir(dir, path, confFile string) Mod {
	m := Mod{
		Name: dir, // fallback, overwritten below if the conf file has a name
		Dir:  dir,
		Path: path,
	}

	data, err := os.ReadFile(filepath.Join(path, confFile))
	if err != nil {
		return m
	}

	conf := util.ParseConfFile(data)

	name, ok := conf["name"]
	if ok && name != "" {
		m.Name = name
		m.ConfOK = true
	}

	m.Title = conf["title"]
	m.Description = conf["description"]
	m.Author = conf["author"]
	m.Release = util.ParseIntField(conf, "release")
	m.Depends = util.SplitList(conf["depends"])
	m.OptionalDepends = util.SplitList(conf["optional_depends"])

	return m
}
