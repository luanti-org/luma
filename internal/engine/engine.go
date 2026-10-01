// Package engine detects the engine version and protocol version of a
// local Luanti install. An install can be a plain directory or a flatpak

package engine

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// ErrProtocolUnknown means the engine version was read, but the protocol table has no entry for it
var ErrProtocolUnknown = errors.New("engine: protocol version unknown for this engine version")

type Info struct {
	Version     string // as printed by --version e.g. "5.17.0"
	Protocol    int    // 0 if unknown
	Source      string // full path to the engine's own install root
	ModsDir     string // where this install's mods live
	GamesDir    string // where this install's games live
	TexturesDir string //where this install's texture packs live
	ViaFlatpak  bool   // true if Source is a flatpak install
}

var (
	versionLine    = regexp.MustCompile(`^(?:Luanti|Minetest)\s+(\S+)`)
	runInPlaceLine = regexp.MustCompile(`^RUN_IN_PLACE=(\d)`)
	protocolEntry  = regexp.MustCompile(`\["(\d+\.\d+\.\d+)"\]\s*=\s*(\d+)`)
)

// Detect finds the install chosen by --dir/--flatpak, defaulting to the current directory.
// fatal means there is nothing usable, warn means content dirs are valid but engine details are incomplete.
func Detect(dir string, flatpak bool) (info Info, warn, fatal error) {
	if flatpak {
		info, err := DetectFlatpak(DefaultFlatpakAppID)
		if errors.Is(err, ErrFlatpakUnavailable) || errors.Is(err, ErrFlatpakAppNotFound) {
			return info, nil, err
		}
		return info, err, nil
	}

	where := dir
	if dir == "" {
		dir, where = ".", "the current directory"
	}

	if !dirExists(dir) {
		return Info{}, nil, fmt.Errorf("%s is not a directory", dir)
	}

	info, err := DetectDir(dir)
	// no binary is fine for a data-only folder, but it must at least hold content
	if errors.Is(err, ErrNoBinary) && !dirExists(info.ModsDir) && !dirExists(info.GamesDir) && !dirExists(info.TexturesDir) {
		return info, nil, fmt.Errorf("no Luanti install in %s, use --dir or --flatpak", where)
	}

	return info, err, nil
}

func dirExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
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

// parseRunInPlace reads the RUN_IN_PLACE=0/1 line from --version output.
// ok is false if the line isn't present.
func parseRunInPlace(out string) (runInPlace, ok bool) {
	for _, line := range strings.Split(out, "\n") {
		if m := runInPlaceLine.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			return m[1] == "1", true
		}
	}

	return false, false
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
