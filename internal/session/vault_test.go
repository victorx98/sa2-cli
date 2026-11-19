package session

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNativeVault(t *testing.T) {
	if os.Getenv("SA2_TEST_NATIVE_VAULT") != "1" {
		t.Skip("requires a provisioned test credential store")
	}
	ctx := t.Context()
	vault, err := openNative(ctx)
	if err != nil {
		t.Fatal("could not open the test credential store", err)
	}
	key := Key(t.TempDir())
	defer func() {
		_ = vault.Delete(ctx, key)
		vault.Close()
	}()

	if _, err := vault.Read(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatal("expected a missing test entry", err)
	}
	if err := vault.Write(ctx, key, []byte{1, 1, 'a'}); err != nil {
		t.Fatal("create credential", err)
	}
	if err := vault.Write(ctx, key, []byte{1, 0}); err != nil {
		t.Fatal("replace with pending", err)
	}
	if value, err := vault.Read(ctx, key); err != nil || !bytes.Equal(value, []byte{1, 0}) {
		t.Fatal("expected pending without the original secret", err)
	}
	if err := vault.Write(ctx, key, []byte{1, 1, 'b'}); err != nil {
		t.Fatal("replace pending with the new credential", err)
	}

	second, err := openNative(ctx)
	if err != nil {
		t.Fatal("reopen credential store", err)
	}
	defer second.Close()
	if value, err := second.Read(ctx, key); err != nil || !bytes.Equal(value, []byte{1, 1, 'b'}) {
		t.Fatal("new credential did not persist across store connections", err)
	}
	if err := second.Delete(ctx, key); err != nil {
		t.Fatal("delete credential", err)
	}
	if _, err := vault.Read(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatal("credential still present after deletion", err)
	}
}

func TestIndependentOriginLocks(t *testing.T) {
	locker := NewLocker(filepath.Join(t.TempDir(), "locks"))
	key := Key("https://one.test")
	unlock, err := locker.Acquire(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	secondKey := Key("https://two.test")
	unlockOther, err := locker.Acquire(t.Context(), secondKey)
	if err != nil {
		t.Fatal(err)
	}
	unlockOther()
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	if _, err := locker.Acquire(ctx, key); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("duplicate lock was not blocked", err)
	}
	unlock()
	unlock, err = locker.Acquire(t.Context(), key)
	if err != nil {
		t.Fatal("lock was not released on close", err)
	}
	unlock()
}
