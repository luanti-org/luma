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

	"github.com/luanti-org/luma/internal/contentdb"
)

// ErrProtocolUnknown means the engine version was read, but the protocol table has no entry for it
var ErrProtocolUnknown = errors.New("engine: protocol version unknown for this engine version")

// ErrVersionInferred means the binary couldn't be read, so version and protocol come from the newest misc_s.lua entry
var ErrVersionInferred = errors.New("engine: version inferred from the newest builtin/game/misc_s.lua entry")

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

// Options picks the install and how its engine version is resolved.
type Options struct {
	Dir           string // plain-directory install, "" means the current directory
	Flatpak       bool
	EngineVersion string // fallback when nothing local gives a version
	// Versions lists ContentDB's known engine versions, only called when needed
	Versions func() ([]contentdb.EngineVersion, error)
}

// Detect finds the install chosen by --dir/--flatpak, defaulting to the current directory.
// fatal means there is nothing usable, warn means detection succeeded but with caveats.
func Detect(opts Options) (info Info, warn, fatal error) {
	if opts.Flatpak {
		info, err := DetectFlatpak(DefaultFlatpakAppID)
		if errors.Is(err, ErrFlatpakUnavailable) || errors.Is(err, ErrFlatpakAppNotFound) {
			return info, nil, err
		}
		return resolveVersion(opts, info, err)
	}

	dir, where := opts.Dir, opts.Dir
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

	return resolveVersion(opts, info, err)
}

// resolveVersion fills in whatever detection left unknown, detectErr being why it's unknown.
// Order: binary --version, newest misc_s.lua entry, then the --engine-version fallback.
func resolveVersion(opts Options, info Info, detectErr error) (Info, error, error) {
	if info.Version == "" {
		if opts.EngineVersion == "" {
			return info, nil, fmt.Errorf("can't detect the engine version, use --engine-version: %w", detectErr)
		}
		proto, err := contentDBProtocol(opts, opts.EngineVersion)
		if err != nil {
			return info, nil, err
		}
		info.Version, info.Protocol = opts.EngineVersion, proto
		return info, nil, nil
	}

	var warn error
	if errors.Is(detectErr, ErrVersionInferred) {
		warn = detectErr
	}
	if opts.EngineVersion != "" {
		warn = errors.Join(warn, fmt.Errorf("ignoring --engine-version %s, detected %s", opts.EngineVersion, info.Version))
	}
	if info.Protocol == 0 {
		proto, err := contentDBProtocol(opts, info.Version)
		if err != nil {
			return info, nil, fmt.Errorf("%w (local lookup: %v)", err, detectErr)
		}
		info.Protocol = proto
	}

	return info, warn, nil
}

func contentDBProtocol(opts Options, version string) (int, error) {
	if opts.Versions == nil {
		return 0, fmt.Errorf("can't look up engine version %s: no ContentDB client", version)
	}
	versions, err := opts.Versions()
	if err != nil {
		return 0, fmt.Errorf("can't look up engine version %s on ContentDB: %w", version, err)
	}

	return MatchVersion(versions, version)
}

// MatchVersion finds version's protocol in ContentDB's list,
// which only names major.minor, e.g. "5.17.1" -> "5.17", "5.18.0-dev-abc" -> "5.18-dev"
func MatchVersion(versions []contentdb.EngineVersion, version string) (int, error) {
	base, suffix, _ := strings.Cut(version, "-")
	parts := strings.Split(base, ".")
	if len(parts) < 2 {
		return 0, fmt.Errorf("engine version %q is not in major.minor form", version)
	}
	name := parts[0] + "." + parts[1]

	candidates := []string{name}
	if suffix != "" {
		// prereleases are listed as "-dev" before the release exists
		candidates = []string{name + "-dev", name}
	}
	for _, c := range candidates {
		for _, v := range versions {
			if v.Name == c && v.ProtocolVersion > 0 {
				return v.ProtocolVersion, nil
			}
		}
	}

	return 0, fmt.Errorf("engine version %s is not known to ContentDB", version)
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

// newestEntry returns the table's highest-protocol version, preferring the higher version on ties
func newestEntry(table map[string]int) (string, int, bool) {
	best, bestProto := "", 0
	for v, p := range table {
		if p > bestProto || (p == bestProto && compareVersions(v, best) > 0) {
			best, bestProto = v, p
		}
	}

	return best, bestProto, bestProto > 0
}

// compareVersions numerically compares dotted versions like "5.10.0" and "5.9.1"
func compareVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		var x, y int
		if i < len(as) {
			x, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(bs[i])
		}
		if x != y {
			return x - y
		}
	}

	return 0
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
