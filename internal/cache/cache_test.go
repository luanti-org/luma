package cache

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"
)

func TestDirFromEnv(t *testing.T) {
	t.Setenv("LUMA_CACHE_DIR", "/some/dir")

	if dir, err := Dir(); err != nil || dir != "/some/dir" {
		t.Errorf("Dir() = %q, %v", dir, err)
	}
}

func TestDirDefault(t *testing.T) {
	t.Setenv("LUMA_CACHE_DIR", "")
	base, err := os.UserCacheDir()
	if err != nil {
		t.Skip("no user cache dir:", err)
	}

	if dir, err := Dir(); err != nil || dir != filepath.Join(base, "luma") {
		t.Errorf("Dir() = %q, %v", dir, err)
	}
}

func TestSaveGet(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "not", "made", "yet")
	t.Setenv("LUMA_CACHE_DIR", dir)

	if data := Get("https://example.com/a?x=1", time.Hour); data != nil {
		t.Fatalf("hit before anything was saved: %q", data)
	}

	if err := Save("https://example.com/a?x=1", []byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := Save("https://example.com/a?x=2", []byte("two")); err != nil {
		t.Fatal(err)
	}
	if err := Save("https://example.com/a?x=1", []byte("three")); err != nil {
		t.Fatal(err)
	}

	if data := Get("https://example.com/a?x=1", time.Hour); string(data) != "three" {
		t.Errorf("Get = %q", data)
	}
	if data := Get("https://example.com/a?x=2", time.Hour); string(data) != "two" {
		t.Errorf("other key = %q", data)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("%d files in the cache dir, want 2 with no temp files left", len(entries))
	}
	info, err := entries[0].Info()
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v, want 0644", info.Mode())
	}
}

func TestGetMaxAge(t *testing.T) {
	t.Setenv("LUMA_CACHE_DIR", t.TempDir())
	if err := Save("k", []byte("x")); err != nil {
		t.Fatal(err)
	}
	p, err := path("k")
	if err != nil {
		t.Fatal(err)
	}

	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	if data := Get("k", 3*time.Hour); string(data) != "x" {
		t.Errorf("younger than maxAge: Get = %q", data)
	}
	if data := Get("k", time.Hour); data != nil {
		t.Errorf("older than maxAge: Get = %q", data)
	}

	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(p, future, future); err != nil {
		t.Fatal(err)
	}
	if data := Get("k", 3*time.Hour); data != nil {
		t.Errorf("saved in the future: Get = %q", data)
	}
}

func TestSaveUnwritableDir(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LUMA_CACHE_DIR", filepath.Join(file, "sub"))

	if err := Save("k", []byte("x")); err == nil {
		t.Error("Save into a path under a file should fail")
	}
	if data := Get("k", time.Hour); data != nil {
		t.Errorf("Get = %q", data)
	}
}

func TestJSON(t *testing.T) {
	t.Setenv("LUMA_CACHE_DIR", t.TempDir())

	want := map[string][]string{"mvps": {"jeija/mesecons"}}
	if err := SaveJSON("k", want); err != nil {
		t.Fatal(err)
	}
	var got map[string][]string
	if !GetJSON("k", time.Hour, &got) || !reflect.DeepEqual(got, want) {
		t.Errorf("GetJSON = %v", got)
	}

	if err := Save("k", []byte("{not json")); err != nil {
		t.Fatal(err)
	}
	if GetJSON("k", time.Hour, &got) {
		t.Error("a file that doesn't decode should be a miss")
	}
	if GetJSON("missing", time.Hour, &got) {
		t.Error("a missing key should be a miss")
	}
}
