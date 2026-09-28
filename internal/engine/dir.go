package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// ErrNoBinary means dir has no bin/luanti or bin/minetest to run.
var ErrNoBinary = errors.New("engine: no luanti or minetest binary found in bin/")

// Windows names are also tried
var binaryNames = []string{"luanti", "luanti.exe", "minetest", "minetest.exe"}

// buildConfigs are CMake's default multi-config generator names.
// A local build with one nests its binary under bin/<config>/ instead of bin/ directly.
var buildConfigs = []string{"Release", "RelWithDebInfo", "MinSizeRel", "Debug"}

// DetectDir reads the version and protocol version of the plain-directory install at `dir`.
// ModsDir/GamesDir are always set, even if the engine binary can't be found.
func DetectDir(dir string) (Info, error) {
	root := dir
	if abs, err := filepath.Abs(dir); err == nil {
		root = abs
	}
	info := Info{
		Source:      root,
		ModsDir:     filepath.Join(root, "mods"),
		GamesDir:    filepath.Join(root, "games"),
		TexturesDir: filepath.Join(root, "textures"),
	}

	out, err := readVersionOutput(dir)
	if err != nil {
		return info, err
	}

	version, err := parseVersionOutput(out)
	if err != nil {
		return info, err
	}
	info.Version = version

	// dir/mods, dir/games only hold if the binary was built RUN_IN_PLACE
	// otherwise user content lives at the engine's own user path
	if runInPlace, ok := parseRunInPlace(out); ok && !runInPlace {
		if userPath, err := userDataPath(); err == nil {
			info.ModsDir = filepath.Join(userPath, "mods")
			info.GamesDir = filepath.Join(userPath, "games")
			info.TexturesDir = filepath.Join(userPath, "textures")
		}
	}

	table, err := ReadProtocolTable(dir)
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

// ReadVersion runs the install's binary with --version and parses the result.
func ReadVersion(dir string) (string, error) {
	out, err := readVersionOutput(dir)
	if err != nil {
		return "", err
	}

	return parseVersionOutput(out)
}

// readVersionOutput runs the install's binary with --version and returns
// its raw output, e.g. for parseRunInPlace as well as parseVersionOutput.
func readVersionOutput(dir string) (string, error) {
	path, err := findBinary(dir)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	out, err := exec.CommandContext(ctx, path, "--version").CombinedOutput()
	cancel()
	if err != nil {
		return "", fmt.Errorf("engine: running %s --version: %w", path, err)
	}

	return string(out), nil
}

// userDataPath resolves Luanti's user data path outside RUN_IN_PLACE,
// matching porting::getUserPathEnvVar() and its per-OS defaults
func userDataPath() (string, error) {
	if p := os.Getenv("LUANTI_USER_PATH"); p != "" {
		return p, nil
	}
	if p := os.Getenv("MINETEST_USER_PATH"); p != "" {
		return p, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "Luanti"), nil
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "minetest"), nil
	default:
		return filepath.Join(home, ".minetest"), nil
	}
}

// findBinary checks dir/bin/<name> first,
// then dir/bin/<config>/<name> for each of buildConfigs
func findBinary(dir string) (string, error) {
	for _, name := range binaryNames {
		path := filepath.Join(dir, "bin", name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	for _, config := range buildConfigs {
		for _, name := range binaryNames {
			path := filepath.Join(dir, "bin", config, name)
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}
	}

	return "", ErrNoBinary
}

// ReadProtocolTable parses core.protocol_versions from the install's
// builtin/game/misc_s.lua, mapping "5.17.0" to its protocol version.
func ReadProtocolTable(dir string) (map[string]int, error) {
	path := filepath.Join(dir, "builtin", "game", "misc_s.lua")
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
