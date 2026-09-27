package engine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These use the real flatpak command and a real org.luanti.luanti install,
// since flatpak's sandboxing isn't practical to fake.
// They skip when either isn't present, e.g. in CI
func requireFlatpakApp(t *testing.T) {
	t.Helper()
	if _, err := flatpakLocation(DefaultFlatpakAppID); err != nil {
		t.Skipf("org.luanti.luanti not available via flatpak: %v", err)
	}
}

func TestFlatpakVersion(t *testing.T) {
	requireFlatpakApp(t)

	version, err := FlatpakVersion(DefaultFlatpakAppID)
	if err != nil {
		t.Fatalf("FlatpakVersion: %v", err)
	}
	if version == "" {
		t.Error("version is empty")
	}
}

func TestFlatpakProtocolTable(t *testing.T) {
	requireFlatpakApp(t)

	table, err := FlatpakProtocolTable(DefaultFlatpakAppID)
	if err != nil {
		t.Fatalf("FlatpakProtocolTable: %v", err)
	}
	if len(table) == 0 {
		t.Error("table is empty")
	}
}

func TestDetectFlatpak(t *testing.T) {
	requireFlatpakApp(t)

	info, err := DetectFlatpak(DefaultFlatpakAppID)
	if err != nil {
		t.Fatalf("DetectFlatpak: %v", err)
	}
	if info.Version == "" || info.Protocol == 0 {
		t.Errorf("info = %+v, want both fields set", info)
	}

	home, _ := os.UserHomeDir()
	wantMods := filepath.Join(home, ".var", "app", DefaultFlatpakAppID, ".minetest", "mods")
	if info.ModsDir != wantMods {
		t.Errorf("ModsDir = %q, want %q", info.ModsDir, wantMods)
	}
}

func TestDetectFlatpakUnknownApp(t *testing.T) {
	if _, err := exec.LookPath("flatpak"); err != nil {
		t.Skip("flatpak not installed")
	}

	// content scanning shouldn't depend on the app actually being installed
	info, err := DetectFlatpak("org.example.doesnotexist")
	if err == nil {
		t.Error("expected error for an app that isn't installed")
	}
	if !strings.Contains(info.ModsDir, "org.example.doesnotexist") {
		t.Errorf("ModsDir = %q, want it to still be set for the given app ID", info.ModsDir)
	}
}

func TestDetectFallsBackToFlatpak(t *testing.T) {
	requireFlatpakApp(t)

	// an empty dir has no bin/, so Detect should fall back to flatpak
	info, err := Detect(t.TempDir())
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if info.Version == "" {
		t.Error("info.Version is empty")
	}
}
