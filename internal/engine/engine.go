// Package engine detects the engine version and protocol version of a
// local Luanti install. An install can be a plain directory or a flatpak

package engine

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ErrProtocolUnknown means the engine version was read, but the protocol table has no entry for it
var ErrProtocolUnknown = errors.New("engine: protocol version unknown for this engine version")

type Info struct {
	Version  string // as printed by --version e.g. "5.17.0"
	Protocol int    // 0 if unknown
}

var (
	versionLine   = regexp.MustCompile(`^(?:Luanti|Minetest)\s+(\S+)`)
	protocolEntry = regexp.MustCompile(`\["(\d+\.\d+\.\d+)"\]\s*=\s*(\d+)`)
)

// Detect tries DetectDir(dir) first, falling back to DetectFlatpak(DefaultFlatpakAppID) if dir has no binary
// returns the Version and Protocol struct
func Detect(dir string) (Info, error) {
	info, err := DetectDir(dir)
	if !errors.Is(err, ErrNoBinary) {
		return info, err
	}

	return DetectFlatpak(DefaultFlatpakAppID)
}

const protocolTableStart = "core.protocol_versions = {"

// parses --version output, e.g. "Luanti 5.17.0 (Windows)" -> "5.17.0".
func parseVersionOutput(out string) (string, error) {
	for _, line := range strings.Split(out, "\n") {
		if m := versionLine.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			return m[1], nil
		}
	}

	return "", fmt.Errorf("engine: no version line in --version output: %q", out)
}

// parseProtocolTable extracts core.protocol_versions from the text of a misc_s.lua file
func parseProtocolTable(src string) (map[string]int, error) {
	start := strings.Index(src, protocolTableStart)
	if start < 0 {
		return nil, errors.New("engine: no protocol table found")
	}
	block := src[start:]
	if end := strings.Index(block, "}"); end >= 0 {
		block = block[:end]
	}

	table := map[string]int{}
	for _, m := range protocolEntry.FindAllStringSubmatch(block, -1) {
		proto, _ := strconv.Atoi(m[2])
		table[m[1]] = proto
	}
	if len(table) == 0 {
		return nil, errors.New("engine: protocol table has no entries")
	}

	return table, nil
}

// lookupProtocol resolves version against table's release entries, falling back to the release's base patch version.
func lookupProtocol(table map[string]int, version string) (int, bool) {
	// dev/rc builds are assumed to speak the newest known protocol
	if strings.Contains(version, "-") {
		return maxProtocol(table)
	}
	if p, ok := table[version]; ok {
		return p, true
	}

	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return 0, false
	}
	p, ok := table[parts[0]+"."+parts[1]+".0"]

	return p, ok
}

func maxProtocol(table map[string]int) (int, bool) {
	max := 0
	for _, p := range table {
		if p > max {
			max = p
		}
	}

	return max, max > 0
}
