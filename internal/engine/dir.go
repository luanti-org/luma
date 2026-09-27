package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// ErrNoBinary means dir has no bin/luanti or bin/minetest to run.
var ErrNoBinary = errors.New("engine: no luanti or minetest binary found in bin/")

// Windows names are also tried
var binaryNames = []string{"luanti", "luanti.exe", "minetest", "minetest.exe"}

// DetectDir reads the version and protocol version of the plain-directory install at `dir`
func DetectDir(dir string) (Info, error) {
	version, err := ReadVersion(dir)
	if err != nil {
		return Info{}, err
	}

	info := Info{Version: version}

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
	for _, name := range binaryNames {
		path := filepath.Join(dir, "bin", name)
		if _, err := os.Stat(path); err != nil {
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		out, err := exec.CommandContext(ctx, path, "--version").CombinedOutput()
		cancel()
		if err != nil {
			return "", fmt.Errorf("engine: running %s --version: %w", path, err)
		}

		return parseVersionOutput(string(out))
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
