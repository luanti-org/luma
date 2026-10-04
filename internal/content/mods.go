// Package content scans and parses locally installed Luanti content
// (mods, and eventually games/texture packs).
package content

import (
	"errors"
	"io/fs"
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
	Release         int // from *.conf's `release`; 0 if absent
	Depends         []string
	OptionalDepends []string

	Dir         string // folder name, e.g. "everness"
	Path        string // full path to the mod folder
	IsModpack   bool   // has modpack.conf or modpack.txt instead of mod.conf
	ModpackMods []Mod  // mods in a modpack, flattened across nested modpacks
	HasInit     bool   // has init.lua, so the engine can load it as a mod

	// ConfOK is true only if *.conf exists and has a usable `name`
	// field. If *.conf is missing, empty, or has no name, Name
	// falls back to the folder name and ConfOK is false.
	ConfOK bool
}

// ScanMods scans the immediate subdirectories of dir for mods.
// Directories containing a modpack.conf or modpack.txt are reported
// with IsModpack set and scanned into ModpackMods.
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

		if isModpackDir(modPath) {
			mp, err := scanModpack(modDir, modPath)
			if err != nil {
				return nil, err
			}

			mods = append(mods, mp)
			continue
		}

		mods = append(mods, scanModDir(modDir, modPath))
	}

	return mods, nil
}

// ProvidedModNames returns the names of all loadable mods in mods, modpack members included.
func ProvidedModNames(mods []Mod) map[string]bool {
	names := make(map[string]bool)

	for _, m := range mods {
		if m.IsModpack {
			for _, member := range m.ModpackMods {
				names[member.Name] = true
			}
			continue
		}

		if m.HasInit {
			names[m.Name] = true
		}
	}

	return names
}

func isModpackDir(path string) bool {
	return util.FileExists(filepath.Join(path, "modpack.conf")) || util.FileExists(filepath.Join(path, "modpack.txt"))
}

func scanModpack(dir, path string) (Mod, error) {
	mp := Mod{
		Name:      dir,
		Dir:       dir,
		Path:      path,
		IsModpack: true,
	}

	members, err := scanModpackMods(path)
	if err != nil {
		return mp, err
	}
	mp.ModpackMods = members

	data, err := os.ReadFile(filepath.Join(path, "modpack.conf"))
	// a legacy pack has only modpack.txt
	if errors.Is(err, fs.ErrNotExist) {
		return mp, nil
	}
	if err != nil {
		return mp, err
	}

	conf := util.ParseConfFile(data)

	name, ok := conf["name"]
	if ok && name != "" {
		mp.Name = name
		mp.ConfOK = true
	}

	mp.Title = conf["title"]
	mp.Description = conf["description"]
	mp.Author = conf["author"]
	mp.Release = util.ParseIntField(conf, "release")

	return mp, nil
}

// scanModpackMods returns the mods under a modpack at path, flattened across nested modpacks.
func scanModpackMods(path string) ([]Mod, error) {
	folders, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	var mods []Mod

	for _, folder := range folders {
		if !folder.IsDir() {
			continue
		}

		subDir := folder.Name()
		subPath := filepath.Join(path, subDir)

		if isModpackDir(subPath) {
			nested, err := scanModpackMods(subPath)
			if err != nil {
				return nil, err
			}

			mods = append(mods, nested...)
			continue
		}

		if !util.FileExists(filepath.Join(subPath, "init.lua")) {
			continue
		}

		mods = append(mods, scanModDir(subDir, subPath))
	}

	return mods, nil
}

func scanModDir(dir, path string) Mod {
	m := Mod{
		Name:    dir, // fallback, overwritten below if mod.conf has a name
		Dir:     dir,
		Path:    path,
		HasInit: util.FileExists(filepath.Join(path, "init.lua")),
	}

	confPath := filepath.Join(path, "mod.conf")
	data, err := os.ReadFile(confPath)
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
