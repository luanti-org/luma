package engine

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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
	_, warn, fatal := Detect(filepath.Join(t.TempDir(), "nope"), false)
	if fatal == nil || !strings.Contains(fatal.Error(), "is not a directory") {
		t.Errorf("fatal = %v, want a not-a-directory error", fatal)
	}
	if warn != nil {
		t.Errorf("warn = %v, want nil", warn)
	}
}

func TestDetectEmptyDir(t *testing.T) {
	dir := t.TempDir()
	_, _, fatal := Detect(dir, false)
	if fatal == nil || !strings.Contains(fatal.Error(), "no Luanti install in "+dir) {
		t.Errorf("fatal = %v, want a no-install error naming %s", fatal, dir)
	}
}

func TestDetectEmptyCurrentDir(t *testing.T) {
	t.Chdir(t.TempDir())
	_, _, fatal := Detect("", false)
	if fatal == nil || !strings.Contains(fatal.Error(), "the current directory") {
		t.Errorf("fatal = %v, want it to name the current directory", fatal)
	}
}

func TestDetectDataOnlyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "mods"), 0o755); err != nil {
		t.Fatal(err)
	}

	info, warn, fatal := Detect(dir, false)
	if fatal != nil {
		t.Fatalf("fatal = %v, want nil", fatal)
	}
	if !errors.Is(warn, ErrNoBinary) {
		t.Errorf("warn = %v, want ErrNoBinary", warn)
	}
	if info.ModsDir == "" {
		t.Error("ModsDir is empty")
	}
}

func TestDetectFullInstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as a stand-in binary")
	}

	dir := newInstall(t, "Luanti 5.17.0 (Linux)\nUsing LuaJIT 2.1")
	info, warn, fatal := Detect(dir, false)
	if fatal != nil || warn != nil {
		t.Fatalf("warn = %v, fatal = %v, want both nil", warn, fatal)
	}
	if info.Version != "5.17.0" || info.Protocol != 53 {
		t.Errorf("info = %+v, want 5.17.0 / 53", info)
	}
}
