package update

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/luanti-org/luma/internal/content"
	"github.com/luanti-org/luma/internal/contentdb"
	"github.com/luanti-org/luma/internal/engine"
	"github.com/luanti-org/luma/internal/util"
)

// TexturepackUpdate pairs a locally installed texture pack with the latest release CDB knows about for it.
type TexturepackUpdate struct {
	Texturepack   content.Texturepack
	LatestRelease int
}

// CheckAllTexturepackUpdates compares texture packs against ContentDB's /api/updates/ and
// returns the ones with a newer release available, following the Luanti client.
// engineInfo provides version for compatible releases.
// Texture packs with no author or no release are skipped
func CheckAllTexturepackUpdates(texturepacks []content.Texturepack, client *contentdb.Client, engineInfo engine.Info) ([]TexturepackUpdate, error) {
	latest, err := client.Updates(contentdb.UpdatesOptions{
		Types:           []string{"txp"},
		ProtocolVersion: engineInfo.Protocol,
		EngineVersion:   engineInfo.Version,
	})
	if err != nil {
		return nil, err
	}

	latest = lowerKeys(latest)

	var updates []TexturepackUpdate
	for _, tp := range texturepacks {
		if u, ok := matchTexturepackUpdate(tp, latest); ok {
			updates = append(updates, u)
		}
	}

	return updates, nil
}

// UpdateTexturepack downloads and installs the latest release for u, replacing the texture pack's existing directory in place.
func UpdateTexturepack(client *contentdb.Client, u TexturepackUpdate) error {
	tp := u.Texturepack

	url := client.ReleaseDownloadURL(tp.Author, tp.Name, u.LatestRelease, contentdb.ReasonUpdate)
	if err := DownloadPackage(client, url, tp.Path); err != nil {
		return err
	}

	// the name keeps a pack in a renamed folder matched to its package
	return util.SetConfFields(filepath.Join(tp.Path, "texture_pack.conf"), map[string]string{
		"name":    tp.Name,
		"author":  tp.Author,
		"release": strconv.Itoa(u.LatestRelease),
	})
}

// matchTexturepackUpdate reports whether tp has a newer release in latest, whose keys must be lowercase
func matchTexturepackUpdate(tp content.Texturepack, latest map[string]int) (TexturepackUpdate, bool) {
	if tp.Author == "" || tp.Release <= 0 {
		return TexturepackUpdate{}, false
	}

	release, ok := latest[strings.ToLower(tp.Author+"/"+tp.Name)]
	if !ok || release <= tp.Release {
		return TexturepackUpdate{}, false
	}

	return TexturepackUpdate{Texturepack: tp, LatestRelease: release}, true
}
