package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

// These tests pin the boundary codes documented in the agent skills to the exit
// behavior the binary actually produces. A skill that names a code the CLI
// never emits, or emits under a different exit code, misleads the agent that
// depends on it.

const boundaryID = "11111111-1111-4111-8111-111111111111"

// boundaryError mirrors the error envelope the CLI prints on stdout.
type boundaryError struct {
	Code       string   `json:"code"`
	Message    string   `json:"message"`
	RequestID  string   `json:"requestId"`
	SessionID  string   `json:"sessionId"`
	LastStatus string   `json:"lastStatus"`
	Blockers   []string `json:"blockers"`
}

// fixture serves authentication and delegates every other path to a swappable
// handler so one signed-in session can drive many boundary cases.
type fixture struct {
	mu      sync.Mutex
	handler http.HandlerFunc
}

func (f *fixture) set(handler http.HandlerFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handler = handler
}

func (f *fixture) serve(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/auth/login", "/api/auth/refresh":
		reply(w, mockRefresh)
		return
	}
	f.mu.Lock()
	handler := f.handler
	f.mu.Unlock()
	if handler == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	handler(w, r)
}

func observeBoundary(t *testing.T, cmd *exec.Cmd) (int, *boundaryError) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	exit := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exit = exitErr.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(stdout.Bytes(), []byte("\n")) != 1 {
		t.Fatalf("stdout must be one JSON line, got %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr must stay empty, got %q", stderr.String())
	}
	var result struct {
		Data  json.RawMessage `json:"data"`
		Error *boundaryError  `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return exit, result.Error
}

func decodeBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// The skills state that every server business code reaches the agent verbatim
// with exit code 1 and without a blocker list.
func TestServerBoundaryCodesReachTheAgentVerbatim(t *testing.T) {
	codes := []struct {
		status int
		code   string
	}{
		{409, "TIME_CONFLICT"},
		{409, "INSUFFICIENT_BALANCE"},
		{409, "SESSION_BILLING_LOCKED"},
		{409, "SESSION_TYPE_MISMATCH"},
		{409, "MENTOR_INACTIVE"},
		{409, "MENTOR_PRICE_MISSING"},
		{409, "START_IN_PAST"},
		{409, "NO_SCHEDULE_CHANGE"},
		{409, "ENTITLEMENT_NOT_FOUND"},
		{409, "ENTITLEMENT_AMBIGUOUS"},
		{409, "SESSION_NOT_WRITABLE"},
		{409, "OPERATION_IN_PROGRESS"},
		{409, "IDEMPOTENCY_MISMATCH"},
		{409, "PLAN_EFFECTS_CHANGED"},
		{412, "STALE_SESSION"},
		{410, "PLAN_EXPIRED"},
		{403, "FORBIDDEN"},
		{403, "ORIGIN_MISMATCH"},
		{404, "NOT_FOUND"},
		{400, "INVALID_BOOKING_INPUT"},
		{400, "INVALID_SEARCH"},
		{503, "BOOKING_NOT_CONFIGURED"},
	}
	f := &fixture{}
	h := startHarness(t, f.serve)
	h.login()
	for _, tc := range codes {
		t.Run(tc.code, func(t *testing.T) {
			f.set(func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, tc.status, map[string]any{
					"statusCode": tc.status, "code": tc.code, "message": tc.code,
				})
			})
			out := filepath.Join(t.TempDir(), "plan.json")
			exit, err := observeBoundary(t, h.command("", "",
				"sessions", "plan", "cancel", "--id", boundaryID, "--reason", "test", "--out", out))
			if exit != 1 || err == nil || err.Code != tc.code {
				t.Fatalf("exit %d, error %+v; want exit 1 and %s", exit, err, tc.code)
			}
			if len(err.Blockers) != 0 {
				t.Fatalf("a server rejection must not carry blockers: %v", err.Blockers)
			}
		})
	}
}

// A blocked preview is the only place the agent learns every blocker at once.
func TestBlockedPreviewReportsEveryBlocker(t *testing.T) {
	blockers := []string{
		"SESSION_TYPE_MISMATCH", "MENTOR_PRICE_MISSING", "ENTITLEMENT_AMBIGUOUS",
		"SESSION_BILLING_LOCKED", "TIME_CONFLICT", "INSUFFICIENT_BALANCE",
	}
	f := &fixture{}
	h := startHarness(t, f.serve)
	h.login()
	f.set(func(w http.ResponseWriter, r *http.Request) {
		body := decodeBody(t, r)
		writeJSON(w, 200, map[string]any{
			"protocolVersion": 1,
			"capabilities":    map[string]bool{"durableReplay": true, "operationLookup": true},
			"canCommit":       false,
			"requestId":       body["requestId"],
			"action":          body["action"],
			"normalized":      body["input"],
			"blockers":        blockers,
			"warnings":        []string{"ENTITLEMENT_OVERRIDE", "LEGACY_ENTITLEMENT_SOURCE"},
			"expiresAt":       "2027-12-01T00:00:00Z",
		})
	})
	out := filepath.Join(t.TempDir(), "plan.json")
	exit, err := observeBoundary(t, h.command("", "",
		"sessions", "plan", "cancel", "--id", boundaryID, "--reason", "test", "--out", out))
	if exit != 1 || err == nil || err.Code != "PLAN_BLOCKED" {
		t.Fatalf("exit %d, error %+v; want exit 1 and PLAN_BLOCKED", exit, err)
	}
	if !slices.Equal(err.Blockers, blockers) {
		t.Fatalf("blockers %v; want %v", err.Blockers, blockers)
	}
}

// planThenCommit serves a valid preview and fails the commit with the given
// status and body, which is how the commit stage is reached.
func planThenCommit(t *testing.T, commitStatus int, commitBody any) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/services/session-bookings/preview":
			body := decodeBody(t, r)
			writeJSON(w, 200, map[string]any{
				"protocolVersion": 1,
				"capabilities":    map[string]bool{"durableReplay": true, "operationLookup": true},
				"canCommit":       true,
				"requestId":       body["requestId"],
				"action":          body["action"],
				"normalized":      body["input"],
				"planToken":       "signed-plan",
				"revision":        "r1",
				"expiresAt":       "2027-12-01T00:00:00Z",
			})
		case "/api/services/session-bookings/commit":
			writeJSON(w, commitStatus, commitBody)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func commitThrough(t *testing.T, h harness) (int, *boundaryError) {
	t.Helper()
	out := filepath.Join(t.TempDir(), "plan.json")
	if exit, err := observeBoundary(t, h.command("", "",
		"sessions", "plan", "cancel", "--id", boundaryID, "--reason", "test", "--out", out)); exit != 0 {
		t.Fatalf("preview failed with exit %d and %+v", exit, err)
	}
	return observeBoundary(t, h.command("", "", "sessions", "cancel", "--plan", out, "--confirm"))
}

// The commit stage reports one blocker at a time and never a blocker list, so
// the skills must point the agent back at the preview for the full set.
func TestCommitRejectionCarriesNoBlockerList(t *testing.T) {
	f := &fixture{}
	h := startHarness(t, f.serve)
	h.login()
	f.set(planThenCommit(t, 409, map[string]any{
		"statusCode": 409, "code": "TIME_CONFLICT", "message": "TIME_CONFLICT",
	}))
	exit, err := commitThrough(t, h)
	if exit != 1 || err == nil || err.Code != "TIME_CONFLICT" {
		t.Fatalf("exit %d, error %+v; want exit 1 and TIME_CONFLICT", exit, err)
	}
	if len(err.Blockers) != 0 {
		t.Fatalf("the commit stage must not carry a blocker list: %v", err.Blockers)
	}
}

// A 5xx on the commit path leaves the outcome unknown, so the CLI keeps the
// original request ID for a read-only lookup instead of reporting a rejection.
func TestCommitServerErrorKeepsTheOutcomeUnknown(t *testing.T) {
	f := &fixture{}
	h := startHarness(t, f.serve)
	h.login()
	f.set(planThenCommit(t, 500, map[string]any{"statusCode": 500}))
	exit, err := commitThrough(t, h)
	if exit != 1 || err == nil || err.Code != "OUTCOME_UNKNOWN" {
		t.Fatalf("exit %d, error %+v; want exit 1 and OUTCOME_UNKNOWN", exit, err)
	}
	if err.RequestID == "" {
		t.Fatal("OUTCOME_UNKNOWN must keep the original request ID")
	}
}

// A 409 without a machine readable code falls back to CONFLICT.
func TestUnclassifiedConflictFallsBackToConflict(t *testing.T) {
	f := &fixture{}
	h := startHarness(t, f.serve)
	h.login()
	f.set(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 409, map[string]any{"statusCode": 409, "message": "该课时已完成，不能再修改会议时间。"})
	})
	out := filepath.Join(t.TempDir(), "plan.json")
	exit, err := observeBoundary(t, h.command("", "",
		"sessions", "plan", "cancel", "--id", boundaryID, "--reason", "test", "--out", out))
	if exit != 1 || err == nil || err.Code != "CONFLICT" {
		t.Fatalf("exit %d, error %+v; want exit 1 and CONFLICT", exit, err)
	}
}

// An environment without the booking routes is reported once and never retried.
func TestMissingBookingRoutesAreReportedOnce(t *testing.T) {
	f := &fixture{}
	h := startHarness(t, f.serve)
	h.login()
	calls := 0
	f.set(func(w http.ResponseWriter, r *http.Request) {
		calls++
		writeJSON(w, 404, map[string]any{"message": "Cannot POST " + r.URL.Path})
	})
	out := filepath.Join(t.TempDir(), "plan.json")
	exit, err := observeBoundary(t, h.command("", "",
		"sessions", "plan", "cancel", "--id", boundaryID, "--reason", "test", "--out", out))
	if exit != 1 || err == nil || err.Code != "BOOKING_API_NOT_DEPLOYED" {
		t.Fatalf("exit %d, error %+v; want exit 1 and BOOKING_API_NOT_DEPLOYED", exit, err)
	}
	if calls != 1 {
		t.Fatalf("requests %d; want 1", calls)
	}
}

// Argument checks run before any request, so they always exit with code 2.
func TestLocalValidationFailsBeforeAnyRequest(t *testing.T) {
	cases := [][]string{
		{"sessions", "slots", "--student-id", boundaryID, "--mentor-id", boundaryID,
			"--from", "2027-11-01", "--to", "2027-11-09", "--timezone", "Asia/Shanghai"},
		{"sessions", "slots", "--student-id", boundaryID, "--mentor-id", boundaryID,
			"--from", "2027-11-01", "--to", "2027-11-01", "--timezone", "Asia/Shanghai"},
		{"sessions", "plan", "reschedule", "--id", boundaryID,
			"--start", "2027-03-14T02:30:00-05:00", "--timezone", "America/New_York",
			"--duration", "60", "--out", "plan.json"},
		{"sessions", "plan", "reschedule", "--id", boundaryID,
			"--start", "2027-11-07T01:30:00-04:00", "--timezone", "Mars/Olympus",
			"--duration", "60", "--out", "plan.json"},
		{"sessions", "plan", "reschedule", "--id", boundaryID,
			"--start", "2027-11-07T01:30:00-04:00", "--timezone", "America/New_York",
			"--duration", "10", "--out", "plan.json"},
		{"sessions", "plan", "reschedule", "--id", "not-a-uuid",
			"--start", "2027-11-07T01:30:00-04:00", "--timezone", "America/New_York",
			"--duration", "60", "--out", "plan.json"},
		{"sessions", "plan", "reschedule", "--id", boundaryID,
			"--start", "2027-11-07T01:30:00-04:00", "--timezone", "America/New_York",
			"--duration", "60"},
		{"sessions", "plan", "cancel", "--id", boundaryID, "--reason", "test"},
	}
	f := &fixture{}
	h := startHarness(t, f.serve)
	for _, args := range cases {
		t.Run(fmt.Sprint(args), func(t *testing.T) {
			f.set(func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("local validation must not reach the API: %s", r.URL.Path)
			})
			exit, err := observeBoundary(t, h.command("", "", args...))
			if exit != 2 || err == nil || err.Code != "INVALID_ARGUMENT" {
				t.Fatalf("exit %d, error %+v; want exit 2 and INVALID_ARGUMENT", exit, err)
			}
		})
	}
}

// Both offsets of a repeated local hour are accepted, so the agent has to name
// the intended offset explicitly.
func TestRepeatedHourAcceptsBothOffsets(t *testing.T) {
	f := &fixture{}
	h := startHarness(t, f.serve)
	h.login()
	for _, stamp := range []string{"2027-11-07T01:30:00-04:00", "2027-11-07T01:30:00-05:00"} {
		t.Run(stamp, func(t *testing.T) {
			f.set(func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, 409, map[string]any{
					"statusCode": 409, "code": "TIME_CONFLICT", "message": "TIME_CONFLICT",
				})
			})
			out := filepath.Join(t.TempDir(), "plan.json")
			exit, err := observeBoundary(t, h.command("", "",
				"sessions", "plan", "reschedule", "--id", boundaryID, "--start", stamp,
				"--timezone", "America/New_York", "--duration", "60", "--out", out))
			if exit != 1 || err == nil || err.Code != "TIME_CONFLICT" {
				t.Fatalf("exit %d, error %+v; the request must reach the API", exit, err)
			}
		})
	}
}

// Waiting for the external meeting reports a timeout and keeps both IDs.
func TestWaitTimeoutKeepsTheRequestAndSession(t *testing.T) {
	f := &fixture{}
	h := startHarness(t, f.serve)
	h.login()
	f.set(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 404, map[string]any{"code": "OPERATION_NOT_FOUND"})
	})
	exit, err := observeBoundary(t, h.command("", "",
		"sessions", "wait", "--request-id", boundaryID, "--timeout", "1ms"))
	if exit != 1 || err == nil || err.Code != "WAIT_TIMEOUT" {
		t.Fatalf("exit %d, error %+v; want exit 1 and WAIT_TIMEOUT", exit, err)
	}
	if err.RequestID != boundaryID {
		t.Fatalf("request ID %q; want %q", err.RequestID, boundaryID)
	}
}

// A failed external meeting keeps the accepted local write and asks for review.
func TestFailedSyncIsReportedSeparately(t *testing.T) {
	for _, state := range []string{"failed", "needs_review"} {
		t.Run(state, func(t *testing.T) {
			f := &fixture{}
			h := startHarness(t, f.serve)
			h.login()
			f.set(func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, 200, map[string]any{
					"requestId": boundaryID, "sessionId": "session-id",
					"action": "create", "businessStatus": "pending_meeting",
					"syncStatus": state, "notificationStatus": "not_required",
					"revision": "r1", "errorCode": "MEETING_REJECTED",
					"scheduledAt": "2027-12-01T14:00:00Z", "updatedAt": "2027-12-01T14:00:00Z",
				})
			})
			exit, err := observeBoundary(t, h.command("", "",
				"sessions", "wait", "--request-id", boundaryID, "--timeout", "5s"))
			if exit != 1 || err == nil || err.Code != "SYNC_"+strings.ToUpper(state) {
				t.Fatalf("exit %d, error %+v; want exit 1 and SYNC_%s", exit, err, strings.ToUpper(state))
			}
			if err.SessionID != "session-id" || err.LastStatus != state {
				t.Fatalf("error %+v; want the session ID and the last status", err)
			}
		})
	}
}

// The plan file is single use by path and bound to the account and origin.
func TestPlanFileGuards(t *testing.T) {
	f := &fixture{}
	h := startHarness(t, f.serve)
	h.login()
	dir := t.TempDir()
	existing := filepath.Join(dir, "taken.json")
	if err := os.WriteFile(existing, []byte("taken"), 0600); err != nil {
		t.Fatal(err)
	}
	exit, err := observeBoundary(t, h.command("", "",
		"sessions", "plan", "cancel", "--id", boundaryID, "--reason", "test", "--out", existing))
	if exit != 2 || err == nil || err.Code != "PLAN_FILE_EXISTS" {
		t.Fatalf("exit %d, error %+v; want exit 2 and PLAN_FILE_EXISTS", exit, err)
	}
	exit, err = observeBoundary(t, h.command("", "",
		"sessions", "cancel", "--plan", filepath.Join(dir, "missing.json"), "--confirm"))
	if exit != 2 || err == nil || err.Code != "INVALID_PLAN_FILE" {
		t.Fatalf("exit %d, error %+v; want exit 2 and INVALID_PLAN_FILE", exit, err)
	}
}

// The skills promise the CLI never exposes the meeting link, the notes or the
// cancellation reason, so the agent is told to ask the web page instead.
func TestSessionProjectionOmitsMeetingAndNotes(t *testing.T) {
	f := &fixture{}
	h := startHarness(t, f.serve)
	h.login()
	f.set(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{
			"id": boundaryID, "status": "scheduled", "title": "简历辅导",
			"scheduledAt": "2027-12-02T14:00:00Z", "durationMins": 90,
			"bookingTimeZone": "America/New_York", "visibleToMentorOnly": false,
			"revision": "r1", "updatedAt": "2027-11-01T00:00:00Z",
			"description": "仅导师可见的备注", "cancelledReason": "学生要求取消",
			"meeting": map[string]any{
				"meetingId": "meeting-id", "meetingUrl": "https://meeting.invalid/j/abc",
				"status": "scheduled", "scheduleStartTime": "2027-12-02T14:00:00Z",
			},
		})
	})
	var stdout bytes.Buffer
	cmd := h.command("", "", "sessions", "get", "--id", boundaryID)
	cmd.Stdout, cmd.Stderr = &stdout, &bytes.Buffer{}
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"meeting", "meetingUrl", "meetingId", "description", "cancelledReason"} {
		if _, ok := result.Data[field]; ok {
			t.Fatalf("the CLI projection must not expose %s", field)
		}
	}
	for _, field := range []string{"id", "status", "title", "scheduledAt", "durationMins", "bookingTimeZone", "revision"} {
		if _, ok := result.Data[field]; !ok {
			t.Fatalf("the CLI projection must keep %s", field)
		}
	}
}

// The options command reports its own object level failures.

func TestOptionsReportsObjectFailures(t *testing.T) {
	cases := []struct {
		name      string
		mentors   []map[string]any
		contracts []map[string]any
		want      string
	}{
		{"unknown mentor", []map[string]any{}, nil, "MENTOR_NOT_FOUND"},
		{"unbookable mentor", []map[string]any{{"id": boundaryID, "type": "hybrid"}}, nil, "MENTOR_TYPE_UNKNOWN"},
		{"unknown contract", []map[string]any{{"id": boundaryID, "type": "internal mentor"}},
			[]map[string]any{{"id": "33333333-3333-4333-8333-333333333333"}}, "CONTRACT_NOT_FOUND"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fixture{}
			h := startHarness(t, f.serve)
			h.login()
			f.set(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/mentors/find":
					writeJSON(w, 200, tc.mentors)
				case "/api/contracts/students/" + boundaryID:
					writeJSON(w, 200, tc.contracts)
				case "/api/services/session-types":
					writeJSON(w, 200, []map[string]any{})
				case "/api/students/" + boundaryID + "/contracts":
					writeJSON(w, 200, []map[string]any{})
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
					w.WriteHeader(404)
				}
			})
			exit, err := observeBoundary(t, h.command("", "",
				"sessions", "options", "--student-id", boundaryID, "--mentor-id", boundaryID,
				"--contract-id", "22222222-2222-4222-8222-222222222222"))
			if exit != 1 || err == nil || err.Code != tc.want {
				t.Fatalf("exit %d, error %+v; want exit 1 and %s", exit, err, tc.want)
			}
		})
	}
}
