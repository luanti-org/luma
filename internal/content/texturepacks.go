package content

import (
	"os"
	"path/filepath"

	"github.com/luanti-org/luma/internal/util"
)

// Texturepack describes one texturepack folder found under a textures directory.
type Texturepack struct {
	Name        string
	Title       string
	Description string
	Author      string
	Release     int // from texture_pack.conf's `release`; 0 if absent

	Dir  string // folder name, e.g. "soothing32"
	Path string // full path to the texturepack folder

	// ConfOK is true only if texture_pack.conf exists and has a usable `name`
	// field. If texture_pack.conf is missing, empty, or has no name, Name
	// falls back to the folder name and ConfOK is false.
	ConfOK bool
}

// ScanTextures scans the immediate subdirectories of dir for texture packs.
func ScanTextures(dir string) ([]Texturepack, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var texturepacks []Texturepack

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		textureDir := entry.Name()
		texturePath := filepath.Join(dir, textureDir)

		texturepacks = append(texturepacks, scanTexturepackDir(textureDir, texturePath))
	}

	return texturepacks, nil
}

func scanTexturepackDir(dir, path string) Texturepack {
	tp := Texturepack{
		Name: dir, // fallback, overwritten below if texture_pack.conf has a name
		Dir:  dir,
		Path: path,
	}

	confPath := filepath.Join(path, "texture_pack.conf")
	data, err := os.ReadFile(confPath)
	if err != nil {
		return tp
	}

	conf := util.ParseConfFile(data)

	name, ok := conf["name"]
	if ok && name != "" {
		tp.Name = name
		tp.ConfOK = true
	}

	tp.Title = conf["title"]
	tp.Description = conf["description"]
	tp.Author = conf["author"]
	tp.Release = util.ParseIntField(conf, "release")

	return tp
}
