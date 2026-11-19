//go:build linux || darwin

package session

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestUnsafeLockPaths(t *testing.T) {
	temp := t.TempDir()
	private := filepath.Join(temp, "private")
	public := filepath.Join(temp, "public")
	symlink := filepath.Join(temp, "symlink")
	key := Key("https://test.example")
	if err := os.Mkdir(private, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(public, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(public, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(private, symlink); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(temp, "other-file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(file, filepath.Join(private, key+".lock")); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{private, public, symlink} {
		if unlock, err := NewLocker(dir).Acquire(t.Context(), key); !errors.Is(err, ErrUnsafeLock) {
			if unlock != nil {
				unlock()
			}
			t.Fatal("unsafe lock directory or entry accepted", err)
		}
	}
}
