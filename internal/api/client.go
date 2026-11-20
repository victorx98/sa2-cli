package api

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"time"
)

var (
	ErrRejected        = errors.New("authentication rejected")
	ErrInvalidRequest  = errors.New("invalid request")
	ErrUnavailable     = errors.New("API unavailable")
	ErrInvalidResponse = errors.New("invalid authentication response")
)

const maxResponseSize = 1 << 20

type Identity struct {
	ID    string   `json:"id"`
	Email *string  `json:"email"`
	Roles []string `json:"roles"`
}

type Result struct {
	Identity     Identity
	RefreshToken []byte
}

type Client struct {
	HTTP *http.Client
}

func NewHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
			TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
			TLSHandshakeTimeout: 5 * time.Second,
			DisableKeepAlives:   true,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (c Client) Login(ctx context.Context, origin, email string, password []byte) (Result, error) {
	return c.exchange(ctx, origin+"/api/auth/login", struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}{Email: email, Password: string(password)})
}

func (c Client) Refresh(ctx context.Context, origin string, refreshToken []byte) (Result, error) {
	return c.exchange(ctx, origin+"/api/auth/refresh", struct {
		RefreshToken string `json:"refreshToken"`
	}{RefreshToken: string(refreshToken)})
}

func (c Client) exchange(ctx context.Context, url string, input any) (Result, error) {
	res, err := c.post(ctx, url, input)
	if err != nil {
		return Result{}, err
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusOK:
		return decodeSession(res.Body)
	case http.StatusBadRequest:
		return Result{}, ErrInvalidRequest
	case http.StatusUnauthorized, http.StatusForbidden:
		return Result{}, ErrRejected
	default:
		return Result{}, ErrUnavailable
	}
}

func (c Client) post(ctx context.Context, url string, input any) (*http.Response, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, ErrInvalidRequest
	}
	req.GetBody = nil
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cache-Control", "no-store")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, ErrUnavailable
	}
	return res, nil
}

func decodeSession(body io.Reader) (Result, error) {
	raw, err := io.ReadAll(io.LimitReader(body, maxResponseSize+1))
	defer clear(raw)
	if err != nil || len(raw) > maxResponseSize {
		return Result{}, ErrInvalidResponse
	}
	var response struct {
		RefreshToken string `json:"refreshToken"`
		User         struct {
			ID    string  `json:"id"`
			Email *string `json:"email"`
			Roles []struct {
				Name string `json:"enName"`
			} `json:"roles"`
		} `json:"user"`
	}
	if err := json.Unmarshal(raw, &response); err != nil || response.RefreshToken == "" || response.User.ID == "" {
		return Result{}, ErrInvalidResponse
	}
	roles := make([]string, 0, len(response.User.Roles))
	for _, role := range response.User.Roles {
		if role.Name != "" {
			roles = append(roles, role.Name)
		}
	}
	return Result{
		Identity:     Identity{ID: response.User.ID, Email: response.User.Email, Roles: roles},
		RefreshToken: []byte(response.RefreshToken),
	}, nil
}
