package update

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/luanti-org/luma/internal/contentdb"
	"github.com/luanti-org/luma/internal/util"
)

// DownloadPackage fetches the zip at downloadURL and installs it into destDir,
// replacing any existing content there.
// destDir is only touched once download and extraction both succeed,
// temp files are always cleaned up, even on failure.
func DownloadPackage(client *contentdb.Client, downloadURL string, destDir string) error {
	tmpZip, err := os.CreateTemp("", "luma-download-*.zip")
	if err != nil {
		return err
	}
	tmpZipPath := tmpZip.Name()
	defer os.Remove(tmpZipPath)

	if err := client.Download(downloadURL, tmpZip); err != nil {
		tmpZip.Close()
		return err
	}
	if err := tmpZip.Close(); err != nil {
		return err
	}

	tmpExtractDir, err := os.MkdirTemp("", "luma-extract-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpExtractDir)

	if err := unzip(tmpZipPath, tmpExtractDir); err != nil {
		return err
	}

	extractedDir, err := stripSingleTopLevelDir(tmpExtractDir)
	if err != nil {
		return err
	}

	if err := os.RemoveAll(destDir); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destDir), 0o755); err != nil {
		return err
	}

	return moveDir(extractedDir, destDir)
}

// unzip extracts the zip archive at zipPath into destDir.
func unzip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if err := extractZipFile(f, destDir); err != nil {
			return fmt.Errorf("extract %s: %w", f.Name, err)
		}
	}

	return nil
}

func extractZipFile(f *zip.File, destDir string) error {
	path := filepath.Join(destDir, f.Name)

	// guard against zip slip: reject entries that escape destDir
	if !strings.HasPrefix(path, filepath.Clean(destDir)+string(os.PathSeparator)) {
		return fmt.Errorf("illegal file path %q", f.Name)
	}

	if f.FileInfo().IsDir() {
		return os.MkdirAll(path, 0o755)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	src, err := f.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

// manifestFiles are the files the Luanti client checks for a package root
var manifestFiles = []string{"init.lua", "modpack.conf", "modpack.txt", "game.conf", "texture_pack.conf"}

// hasManifest reports whether dir directly contains one of manifestFiles
func hasManifest(dir string) bool {
	for _, name := range manifestFiles {
		if util.FileExists(filepath.Join(dir, name)) {
			return true
		}
	}
	return false
}

// stripSingleTopLevelDir mirrors the Luanti client:
// dir is the root if it already has a conf/init file
func stripSingleTopLevelDir(dir string) (string, error) {
	if hasManifest(dir) {
		return dir, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}

	if len(entries) != 1 || !entries[0].IsDir() {
		return dir, nil
	}

	sub := filepath.Join(dir, entries[0].Name())
	if !hasManifest(sub) {
		return dir, nil
	}

	return sub, nil
}

// moveDir moves srcDir to become destDir, falling back to a copy if rename fails
func moveDir(srcDir, destDir string) error {
	if err := os.Rename(srcDir, destDir); err == nil {
		return nil
	}

	if err := copyDir(srcDir, destDir); err != nil {
		return err
	}

	return os.RemoveAll(srcDir)
}

func copyDir(srcDir, destDir string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destDir, rel)

		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		return copyFile(path, target, info.Mode())
	})
}

func copyFile(srcPath, destPath string, mode os.FileMode) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}
