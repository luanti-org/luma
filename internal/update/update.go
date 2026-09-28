// Package update provides composite operations that combine locally scanned content info with the ContentDB client,
// so both the interactive TUI and a future non-interactive CLI can reuse the same logic
package update

import (
	"github.com/luanti-org/luma/internal/content"
	"github.com/luanti-org/luma/internal/contentdb"
	"github.com/luanti-org/luma/internal/engine"
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

// matchModUpdate reports whether mod has a newer release in latest
func matchModUpdate(mod content.Mod, latest map[string]int) (ModUpdate, bool) {
	if mod.Author == "" {
		return ModUpdate{}, false
	}

	release, ok := latest[mod.Author+"/"+mod.Name]
	if !ok || release == mod.Release {
		return ModUpdate{}, false
	}

	return ModUpdate{Mod: mod, LatestRelease: release}, true
}
