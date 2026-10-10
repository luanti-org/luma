package update

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/luanti-org/luma/internal/contentdb"
)

// buildZip returns zip bytes with the given files, each wrapped under a
// single top-level directory -- the shape ContentDB release zips use.
func buildZip(t *testing.T, topLevelDir string, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	for name, content := range files {
		f, err := w.Create(filepath.Join(topLevelDir, name))
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	return buf.Bytes()
}

func newDownloadTestServer(t *testing.T, zipData []byte) (*contentdb.Client, string) {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(zipData)
	}))
	t.Cleanup(srv.Close)

	return contentdb.New(srv.URL), srv.URL + "/packages/jane/mymod/download/"
}

func TestDownloadPackageInstallsIntoDestDir(t *testing.T) {
	zipData := buildZip(t, "jane-mymod-abc123", map[string]string{
		"mod.conf": "name = mymod\n",
		"init.lua": "-- hi\n",
	})
	client, downloadURL := newDownloadTestServer(t, zipData)

	destDir := filepath.Join(t.TempDir(), "mymod")

	if err := DownloadPackage(client, downloadURL, destDir); err != nil {
		t.Fatalf("DownloadPackage: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(destDir, "mod.conf"))
	if err != nil {
		t.Fatalf("reading mod.conf: %v", err)
	}
	if string(data) != "name = mymod\n" {
		t.Errorf("mod.conf = %q, want %q", data, "name = mymod\n")
	}

	if _, err := os.Stat(filepath.Join(destDir, "init.lua")); err != nil {
		t.Errorf("init.lua missing: %v", err)
	}

	// the top-level wrapper dir must not have leaked into destDir
	if _, err := os.Stat(filepath.Join(destDir, "jane-mymod-abc123")); !os.IsNotExist(err) {
		t.Errorf("expected top-level dir to be stripped, got err=%v", err)
	}
}

func TestDownloadPackageReplacesExistingDestDir(t *testing.T) {
	zipData := buildZip(t, "jane-mymod-def456", map[string]string{
		"mod.conf": "name = mymod\nrelease = 2\n",
		"init.lua": "-- hi\n",
	})
	client, downloadURL := newDownloadTestServer(t, zipData)

	destDir := filepath.Join(t.TempDir(), "mymod")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(destDir, "old.lua"), []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := DownloadPackage(client, downloadURL, destDir); err != nil {
		t.Fatalf("DownloadPackage: %v", err)
	}

	if _, err := os.Stat(filepath.Join(destDir, "old.lua")); !os.IsNotExist(err) {
		t.Errorf("expected old.lua to be removed, got err=%v", err)
	}

	data, err := os.ReadFile(filepath.Join(destDir, "mod.conf"))
	if err != nil {
		t.Fatalf("reading mod.conf: %v", err)
	}
	if string(data) != "name = mymod\nrelease = 2\n" {
		t.Errorf("mod.conf = %q", data)
	}
}

func TestDownloadPackageCleansUpTempFiles(t *testing.T) {
	zipData := buildZip(t, "jane-mymod-ghi789", map[string]string{
		"mod.conf": "name = mymod\n",
		"init.lua": "-- hi\n",
	})
	client, downloadURL := newDownloadTestServer(t, zipData)

	destDir := filepath.Join(t.TempDir(), "mymod")

	// a private temp dir, the system one is shared with tests running in parallel
	tmpDir := t.TempDir()
	for _, env := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(env, tmpDir)
	}

	if err := DownloadPackage(client, downloadURL, destDir); err != nil {
		t.Fatalf("DownloadPackage: %v", err)
	}

	left, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("ReadDir tmp: %v", err)
	}
	for _, entry := range left {
		t.Errorf("leaked temp entry %s", entry.Name())
	}
}

func TestDownloadPackagePropagatesDownloadError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	client := contentdb.New(srv.URL)

	destDir := filepath.Join(t.TempDir(), "mymod")

	if err := DownloadPackage(client, srv.URL+"/packages/jane/mymod/download/", destDir); err == nil {
		t.Fatal("DownloadPackage: expected an error, got nil")
	}

	if _, err := os.Stat(destDir); !os.IsNotExist(err) {
		t.Errorf("destDir should not have been created on failure, got err=%v", err)
	}
}

func TestDownloadPackageDoesNotStripUnwrappedLoneSubdir(t *testing.T) {
	// no manifest anywhere -- "textures" is a lone top-level dir but not
	// a wrapper, so it must land in destDir as-is, not be stripped away.
	zipData := buildZip(t, "textures", map[string]string{
		"leaves.png": "not really a png",
	})
	client, downloadURL := newDownloadTestServer(t, zipData)

	destDir := filepath.Join(t.TempDir(), "mymod")

	if err := DownloadPackage(client, downloadURL, destDir); err != nil {
		t.Fatalf("DownloadPackage: %v", err)
	}

	if _, err := os.Stat(filepath.Join(destDir, "textures", "leaves.png")); err != nil {
		t.Errorf("expected textures/leaves.png to survive unstripped, got err=%v", err)
	}
}

func TestDownloadPackageRejectsZipSlip(t *testing.T) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.Create("../../evil.txt")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := f.Write([]byte("pwned")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	client, downloadURL := newDownloadTestServer(t, buf.Bytes())
	destDir := filepath.Join(t.TempDir(), "mymod")

	if err := DownloadPackage(client, downloadURL, destDir); err == nil {
		t.Fatal("DownloadPackage: expected zip slip to be rejected, got nil error")
	}
}
