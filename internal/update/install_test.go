package update

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallRunRollback(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "existing")
	created := filepath.Join(dir, "created")
	failed := filepath.Join(dir, "failed")
	if err := os.Mkdir(existing, 0o755); err != nil {
		t.Fatal(err)
	}

	var run InstallRun
	mkdir := func(path string) func() error {
		return func() error { return os.MkdirAll(filepath.Join(path, "sub"), 0o755) }
	}

	if err := run.Step(existing, mkdir(existing)); err != nil {
		t.Fatal(err)
	}
	if err := run.Step(created, mkdir(created)); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("boom")
	err := run.Step(failed, func() error {
		mkdir(failed)()
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("Step error = %v, want %v", err, boom)
	}

	if err := run.Rollback(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(existing); err != nil {
		t.Errorf("folder that existed before the run was removed: %v", err)
	}
	for _, path := range []string{created, failed} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s still exists after Rollback", path)
		}
	}
}
