package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DefaultFlatpakAppID is Luanti's official flatpak app ID on Flathub
const DefaultFlatpakAppID = "org.luanti.luanti"

var (
	// ErrFlatpakUnavailable means the flatpak command itself isn't on PATH
	ErrFlatpakUnavailable = errors.New("engine: flatpak is not installed")
	// ErrFlatpakAppNotFound means flatpak is installed, but appID isn't.
	ErrFlatpakAppNotFound = errors.New("engine: flatpak app is not installed")
)

// DetectFlatpak reads the version and protocol version of appID as installed via flatpak.
// ModsDir/GamesDir are always set, even if the version/protocol can't be read,
// since content scanning doesn't depend on the engine binary being detectable.
func DetectFlatpak(appID string) (Info, error) {
	info := Info{ViaFlatpak: true}
	if home, err := os.UserHomeDir(); err == nil {
		// flatpak apps get their own HOME, set via MINETEST_USER_PATH in
		// the launcher script inside the sandbox; confirmed for org.luanti.luanti
		userPath := filepath.Join(home, ".var", "app", appID, ".minetest")
		info.ModsDir = filepath.Join(userPath, "mods")
		info.GamesDir = filepath.Join(userPath, "games")
		info.TexturesDir = filepath.Join(userPath, "textures")
	}
	if loc, err := flatpakLocation(appID); err == nil {
		info.Source = filepath.Join(loc, "files")
	}

	version, err := FlatpakVersion(appID)
	if err != nil {
		return info, err
	}
	info.Version = version

	table, err := FlatpakProtocolTable(appID)
	if err != nil {
		return info, err
	}

	proto, ok := lookupProtocol(table, version)
	if !ok {
		return info, ErrProtocolUnknown
	}
	info.Protocol = proto

	return info, nil
}

// FlatpakVersion runs `flatpak run <appID> --version` and parses the result.
// This launches the app inside its sandbox
func FlatpakVersion(appID string) (string, error) {
	if _, err := exec.LookPath("flatpak"); err != nil {
		return "", ErrFlatpakUnavailable
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "flatpak", "run", appID, "--version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("engine: %s: %w", appID, ErrFlatpakAppNotFound)
	}

	return parseVersionOutput(string(out))
}

// FlatpakProtocolTable parses core.protocol_versions out of `appID`
// builtin/game/misc_s.lua, mapping "5.17.0" to its protocol version
func FlatpakProtocolTable(appID string) (map[string]int, error) {
	loc, err := flatpakLocation(appID)
	if err != nil {
		return nil, err
	}

	// "luanti" here is the app's own share dir name, not the app ID
	path := filepath.Join(loc, "files", "share", "luanti", "builtin", "game", "misc_s.lua")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("engine: %w", err)
	}

	table, err := parseProtocolTable(string(data))
	if err != nil {
		return nil, fmt.Errorf("engine: %w in %s", err, path)
	}

	return table, nil
}

// flatpakLocation returns the install directory flatpak reports for
// appID, e.g. .../app/org.luanti.luanti/x86_64/stable/<commit>.
func flatpakLocation(appID string) (string, error) {
	if _, err := exec.LookPath("flatpak"); err != nil {
		return "", ErrFlatpakUnavailable
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "flatpak", "info", "--show-location", appID).Output()
	if err != nil {
		return "", fmt.Errorf("engine: %s: %w", appID, ErrFlatpakAppNotFound)
	}

	return strings.TrimSpace(string(out)), nil
}
