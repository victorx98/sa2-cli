package reports

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/victorx98/sa2-cli/internal/api"
)

const stamp = "2026-08-05T23:18:49.843Z"

func updateCommand() Command {
	notes := "本月无相关内容，可保持系统默认"
	return Command{Action: "update", StudentID: studentID, ReportID: reportID, Period: "2026-06", Key: "first_counselor_strategy_meeting", ExpectedUpdatedAt: stamp, Notes: &notes}
}

func TestMilestoneUpdatePreviewAndConfirmation(t *testing.T) {
	for _, confirm := range []bool{false, true} {
		t.Run(map[bool]string{false: "preview", true: "confirm"}[confirm], func(t *testing.T) {
			cmd := updateCommand()
			cmd.Confirm = confirm
			var methods []string
			report := map[string]any{"id": reportID, "studentId": studentID, "reportPeriod": "2026-06", "updatedAt": stamp,
				"monthlyMilestonesSnapshot": []any{map[string]any{"key": cmd.Key, "status": "on_track", "notes": "Old", "label": "Keep"}, map[string]any{"key": "mock_interview", "status": "done"}}}
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				methods = append(methods, r.Method)
				if r.Method == "PATCH" {
					if !strings.HasSuffix(r.URL.Path, "/"+reportID+"/milestones/"+cmd.Key) {
						t.Errorf("wrong path: %s", r.URL.Path)
					}
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if !reflect.DeepEqual(body, map[string]any{"reportPeriod": cmd.Period, "expectedUpdatedAt": stamp, "notes": *cmd.Notes}) {
						t.Errorf("unexpected patch: %v", body)
					}
					report["monthlyMilestonesSnapshot"].([]any)[0].(map[string]any)["notes"] = *cmd.Notes
					report["updatedAt"] = "2026-08-05T23:18:49.844Z"
				}
				_ = json.NewEncoder(w).Encode(report)
			}))
			defer server.Close()
			client := Client{API: api.Client{HTTP: server.Client()}, Origin: server.URL, Token: []byte("synthetic")}
			result, err := client.Run(context.Background(), cmd)
			if err != nil {
				t.Fatal(err)
			}
			wantMethods := []string{"GET"}
			if confirm {
				wantMethods = []string{"GET", "PATCH", "GET"}
			}
			if !reflect.DeepEqual(methods, wantMethods) {
				t.Fatalf("methods: %v", methods)
			}
			data := result.(map[string]any)
			if data["submitted"] != confirm || (confirm && data["verified"] != true) {
				t.Fatalf("result: %v", data)
			}
			after := data["after"].(map[string]any)
			if after["status"] != "on_track" || after["label"] != "Keep" || after["notes"] != *cmd.Notes {
				t.Fatalf("after: %v", after)
			}
		})
	}
}

func TestUpdateFailureDoesNotRetryOrFallback(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"conflict", 409, `{"code":"STALE_REPORT"}`, "STALE_REPORT"},
		{"not_deployed", 404, "", "MILESTONE_API_NOT_DEPLOYED"},
		{"server_failure", 500, `{}`, "UPDATE_OUTCOME_UNKNOWN"},
		{"bad_success", 200, `invalid`, "UPDATE_OUTCOME_UNKNOWN"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := updateCommand()
			cmd.Confirm = true
			var methods []string
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				methods = append(methods, r.Method)
				if r.Method == "GET" {
					_ = json.NewEncoder(w).Encode(map[string]any{"id": reportID, "studentId": studentID, "reportPeriod": cmd.Period, "updatedAt": stamp, "monthlyMilestonesSnapshot": []any{map[string]any{"key": cmd.Key, "status": "not_started"}}})
					return
				}
				w.WriteHeader(tc.status)
				if tc.name == "not_deployed" {
					_ = json.NewEncoder(w).Encode(map[string]any{"message": "Cannot PATCH " + r.URL.Path})
				} else {
					_, _ = w.Write([]byte(tc.body))
				}
			}))
			defer server.Close()
			client := Client{API: api.Client{HTTP: server.Client()}, Origin: server.URL, Token: []byte("synthetic")}
			_, err := client.Run(context.Background(), cmd)
			var failure *Failure
			var business *api.BusinessError
			code := ""
			if errors.As(err, &failure) {
				code = failure.Code
			} else if errors.As(err, &business) {
				code = business.Code
			}
			if code != tc.want {
				t.Fatalf("got %v, want %s", err, tc.want)
			}
			if !reflect.DeepEqual(methods, []string{"GET", "PATCH"}) {
				t.Fatalf("methods: %v", methods)
			}
		})
	}
}

func TestUpdatePreflightStopsStaleOrWrongReport(t *testing.T) {
	for _, field := range []string{"id", "updatedAt"} {
		t.Run(field, func(t *testing.T) {
			requests := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Method != "GET" {
					t.Errorf("unexpected write")
				}
				report := map[string]any{"id": reportID, "studentId": studentID, "reportPeriod": "2026-06", "updatedAt": stamp, "monthlyMilestonesSnapshot": []any{}}
				if field == "id" {
					report[field] = studentID
				} else {
					report[field] = "2026-08-06T00:00:00.000Z"
				}
				_ = json.NewEncoder(w).Encode(report)
			}))
			defer server.Close()
			client := Client{API: api.Client{HTTP: server.Client()}, Origin: server.URL, Token: []byte("synthetic")}
			cmd := updateCommand()
			cmd.Confirm = true
			_, err := client.Run(context.Background(), cmd)
			var failure *Failure
			if !errors.As(err, &failure) || requests != 1 {
				t.Fatalf("err=%v requests=%d", err, requests)
			}
		})
	}
}

func TestParseUpdate(t *testing.T) {
	base := []string{"monthly", "milestone", "update", "--student-id", studentID, "--report-id", reportID, "--period", "2026-06", "--key", "first_counselor_strategy_meeting", "--expected-updated-at", stamp}
	path := filepath.Join(t.TempDir(), "notes.html")
	if err := os.WriteFile(path, []byte("本月无相关内容"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, flags := range [][]string{{"--notes", "本月完成目标"}, {"--notes-file", path}, {"--status", "done", "--confirm"}, {"--clear-notes"}} {
		if _, err := Parse(append(append([]string{}, base...), flags...)); err != nil {
			t.Fatal(err)
		}
	}
	cmd, err := Parse(append(append([]string{}, base...), "--notes", "本月完成目标"))
	if err != nil || cmd.Notes == nil || *cmd.Notes != "本月完成目标" {
		t.Fatalf("inline notes: cmd=%+v err=%v", cmd, err)
	}
	for _, flags := range [][]string{nil, {"--status", "—"}, {"--notes", ""}, {"--notes", "  "}, {"--notes", "文案", "--notes-file", path}, {"--notes", "文案", "--clear-notes"}, {"--notes", strings.Repeat("😀", 50001)}, {"--notes-file", path, "--clear-notes"}, {"--notes-file", "missing"}, {"--key", "../other", "--status", "done"}, {"--expected-updated-at", "2026-06-01", "--clear-notes"}} {
		if _, err := Parse(append(append([]string{}, base...), flags...)); !errors.Is(err, ErrArgument) {
			t.Fatalf("flags=%v err=%v", flags, err)
		}
	}
}

func TestParseCreateMilestone(t *testing.T) {
	base := []string{"monthly", "milestone", "create", "--student-id", studentID, "--report-id", reportID, "--period", "2026-06", "--expected-updated-at", stamp, "--status", "critical", "--name", "上课节点"}
	cmd, err := Parse(append(append([]string{}, base...), "--notes", "测试"))
	if err != nil || cmd.Action != "create" || cmd.Name != "上课节点" || cmd.Notes == nil || *cmd.Notes != "测试" {
		t.Fatalf("create: cmd=%+v err=%v", cmd, err)
	}
	for _, flags := range [][]string{{"--status", "Critical1"}, {"--name", " "}, {"--clear-notes"}, {"--key", "mock_interview"}} {
		if _, err := Parse(append(append([]string{}, base...), flags...)); !errors.Is(err, ErrArgument) {
			t.Fatalf("flags=%v err=%v", flags, err)
		}
	}
}

func TestCreateMilestonePreviewAndConfirmation(t *testing.T) {
	notes := "测试"
	cmd := Command{Action: "create", StudentID: studentID, ReportID: reportID, Period: "2026-06", Name: "上课节点", ExpectedUpdatedAt: stamp, Status: "critical", Notes: &notes}
	report := map[string]any{"id": reportID, "studentId": studentID, "reportPeriod": cmd.Period, "updatedAt": stamp,
		"monthlyMilestonesSnapshot": []any{map[string]any{"key": "first_counselor_strategy_meeting", "status": "done"}}}
	var methods []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		if r.Method == "POST" {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["label"] != cmd.Name || body["status"] != cmd.Status || body["notes"] != notes {
				t.Errorf("unexpected body: %v", body)
			}
			report["monthlyMilestonesSnapshot"] = append(report["monthlyMilestonesSnapshot"].([]any), map[string]any{"key": "custom_44444444-4444-4444-8444-444444444444", "label": cmd.Name, "status": cmd.Status, "notes": notes})
			report["updatedAt"] = "2026-08-05T23:18:49.844Z"
		}
		_ = json.NewEncoder(w).Encode(report)
	}))
	defer server.Close()
	client := Client{API: api.Client{HTTP: server.Client()}, Origin: server.URL, Token: []byte("synthetic")}
	preview, err := client.Run(context.Background(), cmd)
	if err != nil || preview.(map[string]any)["submitted"] != false || !reflect.DeepEqual(methods, []string{"GET"}) {
		t.Fatalf("preview=%v err=%v methods=%v", preview, err, methods)
	}
	cmd.Confirm = true
	result, err := client.Run(context.Background(), cmd)
	if err != nil || result.(map[string]any)["verified"] != true || !reflect.DeepEqual(methods, []string{"GET", "GET", "POST", "GET"}) {
		t.Fatalf("result=%v err=%v methods=%v", result, err, methods)
	}
}
