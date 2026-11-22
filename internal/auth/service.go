package auth

import (
	"bytes"
	"context"
	"errors"
	"io"

	"github.com/victorx98/sa2-cli/internal/api"
	"github.com/victorx98/sa2-cli/internal/session"
)

var ErrLoginRequired = errors.New("login required")

const (
	credentialVersion byte = 1
	statePending      byte = 0
	stateReady        byte = 1
)

type Service struct {
	API   api.Client
	Vault session.Vault
	Locks *session.Locker
}

func (s Service) Login(ctx context.Context, origin, email string, password func(context.Context) ([]byte, error)) (api.Identity, error) {
	return s.login(ctx, origin, func() (api.Result, error) {
		secret, err := password(ctx)
		defer clear(secret)
		if err != nil {
			return api.Result{}, err
		}
		return s.API.Login(ctx, origin, email, secret)
	})
}

func (s Service) FeishuLogin(ctx context.Context, origin string, out io.Writer) (api.Identity, error) {
	return s.login(ctx, origin, func() (api.Result, error) {
		code, err := waitForFeishuCode(ctx, origin, out)
		if err != nil {
			return api.Result{}, err
		}
		return s.API.FeishuLogin(ctx, origin, code, origin+"/auth")
	})
}

func (s Service) login(ctx context.Context, origin string, exchange func() (api.Result, error)) (api.Identity, error) {
	key := session.Key(origin)
	unlock, err := s.Locks.Acquire(ctx, key)
	if err != nil {
		return api.Identity{}, err
	}
	defer unlock()

	existing, err := s.Vault.Read(ctx, key)
	clear(existing)
	if err != nil && !errors.Is(err, session.ErrNotFound) {
		return api.Identity{}, session.ErrUnavailable
	}

	result, err := exchange()
	if err != nil {
		if ctx.Err() != nil {
			return api.Identity{}, ctx.Err()
		}
		return api.Identity{}, err
	}
	return s.save(context.WithoutCancel(ctx), key, result)
}

func (s Service) Whoami(ctx context.Context, origin string) (api.Identity, error) {
	key := session.Key(origin)
	unlock, err := s.Locks.Acquire(ctx, key)
	if err != nil {
		return api.Identity{}, err
	}
	defer unlock()

	current, err := s.Vault.Read(ctx, key)
	defer clear(current)
	pending := []byte{credentialVersion, statePending}
	if errors.Is(err, session.ErrNotFound) || err == nil && bytes.Equal(current, pending) {
		return api.Identity{}, ErrLoginRequired
	}
	if err != nil || len(current) < 3 || current[0] != credentialVersion || current[1] != stateReady {
		return api.Identity{}, session.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return api.Identity{}, err
	}
	if err := s.Vault.Write(ctx, key, pending); err != nil {
		return api.Identity{}, session.ErrUnavailable
	}

	// Once pending is committed, an ambiguous result must not restore the used token.
	result, err := s.API.Refresh(ctx, origin, current[2:])
	clear(current)
	if err != nil {
		return api.Identity{}, ErrLoginRequired
	}
	return s.save(context.WithoutCancel(ctx), key, result)
}

func (s Service) Logout(ctx context.Context, origin string) error {
	key := session.Key(origin)
	unlock, err := s.Locks.Acquire(ctx, key)
	if err != nil {
		return err
	}
	defer unlock()
	if err := s.Vault.Delete(ctx, key); err != nil && !errors.Is(err, session.ErrNotFound) {
		return session.ErrUnavailable
	}
	return nil
}

func (s Service) save(ctx context.Context, key string, result api.Result) (api.Identity, error) {
	defer clear(result.RefreshToken)
	record := make([]byte, 2+len(result.RefreshToken))
	record[0], record[1] = credentialVersion, stateReady
	copy(record[2:], result.RefreshToken)
	defer clear(record)
	if err := s.Vault.Write(ctx, key, record); err != nil {
		return api.Identity{}, session.ErrUnavailable
	}
	return result.Identity, nil
}
