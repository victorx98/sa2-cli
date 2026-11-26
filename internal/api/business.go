package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

type BusinessError struct {
	Status int
	Code   string
}

func (e *BusinessError) Error() string { return e.Code }

var safeCode = regexp.MustCompile("^[A-Z][A-Z0-9_]{0,79}$")
var ErrOutcomeUnknown = errors.New("submission outcome unknown")
var ErrBookingAPINotDeployed = errors.New("This environment does not expose the required SA2 booking API.")

// Business makes exactly one authenticated call to a compiled-in route on the chosen origin.
func (c Client) Business(ctx context.Context, origin string, access []byte, method, path string, query url.Values, input any, headers map[string]string, output any) error {
	if len(access) == 0 {
		return ErrInvalidResponse
	}
	if !strings.HasPrefix(path, "/api/") || strings.ContainsAny(path, "?#\\") {
		return ErrInvalidRequest
	}
	var body io.Reader
	if input != nil {
		raw, err := json.Marshal(input)
		if err != nil {
			return ErrInvalidRequest
		}
		body = bytes.NewReader(raw)
	}
	target := origin + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return ErrInvalidRequest
	}
	req.Header.Set("Authorization", "Bearer "+string(access))
	req.Header.Set("X-SA2-Origin", origin)
	req.Header.Set("Accept", "application/json")
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	commit := path == "/api/services/session-bookings/commit"
	milestonePatch := method == "PATCH" && strings.HasPrefix(path, "/api/reports/students/") && strings.Contains(path, "/monthly-service-reports/") && strings.Contains(path, "/milestones/")
	milestoneCreate := method == "POST" && strings.HasPrefix(path, "/api/reports/students/") && strings.HasSuffix(path, "/milestones")
	uncertainWrite := commit || milestonePatch || milestoneCreate
	// Go may transparently replay requests carrying an idempotency key on a stale connection.
	if uncertainWrite {
		req.GetBody = nil
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		if uncertainWrite {
			return ErrOutcomeUnknown
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return ErrUnavailable
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponseSize+1))
	defer clear(raw)
	if err != nil || len(raw) > maxResponseSize {
		if uncertainWrite {
			return ErrOutcomeUnknown
		}
		return ErrInvalidResponse
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		if res.StatusCode == 401 {
			return ErrRejected
		}
		if uncertainWrite && (res.StatusCode >= 500 || res.StatusCode == 408) {
			return ErrOutcomeUnknown
		}
		var detail struct {
			Code      string          `json:"code"`
			ErrorCode string          `json:"errorCode"`
			Message   string          `json:"message"`
			Error     json.RawMessage `json:"error"`
		}
		_ = json.Unmarshal(raw, &detail)
		if res.StatusCode == http.StatusNotFound && strings.HasPrefix(path, "/api/services/session-bookings/") &&
			(detail.Message == "Cannot "+method+" "+path || detail.Message == "Cannot "+method+" "+req.URL.RequestURI()) {
			return ErrBookingAPINotDeployed
		}
		if (milestonePatch || milestoneCreate) && res.StatusCode == http.StatusNotFound && (detail.Message == "Cannot "+method+" "+path || detail.Message == "Cannot "+method+" "+req.URL.RequestURI()) {
			return &BusinessError{res.StatusCode, "MILESTONE_API_NOT_DEPLOYED"}
		}
		code := detail.Code
		if !safeCode.MatchString(code) {
			code = detail.ErrorCode
		}
		if !safeCode.MatchString(code) {
			code = detail.Message
		}
		if !safeCode.MatchString(code) {
			var nested struct {
				Code string `json:"code"`
			}
			_ = json.Unmarshal(detail.Error, &nested)
			code = nested.Code
		}
		if !safeCode.MatchString(code) {
			switch res.StatusCode {
			case 403:
				code = "FORBIDDEN"
			case 404:
				code = "NOT_FOUND"
			case 409:
				code = "CONFLICT"
			case 410:
				code = "PLAN_EXPIRED"
			case 412:
				code = "STALE_SESSION"
			default:
				code = "API_ERROR"
			}
		}
		return &BusinessError{res.StatusCode, code}
	}
	if err = json.Unmarshal(raw, output); err != nil {
		if uncertainWrite {
			return ErrOutcomeUnknown
		}
		return ErrInvalidResponse
	}
	return nil
}
