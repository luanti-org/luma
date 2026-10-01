package engine

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/luanti-org/luma/internal/contentdb"
)

func TestParseVersionOutput(t *testing.T) {
	tests := []struct {
		name, out, want string
		wantErr         bool
	}{
		{"luanti", "Luanti 5.17.0 (Windows)\nUsing LuaJIT 2.1\n", "5.17.0", false},
		{"minetest", "Minetest 5.4.1 (Linux)\n", "5.4.1", false},
		{"noise first", "warning: something\nLuanti 5.9.1 (Linux)\n", "5.9.1", false},
		{"dev", "Luanti 5.18.0-dev-abc123 (Linux)\n", "5.18.0-dev-abc123", false},
		{"crlf", "Luanti 5.17.0 (Windows)\r\nUsing LuaJIT\r\n", "5.17.0", false},
		{"garbage", "hello world\n", "", true},
		{"empty", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseVersionOutput(tt.out)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("version = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseRunInPlace(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want bool
		ok   bool
	}{
		{"in place", "Luanti 5.17.0 (Windows)\nBUILD_TYPE=Release\nRUN_IN_PLACE=1\n", true, true},
		{"not in place", "Luanti 5.17.0 (Linux)\nBUILD_TYPE=Release\nRUN_IN_PLACE=0\n", false, true},
		{"absent", "Luanti 5.17.0 (Linux)\n", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseRunInPlace(tt.out)
			if got != tt.want || ok != tt.ok {
				t.Errorf("parseRunInPlace(...) = %v, %v; want %v, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

const miscS = `
local other = {
	["9.9.9"] = 1,
}

-- Protocol version table
core.protocol_versions = {
	["5.0.0"] = 37,
	["5.9.0"] = 44,
	["5.9.1"] = 45,
	["5.16.0"] = 52,
	["5.17.0"] = 53,
}

setmetatable(core.protocol_versions, {})
`

func TestParseProtocolTable(t *testing.T) {
	table, err := parseProtocolTable(miscS)
	if err != nil {
		t.Fatalf("parseProtocolTable: %v", err)
	}
	want := map[string]int{"5.0.0": 37, "5.9.0": 44, "5.9.1": 45, "5.16.0": 52, "5.17.0": 53}
	if len(table) != len(want) {
		t.Fatalf("table = %v, want %v", table, want)
	}
	for k, v := range want {
		if table[k] != v {
			t.Errorf("table[%q] = %d, want %d", k, table[k], v)
		}
	}
}

func TestParseProtocolTableMissing(t *testing.T) {
	if _, err := parseProtocolTable("-- nothing here\n"); err == nil {
		t.Error("expected error for text without a protocol table")
	}
}

func TestLookupProtocol(t *testing.T) {
	table := map[string]int{"5.9.0": 44, "5.9.1": 45, "5.17.0": 53}
	tests := []struct {
		version string
		want    int
		ok      bool
	}{
		{"5.17.0", 53, true},
		{"5.9.1", 45, true},
		{"5.17.1", 53, true},
		{"5.17", 53, true},
		{"5.9.2", 44, true},
		{"5.18.0-dev", 53, true}, // assumed to speak the newest known protocol
		{"5.17.0-rc1", 53, true},
		{"6.0.0", 0, false},
		{"5", 0, false},
	}
	for _, tt := range tests {
		got, ok := lookupProtocol(table, tt.version)
		if got != tt.want || ok != tt.ok {
			t.Errorf("lookupProtocol(%q) = %d, %v; want %d, %v", tt.version, got, ok, tt.want, tt.ok)
		}
	}
}

func TestDetectMissingDir(t *testing.T) {
	_, warn, fatal := Detect(Options{Dir: filepath.Join(t.TempDir(), "nope")})
	if fatal == nil || !strings.Contains(fatal.Error(), "is not a directory") {
		t.Errorf("fatal = %v, want a not-a-directory error", fatal)
	}
	if warn != nil {
		t.Errorf("warn = %v, want nil", warn)
	}
}

func TestDetectEmptyDir(t *testing.T) {
	dir := t.TempDir()
	_, _, fatal := Detect(Options{Dir: dir})
	if fatal == nil || !strings.Contains(fatal.Error(), "no Luanti install in "+dir) {
		t.Errorf("fatal = %v, want a no-install error naming %s", fatal, dir)
	}
}

func TestDetectEmptyCurrentDir(t *testing.T) {
	t.Chdir(t.TempDir())
	_, _, fatal := Detect(Options{})
	if fatal == nil || !strings.Contains(fatal.Error(), "the current directory") {
		t.Errorf("fatal = %v, want it to name the current directory", fatal)
	}
}

func newDataOnlyDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "mods"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

var cdbVersions = []contentdb.EngineVersion{
	{Name: "5.16", ProtocolVersion: 52},
	{Name: "5.17", ProtocolVersion: 53},
	{Name: "5.18-dev", ProtocolVersion: 54, IsDev: true},
}

func fakeVersions(calls *int) func() ([]contentdb.EngineVersion, error) {
	return func() ([]contentdb.EngineVersion, error) {
		if calls != nil {
			*calls++
		}
		return cdbVersions, nil
	}
}

func offlineVersions() ([]contentdb.EngineVersion, error) {
	return nil, errors.New("offline")
}

func TestDetectDataOnlyDirNeedsEngineVersion(t *testing.T) {
	_, _, fatal := Detect(Options{Dir: newDataOnlyDir(t), Versions: fakeVersions(nil)})
	if fatal == nil || !strings.Contains(fatal.Error(), "--engine-version") {
		t.Errorf("fatal = %v, want an error pointing at --engine-version", fatal)
	}
}

func TestDetectDataOnlyDirWithEngineVersion(t *testing.T) {
	info, warn, fatal := Detect(Options{Dir: newDataOnlyDir(t), EngineVersion: "5.17.1", Versions: fakeVersions(nil)})
	if fatal != nil || warn != nil {
		t.Fatalf("warn = %v, fatal = %v, want both nil", warn, fatal)
	}
	if info.Version != "5.17.1" || info.Protocol != 53 || info.ModsDir == "" {
		t.Errorf("info = %+v, want 5.17.1 / 53 with ModsDir set", info)
	}
}

func TestDetectEngineVersionUnknown(t *testing.T) {
	_, _, fatal := Detect(Options{Dir: newDataOnlyDir(t), EngineVersion: "4.0", Versions: fakeVersions(nil)})
	if fatal == nil || !strings.Contains(fatal.Error(), "not known to ContentDB") {
		t.Errorf("fatal = %v, want a not-known-to-ContentDB error", fatal)
	}
}

func TestDetectEngineVersionOffline(t *testing.T) {
	_, _, fatal := Detect(Options{Dir: newDataOnlyDir(t), EngineVersion: "5.17", Versions: offlineVersions})
	if fatal == nil || !strings.Contains(fatal.Error(), "offline") {
		t.Errorf("fatal = %v, want the ContentDB error", fatal)
	}
}

func TestDetectEngineVersionIgnoredWhenDetected(t *testing.T) {
	dir := newInstall(t, "")
	if err := os.Mkdir(filepath.Join(dir, "mods"), 0o755); err != nil {
		t.Fatal(err)
	}

	info, warn, fatal := Detect(Options{Dir: dir, EngineVersion: "5.16", Versions: fakeVersions(nil)})
	if fatal != nil {
		t.Fatalf("fatal = %v, want nil", fatal)
	}
	if info.Version != "5.17.0" || info.Protocol != 53 {
		t.Errorf("info = %+v, want the misc_s.lua 5.17.0 / 53, not the flag", info)
	}
	if warn == nil || !strings.Contains(warn.Error(), "ignoring --engine-version") {
		t.Errorf("warn = %v, want it to mention the ignored flag", warn)
	}
}

func TestDetectInferredFromTable(t *testing.T) {
	dir := newInstall(t, "")
	if err := os.Mkdir(filepath.Join(dir, "mods"), 0o755); err != nil {
		t.Fatal(err)
	}
	calls := 0
	info, warn, fatal := Detect(Options{Dir: dir, Versions: fakeVersions(&calls)})
	if fatal != nil {
		t.Fatalf("fatal = %v, want nil", fatal)
	}
	if !errors.Is(warn, ErrVersionInferred) || !errors.Is(warn, ErrNoBinary) {
		t.Errorf("warn = %v, want ErrVersionInferred wrapping ErrNoBinary", warn)
	}
	if info.Version != "5.17.0" || info.Protocol != 53 {
		t.Errorf("info = %+v, want 5.17.0 / 53", info)
	}
	if calls != 0 {
		t.Errorf("ContentDB called %d times, want 0", calls)
	}
}

func TestDetectProtocolFromContentDB(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as a stand-in binary")
	}

	dir := newInstall(t, "Luanti 5.18.0-dev-abc123 (Linux)")
	os.Remove(filepath.Join(dir, "builtin", "game", "misc_s.lua"))

	info, warn, fatal := Detect(Options{Dir: dir, Versions: fakeVersions(nil)})
	if fatal != nil || warn != nil {
		t.Fatalf("warn = %v, fatal = %v, want both nil", warn, fatal)
	}
	if info.Protocol != 54 {
		t.Errorf("Protocol = %d, want 54 from ContentDB", info.Protocol)
	}

	_, _, fatal = Detect(Options{Dir: dir, Versions: offlineVersions})
	if fatal == nil {
		t.Error("fatal = nil, want an error when ContentDB is unavailable")
	}
}

func TestMatchVersion(t *testing.T) {
	tests := []struct {
		version string
		want    int
		ok      bool
	}{
		{"5.17", 53, true},
		{"5.17.0", 53, true},
		{"5.17.1", 53, true},
		{"5.18-dev", 54, true},
		{"5.18.0-dev-abc123", 54, true},
		{"5.17.0-rc1", 53, true}, // no 5.17-dev listed, falls back to the release
		{"5.18", 0, false},
		{"5", 0, false},
		{"", 0, false},
	}
	for _, tt := range tests {
		got, err := MatchVersion(cdbVersions, tt.version)
		if got != tt.want || (err == nil) != tt.ok {
			t.Errorf("MatchVersion(%q) = %d, %v; want %d, ok=%v", tt.version, got, err, tt.want, tt.ok)
		}
	}
}

func TestNewestEntry(t *testing.T) {
	v, p, ok := newestEntry(map[string]int{"5.2.0": 39, "5.4.0": 39, "5.3.0": 39, "5.10.0": 46, "5.9.1": 45})
	if v != "5.10.0" || p != 46 || !ok {
		t.Errorf("newestEntry = %q, %d, %v; want 5.10.0, 46, true", v, p, ok)
	}
	v, _, _ = newestEntry(map[string]int{"5.2.0": 39, "5.4.0": 39, "5.3.0": 39})
	if v != "5.4.0" {
		t.Errorf("newestEntry on a protocol tie = %q, want 5.4.0", v)
	}
}

func TestDetectFullInstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as a stand-in binary")
	}

	dir := newInstall(t, "Luanti 5.17.0 (Linux)\nUsing LuaJIT 2.1")
	info, warn, fatal := Detect(Options{Dir: dir})
	if fatal != nil || warn != nil {
		t.Fatalf("warn = %v, fatal = %v, want both nil", warn, fatal)
	}
	if info.Version != "5.17.0" || info.Protocol != 53 {
		t.Errorf("info = %+v, want 5.17.0 / 53", info)
	}
}
