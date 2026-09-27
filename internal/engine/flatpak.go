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

// DetectFlatpak reads the version and protocol version of appID as installed via flatpak
func DetectFlatpak(appID string) (Info, error) {
	version, err := FlatpakVersion(appID)
	if err != nil {
		return Info{}, err
	}

	info := Info{Version: version}

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
