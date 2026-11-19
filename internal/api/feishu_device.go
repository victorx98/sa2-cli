package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var (
	ErrDeviceClaimed = errors.New("device authorization already claimed")
	ErrDeviceExpired = errors.New("device authorization expired")
	ErrDeviceLimited = errors.New("device authorization rate limited")
	deviceCredential = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
	deviceUserCode   = regexp.MustCompile(`^[A-Za-z0-9 -]{1,64}$`)
)

type FeishuDeviceGrant struct {
	Credential      string `json:"credential"`
	VerificationURI string `json:"verificationUri"`
	UserCode        string `json:"userCode"`
	ExpiresIn       int    `json:"expiresIn"`
	Interval        int    `json:"interval"`
}

func (c Client) StartFeishuDevice(ctx context.Context, origin string) (FeishuDeviceGrant, error) {
	res, err := c.post(ctx, origin+"/api/auth/feishu/device/start", struct{}{})
	if err != nil {
		return FeishuDeviceGrant{}, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusTooManyRequests {
		return FeishuDeviceGrant{}, ErrDeviceLimited
	}
	if res.StatusCode != http.StatusOK {
		return FeishuDeviceGrant{}, ErrUnavailable
	}
	var grant FeishuDeviceGrant
	err = decodeDeviceResponse(res.Body, &grant)
	if err != nil || !deviceCredential.MatchString(grant.Credential) ||
		!safeFeishuVerificationURI(grant.VerificationURI) || !deviceUserCode.MatchString(grant.UserCode) ||
		grant.Interval < 1 || grant.ExpiresIn > 300 || grant.ExpiresIn <= grant.Interval {
		return FeishuDeviceGrant{}, ErrInvalidResponse
	}
	return grant, nil
}

func (c Client) PollFeishuDevice(ctx context.Context, origin, credential string) (Result, int, error) {
	res, err := c.post(ctx, origin+"/api/auth/feishu/device/poll", struct {
		Credential string `json:"credential"`
	}{Credential: credential})
	if err != nil {
		return Result{}, 0, err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK:
		result, err := decodeSession(res.Body)
		return result, 0, err
	case http.StatusAccepted:
		var pending struct {
			Interval int `json:"interval"`
		}
		if err := decodeDeviceResponse(res.Body, &pending); err != nil || pending.Interval < 1 || pending.Interval > 900 {
			return Result{}, 0, ErrInvalidResponse
		}
		return Result{}, pending.Interval, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return Result{}, 0, ErrRejected
	case http.StatusGone:
		return Result{}, 0, ErrDeviceExpired
	case http.StatusConflict:
		return Result{}, 0, ErrDeviceClaimed
	default:
		return Result{}, 0, ErrUnavailable
	}
}

func decodeDeviceResponse(body io.Reader, target any) error {
	raw, err := io.ReadAll(io.LimitReader(body, maxResponseSize+1))
	defer clear(raw)
	if err != nil || len(raw) > maxResponseSize || json.Unmarshal(raw, target) != nil {
		return ErrInvalidResponse
	}
	return nil
}

func safeFeishuVerificationURI(value string) bool {
	if strings.IndexFunc(value, func(r rune) bool { return r <= ' ' || r == 0x7f }) >= 0 {
		return false
	}
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	switch u.Host {
	case "accounts.feishu.cn", "passport.feishu.cn", "open.feishu.cn":
		return true
	default:
		return false
	}
}
