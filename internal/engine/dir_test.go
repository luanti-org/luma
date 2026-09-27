package engine

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func newInstall(t *testing.T, versionOutput string) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "builtin", "game", "misc_s.lua"), miscS, 0o644)
	if versionOutput != "" {
		script := "#!/bin/sh\ncat <<'EOF'\n" + versionOutput + "\nEOF\n"
		writeFile(t, filepath.Join(dir, "bin", "luanti"), script, 0o755)
	}
	return dir
}

func TestReadProtocolTable(t *testing.T) {
	dir := newInstall(t, "")

	table, err := ReadProtocolTable(dir)
	if err != nil {
		t.Fatalf("ReadProtocolTable: %v", err)
	}
	if table["5.17.0"] != 53 {
		t.Errorf("table[5.17.0] = %d, want 53", table["5.17.0"])
	}
}

func TestReadProtocolTableMissing(t *testing.T) {
	if _, err := ReadProtocolTable(t.TempDir()); err == nil {
		t.Error("expected error for missing misc_s.lua")
	}
}

func TestDetectDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as a stand-in binary")
	}

	dir := newInstall(t, "Luanti 5.17.0 (Linux)\nUsing LuaJIT 2.1")
	info, err := DetectDir(dir)
	if err != nil {
		t.Fatalf("DetectDir: %v", err)
	}
	if info.Version != "5.17.0" || info.Protocol != 53 {
		t.Errorf("info = %+v, want 5.17.0 / 53", info)
	}
	if info.ModsDir != filepath.Join(dir, "mods") || info.GamesDir != filepath.Join(dir, "games") {
		t.Errorf("info = %+v, want ModsDir/GamesDir under %s", info, dir)
	}
}

func TestDetectDirDevBuild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as a stand-in binary")
	}

	dir := newInstall(t, "Luanti 5.18.0-dev-abc123 (Linux)")
	info, err := DetectDir(dir)
	if err != nil {
		t.Fatalf("DetectDir: %v", err)
	}
	if info.Version != "5.18.0-dev-abc123" || info.Protocol != 53 {
		t.Errorf("info = %+v, want version set and protocol 53 (newest in table)", info)
	}
}

func TestDetectDirNoBinary(t *testing.T) {
	dir := newInstall(t, "")
	info, err := DetectDir(dir)
	if !errors.Is(err, ErrNoBinary) {
		t.Errorf("err = %v, want ErrNoBinary", err)
	}
	// content scanning shouldn't depend on the engine binary being found
	if info.ModsDir != filepath.Join(dir, "mods") || info.GamesDir != filepath.Join(dir, "games") {
		t.Errorf("info = %+v, want ModsDir/GamesDir still set under %s", info, dir)
	}
}

func TestDetectPrefersDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as a stand-in binary")
	}

	dir := newInstall(t, "Luanti 5.17.0 (Linux)\nUsing LuaJIT 2.1")
	info, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if info.Version != "5.17.0" || info.Protocol != 53 {
		t.Errorf("info = %+v, want 5.17.0 / 53 (from dir, not flatpak)", info)
	}
}
