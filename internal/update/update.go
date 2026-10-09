// Package update provides composite operations that combine locally scanned content info with the ContentDB client,
// so both the interactive TUI and the non-interactive CLI can reuse the same logic
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

// ModUpdate pairs a locally installed mod with the latest release CDB knows about for it.
type ModUpdate struct {
	Mod           content.Mod
	LatestRelease int
}

// CheckAllModUpdates compares mods against ContentDB's /api/updates/ and
// returns the ones with a newer release available. Modpacks are included
// engineInfo provides version for compatible releases.
// Mods with no author are skipped
func CheckAllModUpdates(mods []content.Mod, client *contentdb.Client, engineInfo engine.Info) ([]ModUpdate, error) {
	latest, err := client.Updates(modUpdatesOptions(engineInfo))
	if err != nil {
		return nil, err
	}

	latest = lowerKeys(latest)

	var updates []ModUpdate
	for _, mod := range mods {
		if u, ok := matchModUpdate(mod, latest); ok {
			updates = append(updates, u)
		}
	}

	return updates, nil
}

func modUpdatesOptions(engineInfo engine.Info) contentdb.UpdatesOptions {
	return contentdb.UpdatesOptions{
		Types:           []string{"mod"},
		ProtocolVersion: engineInfo.Protocol,
		EngineVersion:   engineInfo.Version,
	}
}

// UpdateMod downloads and installs the latest release for u,
// replacing the mod's - or modpack's existing directory in place.
func UpdateMod(client *contentdb.Client, u ModUpdate) error {
	return InstallMod(client, u.Mod.Author, u.Mod.Name, u.LatestRelease, u.Mod.Path, contentdb.ReasonUpdate)
}

// InstallMod downloads and installs a release of author/name into destDir,
// replacing anything already there.
func InstallMod(client *contentdb.Client, author, name string, release int, destDir, reason string) error {
	url := client.ReleaseDownloadURL(author, name, release, reason)
	if err := DownloadPackage(client, url, destDir); err != nil {
		return err
	}

	// CDB zips lack these; the Luanti client writes them after install too
	return util.SetConfFields(modConfPath(destDir), map[string]string{
		"name":    name,
		"author":  author,
		"release": strconv.Itoa(release),
	})
}

// modConfPath returns modpack.conf for an installed modpack, else mod.conf.
func modConfPath(dir string) string {
	if util.FileExists(filepath.Join(dir, "modpack.conf")) || util.FileExists(filepath.Join(dir, "modpack.txt")) {
		return filepath.Join(dir, "modpack.conf")
	}
	return filepath.Join(dir, "mod.conf")
}

// lowerKeys returns m with lowercase keys, ContentDB ids are case-insensitive
func lowerKeys(m map[string]int) map[string]int {
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[strings.ToLower(k)] = v
	}
	return out
}

// matchModUpdate reports whether mod has a newer release in latest, whose keys must be lowercase
func matchModUpdate(mod content.Mod, latest map[string]int) (ModUpdate, bool) {
	if mod.Author == "" {
		return ModUpdate{}, false
	}

	release, ok := latest[strings.ToLower(mod.Author+"/"+mod.Name)]
	if !ok || release <= mod.Release {
		return ModUpdate{}, false
	}

	return ModUpdate{Mod: mod, LatestRelease: release}, true
}
