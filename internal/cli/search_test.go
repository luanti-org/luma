package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchInstalledFirst(t *testing.T) {
	eng, _, cdb := setupInstall(t)
	writeFile(t, filepath.Join(eng.ModsDir, "thing", "mod.conf"), "name = thing\nauthor = jane\nrelease = 5\n")
	writeFile(t, filepath.Join(eng.ModsDir, "thing", "init.lua"), "")

	code, stdout, stderr := runWithCDB(t, eng, cdb, "search")
	if code != exitOK || stderr != "" {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	checkTable(t, stdout, [][]string{
		{"TITLE", "PACKAGE", "TYPE"},
		{"[U]", "Thing", "jane/thing", "mod"},
		{"Lib", "amy/lib", "mod"},
		{"Quest", "Land", "(formerly", "Questy)", "jane/quest", "game"},
		{"Pixels", "jane/pixels", "txp"},
		{"Dupe", "bob/dupe", "mod"},
		{"Dupe", "sam/dupe", "mod"},
	})
}

func TestSearchWordsAndType(t *testing.T) {
	eng, _, cdb := setupInstall(t)

	code, stdout, stderr := runWithCDB(t, eng, cdb, "search", "-t", "m", "JANE/", "th")
	if code != exitOK || stderr != "" {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	checkTable(t, stdout, [][]string{
		{"TITLE", "PACKAGE"},
		{"Thing", "jane/thing"},
	})
}

func TestSearchNames(t *testing.T) {
	eng, _, cdb := setupInstall(t)

	code, stdout, _ := runWithCDB(t, eng, cdb, "search", "--names", "--type", "texturepack")
	if code != exitOK || stdout != "jane/pixels\n" {
		t.Errorf("code = %d, stdout = %q", code, stdout)
	}

	code, stdout, _ = runWithCDB(t, eng, cdb, "search", "--names", "nothing")
	if code != exitOK || stdout != "" {
		t.Errorf("no match: code = %d, stdout = %q", code, stdout)
	}
}

func TestSearchNoMatch(t *testing.T) {
	eng, _, cdb := setupInstall(t)

	code, stdout, _ := runWithCDB(t, eng, cdb, "search", "nothing")
	if code != exitOK || stdout != "No packages found.\n" {
		t.Errorf("code = %d, stdout = %q", code, stdout)
	}
}

func TestSearchUnknownType(t *testing.T) {
	eng, _, cdb := setupInstall(t)

	code, _, stderr := runWithCDB(t, eng, cdb, "search", "-t", "world")
	if code != exitUsage || !strings.Contains(stderr, `unknown type "world"`) {
		t.Errorf("code = %d, stderr = %q", code, stderr)
	}
}

func TestSearchUsesCacheUnlessRefresh(t *testing.T) {
	eng, f, cdb := setupInstall(t)

	for _, args := range [][]string{{"search"}, {"search", "lib"}, {"search", "-r"}} {
		if code, _, stderr := runWithCDB(t, eng, cdb, args...); code != exitOK {
			t.Fatalf("%v: code = %d, stderr = %q", args, code, stderr)
		}
	}
	if f.listRequests != 2 {
		t.Errorf("list requests = %d, want 2", f.listRequests)
	}
}
