package update

import (
	"archive/zip"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/luanti-org/luma/internal/content"
	"github.com/luanti-org/luma/internal/contentdb"
	"github.com/luanti-org/luma/internal/util"
)

// LocalPackage is a package from a local zip or folder, see OpenLocal.
type LocalPackage struct {
	Type   string      // "mod", "game" or "txp" as on ContentDB, a modpack is a "mod"
	Name   string      // mod, modpack or texture pack name from its conf, else Folder. A game's ID is Folder.
	Folder string      // folder name to install into
	Author string      // from the conf, unverified
	Dir    string      // the package root
	Mod    content.Mod // the scanned mod or modpack, for Type "mod"

	tmpDir string
}

// OpenLocal opens the zip or folder at path, unpacking a zip into a temp folder that Close removes.
// folder overrides the folder name, which otherwise comes from the conf's name, the root folder or the zip name.
func OpenLocal(path, folder string) (p LocalPackage, err error) {
	path, err = filepath.Abs(path)
	if err != nil {
		return p, err
	}

	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return p, errors.New("no such file or folder")
	}
	if err != nil {
		return p, err
	}

	root := path
	if !info.IsDir() {
		p.tmpDir, err = os.MkdirTemp("", "luma-local-*")
		if err != nil {
			return p, err
		}
		defer func() {
			if err != nil {
				p.Close()
			}
		}()

		if err := unzip(path, p.tmpDir); errors.Is(err, zip.ErrFormat) {
			return p, errors.New("not a zip file or folder")
		} else if err != nil {
			return p, err
		}
		root = p.tmpDir
	}

	p.Dir, err = stripSingleTopLevelDir(root)
	if err != nil {
		return p, err
	}

	var confFile string
	p.Type, confFile = localType(p.Dir)
	if p.Type == "" {
		return p, errors.New("no game.conf, texture_pack.conf, modpack.conf or init.lua found")
	}

	conf := map[string]string{}
	if confFile != "" {
		conf, err = readConf(filepath.Join(p.Dir, confFile))
		if err != nil {
			return p, err
		}
	}
	p.Author = conf["author"]

	given := folder != ""
	if !given && p.Type != "game" {
		// in game.conf, name is a deprecated synonym for title
		folder = conf["name"]
	}
	if folder == "" {
		folder = filepath.Base(p.Dir)
		if p.Dir == p.tmpDir {
			folder = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		}
	}

	if !packageNameRe.MatchString(folder) {
		if given {
			return p, fmt.Errorf("%q is not a valid name", folder)
		}
		return p, fmt.Errorf("%q is not a valid name, use --name", folder)
	}
	p.Folder = folder

	p.Name = folder
	switch p.Type {
	case "mod":
		p.Mod, err = content.ScanModDir(folder, p.Dir)
		if err != nil {
			return p, err
		}
		p.Name = p.Mod.Name
	case "txp":
		if conf["name"] != "" {
			p.Name = conf["name"]
		}
	}

	return p, nil
}

// Close removes the temp folder a zip was unpacked into.
func (p LocalPackage) Close() error {
	if p.tmpDir == "" {
		return nil
	}
	return os.RemoveAll(p.tmpDir)
}

// Package returns p as a ContentDB package with no release, for PackageDir.
func (p LocalPackage) Package() contentdb.Package {
	return contentdb.Package{Author: p.Author, Name: p.Folder, Type: p.Type}
}

// Install copies p into dest, which must not exist. Nothing is written into its conf.
func (p LocalPackage) Install(dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("folder %s already exists", dest)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}

	if err := copyDir(p.Dir, dest); err != nil {
		os.RemoveAll(dest)
		return err
	}
	return nil
}

// localType returns the package type of the root dir and its conf file, in the order the manifests are checked.
func localType(dir string) (pkgType, confFile string) {
	switch {
	case util.FileExists(filepath.Join(dir, "game.conf")):
		return "game", "game.conf"
	case util.FileExists(filepath.Join(dir, "texture_pack.conf")):
		return "txp", "texture_pack.conf"
	case util.FileExists(filepath.Join(dir, "modpack.conf")):
		return "mod", "modpack.conf"
	case util.FileExists(filepath.Join(dir, "modpack.txt")):
		return "mod", ""
	case util.FileExists(filepath.Join(dir, "init.lua")):
		return "mod", "mod.conf"
	}
	return "", ""
}

// readConf parses the conf at path, a missing one is empty.
func readConf(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	return util.ParseConfFile(data), nil
}
