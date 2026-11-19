package session

import (
	"context"
	"errors"

	"github.com/danieljoos/wincred"
	"golang.org/x/sys/windows"
)

type windowsVault struct{}

func openNative(context.Context) (Vault, error) {
	return windowsVault{}, nil
}

func (windowsVault) Read(_ context.Context, key string) ([]byte, error) {
	credential, err := wincred.GetGenericCredential(credentialService + ":" + key)
	if err != nil {
		return nil, windowsVaultError(err)
	}
	return credential.CredentialBlob, nil
}

func (windowsVault) Write(_ context.Context, key string, value []byte) error {
	if len(value) > 5*512 {
		return ErrUnavailable
	}
	credential := wincred.NewGenericCredential(credentialService + ":" + key)
	credential.CredentialBlob = value
	credential.Persist = wincred.PersistLocalMachine
	return windowsVaultError(credential.Write())
}

func (windowsVault) Delete(_ context.Context, key string) error {
	return windowsVaultError(wincred.NewGenericCredential(credentialService + ":" + key).Delete())
}

func (windowsVault) Close() {}

func windowsVaultError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, wincred.ErrElementNotFound), errors.Is(err, windows.ERROR_NOT_FOUND):
		return ErrNotFound
	default:
		return ErrUnavailable
	}
}
