package session

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

var (
	ErrNotFound    = errors.New("credential not found")
	ErrUnavailable = errors.New("credential store unavailable")
	ErrUnsafeLock  = errors.New("invalid session lock")
	ErrBusy        = errors.New("session busy")
)

const credentialService = "sa2-cli.session.v1"

type Vault interface {
	Read(context.Context, string) ([]byte, error)
	Write(context.Context, string, []byte) error
	Delete(context.Context, string) error
	Close()
}

func Key(origin string) string {
	hash := sha256.Sum256([]byte(origin))
	return hex.EncodeToString(hash[:])
}

func Open(ctx context.Context) (Vault, *Locker, error) {
	dir, err := stateDirectory()
	if err != nil {
		return nil, nil, ErrUnsafeLock
	}
	vault, err := openNative(ctx)
	if err != nil {
		return nil, nil, ErrUnavailable
	}
	return vault, NewLocker(dir), nil
}
