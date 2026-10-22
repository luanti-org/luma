package update

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/luanti-org/luma/internal/contentdb"
	"github.com/luanti-org/luma/internal/engine"
	"github.com/luanti-org/luma/internal/util"
)

// FindPackages returns the packages arg names in packages, the index from FetchPackageIndex.
// An "author/name" arg names at most one package, a bare name every package with that name, sorted by id.
func FindPackages(packages map[string]contentdb.Package, arg string) []contentdb.Package {
	if strings.Contains(arg, "/") {
		if pkg, ok := packages[packageKey(arg)]; ok {
			return []contentdb.Package{pkg}
		}
		return nil
	}

	var found []contentdb.Package
	for _, pkg := range packages {
		if strings.EqualFold(pkg.Name, arg) {
			found = append(found, pkg)
		}
	}

	slices.SortFunc(found, func(a, b contentdb.Package) int {
		return strings.Compare(packageKey(a.Author+"/"+a.Name), packageKey(b.Author+"/"+b.Name))
	})

	return found
}

// SuggestPackage returns the one package whose title matches name, ignoring case, spaces, punctuation
// and a trailing parenthetical like "(formerly MineClone2)".
func SuggestPackage(packages map[string]contentdb.Package, name string) (contentdb.Package, bool) {
	want := titleKey(name)
	if want == "" {
		return contentdb.Package{}, false
	}

	var match contentdb.Package
	count := 0
	for _, pkg := range packages {
		short, _, _ := strings.Cut(pkg.Title, "(")
		if titleKey(pkg.Title) == want || titleKey(short) == want {
			match = pkg
			count++
		}
	}

	return match, count == 1
}

func titleKey(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, s)
}

// PackageDir returns the folder pkg installs to, its name under the folder for its type.
func PackageDir(eng engine.Info, pkg contentdb.Package) (string, error) {
	// the name comes from the network and becomes a folder name
	if !packageNameRe.MatchString(pkg.Name) {
		return "", fmt.Errorf("invalid package name %q", pkg.Name)
	}

	var dir string
	switch pkg.Type {
	case "mod":
		dir = eng.ModsDir
	case "game":
		dir = eng.GamesDir
	case "txp":
		dir = eng.TexturesDir
	default:
		return "", fmt.Errorf("unsupported package type %q", pkg.Type)
	}

	return filepath.Join(dir, pkg.Name), nil
}

// InstallPackage installs pkg into a new folder from PackageDir, never replacing an existing one.
func InstallPackage(client *contentdb.Client, eng engine.Info, pkg contentdb.Package) error {
	dest, err := PackageDir(eng, pkg)
	if err != nil {
		return err
	}

	return installNew(client, pkg, dest, contentdb.ReasonNew)
}

// InstallRun tracks the folders a run of installs created, so a failed run can be undone.
type InstallRun struct {
	created []string
}

// Step runs install, which installs into dest, and remembers dest if install created it.
func (r *InstallRun) Step(dest string, install func() error) error {
	_, statErr := os.Stat(dest)
	err := install()
	if _, after := os.Stat(dest); statErr != nil && after == nil {
		r.created = append(r.created, dest)
	}
	return err
}

// Rollback removes every folder the run created, trying all of them even if one fails.
func (r *InstallRun) Rollback() error {
	var errs []error
	for _, dir := range r.created {
		if err := os.RemoveAll(dir); err != nil {
			errs = append(errs, fmt.Errorf("removing %s: %w", dir, err))
		}
	}
	r.created = nil
	return errors.Join(errs...)
}

// installNew installs pkg into dest, which must not exist, and writes the conf fields the Luanti client writes.
func installNew(client *contentdb.Client, pkg contentdb.Package, dest, reason string) error {
	// the downloads below replace their target, which here would be a folder luma didn't put there
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("folder %s already exists", dest)
	}

	release := strconv.Itoa(pkg.Release)

	switch pkg.Type {
	case "game":
		url := client.ReleaseDownloadURL(pkg.Author, pkg.Name, pkg.Release, reason)
		if err := DownloadPackage(client, url, dest); err != nil {
			return err
		}
		// no `name` here, in game.conf it is a deprecated synonym for title
		return util.SetConfFields(filepath.Join(dest, "game.conf"), map[string]string{
			"author":  pkg.Author,
			"release": release,
		})

	case "txp":
		url := client.ReleaseDownloadURL(pkg.Author, pkg.Name, pkg.Release, reason)
		if err := DownloadPackage(client, url, dest); err != nil {
			return err
		}
		return util.SetConfFields(filepath.Join(dest, "texture_pack.conf"), map[string]string{
			"name":    pkg.Name,
			"author":  pkg.Author,
			"release": release,
		})

	default:
		return InstallMod(client, pkg.Author, pkg.Name, pkg.Release, dest, reason)
	}
}
