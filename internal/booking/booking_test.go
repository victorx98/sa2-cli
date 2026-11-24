package booking

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/victorx98/sa2-cli/internal/api"
	"github.com/victorx98/sa2-cli/internal/session"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func reply(code int, body any) (*http.Response, error) {
	b, _ := json.Marshal(body)
	return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(string(b))), Header: make(http.Header)}, nil
}

const fakeID = "11111111-1111-4111-8111-111111111111"

func TestListUsesDocumentedFilters(t *testing.T) {
	cmd, err := Parse([]string{"list", "--from", "2027-11-01", "--to", "2027-11-08", "--status", "scheduled", "--page-size", "25"})
	if err != nil {
		t.Fatal(err)
	}
	c := Client{Origin: "https://booking.invalid", Token: []byte("memory-only")}
	c.API = api.Client{HTTP: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		q := r.URL.Query()
		if q.Get("startDate") != "2027-11-01" || q.Get("endDate") != "2027-11-08" || q.Get("status") != "scheduled" || q.Get("pageSize") != "25" {
			t.Fatal(q)
		}
		return reply(200, map[string]any{"data": []any{}, "page": 1, "pageSize": 25, "total": 0, "totalPages": 1})
	})}}
	if _, err := c.Run(context.Background(), cmd); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse([]string{"list", "--page-size", "101"}); err == nil {
		t.Fatal("accepted oversized page")
	}
}

func TestPreviewCommitAndUnknownOutcome(t *testing.T) {
	p := filepath.Join(t.TempDir(), "booking.plan")
	previewID := ""
	commits := 0
	c := Client{Origin: "https://booking.invalid", Actor: "counselor", Token: []byte("memory-only")}
	c.API = api.Client{HTTP: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer memory-only" || r.Header.Get("X-SA2-Origin") != c.Origin {
			t.Fatal("missing authentication or audience")
		}
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		switch r.URL.Path {
		case prefix + "/preview":
			previewID = in["requestId"].(string)
			return reply(200, map[string]any{"protocolVersion": 1, "capabilities": map[string]bool{"durableReplay": true, "operationLookup": true}, "canCommit": true, "requestId": previewID, "action": "cancel", "normalized": in["input"], "planToken": "signed-plan", "revision": "v2", "expiresAt": "2027-12-01T00:00:00Z"})
		case prefix + "/commit":
			commits++
			if r.Header.Get("If-Match") != "\"v2\"" || r.Header.Get("X-Idempotency-Key") != previewID || in["planToken"] != "signed-plan" {
				t.Fatal("incorrect plan binding")
			}
			return nil, io.ErrUnexpectedEOF
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
			return nil, nil
		}
	})}}
	cmd, err := Parse([]string{"plan", "cancel", "--id", fakeID, "--reason", "test", "--out", p})
	if err != nil {
		t.Fatal(err)
	}
	data, err := c.Run(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(data)
	if strings.Contains(string(out), "signed-plan") {
		t.Fatal("token in output")
	}
	raw, err := session.ReadPlan(p)
	if err != nil || strings.Contains(string(raw), "memory-only") {
		t.Fatal("invalid private plan", err)
	}
	if _, err := Parse([]string{"cancel", "--plan", p}); err == nil {
		t.Fatal("confirmation missing")
	}
	cmd, err = Parse([]string{"cancel", "--plan", p, "--confirm"})
	if err != nil {
		t.Fatal(err)
	}
	changed := c
	changed.Actor = "other"
	if _, err := changed.Run(context.Background(), cmd); !errors.Is(err, session.ErrPlanFile) {
		t.Fatal(err)
	}
	_, err = c.Run(context.Background(), cmd)
	var f *Failure
	if !errors.As(err, &f) || f.RequestID != previewID || f.Code != "OUTCOME_UNKNOWN" || commits != 1 {
		t.Fatal(err, commits)
	}
}
func TestSlotsAndDSTArguments(t *testing.T) {
	args := []string{"plan", "reschedule", "--id", fakeID, "--timezone", "America/New_York", "--duration", "60", "--out", "p", "--start"}
	for _, stamp := range []string{"2027-11-07T01:30:00-04:00", "2027-11-07T01:30:00-05:00"} {
		if _, err := Parse(append(args, stamp)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Parse(append(args, "2027-03-14T02:30:00-05:00")); err == nil {
		t.Fatal("nonexistent local time")
	}
	if _, err := Parse([]string{"slots", "--student-id", fakeID, "--mentor-id", fakeID, "--from", "2027-11-01", "--to", "2027-11-09", "--timezone", "Asia/Shanghai"}); err == nil {
		t.Fatal("accepted eight days")
	}
}
func TestWait404ReturnsOriginalRequest(t *testing.T) {
	cmd, err := Parse([]string{"wait", "--request-id", fakeID, "--timeout", "3ms"})
	if err != nil {
		t.Fatal(err)
	}
	c := Client{Origin: "https://booking.invalid", Token: []byte("fake"), API: api.Client{HTTP: &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
		return reply(404, map[string]string{"code": "OPERATION_NOT_FOUND"})
	})}}}
	_, err = c.Run(context.Background(), cmd)
	var failure *Failure
	if !errors.As(err, &failure) || failure.Code != "WAIT_TIMEOUT" || failure.RequestID != fakeID {
		t.Fatal(err)
	}
}

func TestWaitStopsOnMissingBookingRoute(t *testing.T) {
	cmd, err := Parse([]string{"wait", "--request-id", fakeID})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	c := Client{Origin: "https://booking.invalid", Token: []byte("fake"), API: api.Client{HTTP: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls > 1 {
			t.Fatal("retried an undeployed route")
		}
		return reply(404, map[string]string{"message": "Cannot GET " + r.URL.Path})
	})}}}
	_, err = c.Run(context.Background(), cmd)
	var failure *Failure
	if !errors.As(err, &failure) || failure.Code != "BOOKING_API_NOT_DEPLOYED" || failure.RequestID != fakeID || calls != 1 {
		t.Fatal(err)
	}
}

func TestOperationLookupPreservesReceiptAndResourceErrors(t *testing.T) {
	for _, tc := range []struct{ name, code string }{
		{"operation", "OPERATION_NOT_FOUND"},
		{"wait", "NOT_FOUND"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd, err := Parse([]string{tc.name, "--request-id", fakeID})
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			c := Client{Origin: "https://booking.invalid", Token: []byte("fake"), API: api.Client{HTTP: &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
				calls++
				if calls != 1 {
					t.Fatal("retried a terminal lookup")
				}
				return reply(404, map[string]string{"code": tc.code})
			})}}}
			_, err = c.Run(context.Background(), cmd)
			var failure *Failure
			if !errors.As(err, &failure) || failure.Code != tc.code || failure.RequestID != fakeID {
				t.Fatal(err)
			}
		})
	}
}
func TestPrivatePlanPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file mode test")
	}
	d := t.TempDir()
	path := filepath.Join(d, "plan")
	if err := session.WritePlan(path, []byte("data")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ReadPlan(path); err == nil {
		t.Fatal("accepted a shared plan")
	}
}
