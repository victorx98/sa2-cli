package reports

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/victorx98/sa2-cli/internal/api"
)

const studentID = "11111111-1111-4111-8111-111111111111"
const reportID = "22222222-2222-4222-8222-222222222222"

func TestParse(t *testing.T) {
	for _, args := range [][]string{
		{"monthly", "get", "--student-id", studentID, "--period", "2026-11"},
		{"monthly", "latest", "--student-id", studentID, "--through", "2026-11", "--section", "milestones"},
	} {
		if _, err := Parse(args); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	for _, args := range [][]string{
		{"monthly", "get", "--student-id", "../other", "--period", "2026-11"},
		{"monthly", "get", "--student-id", studentID, "--period", "2026-13"},
		{"monthly", "get", "--student-id", studentID, "--period", "2026-11", "--section", "unknown"},
		{"monthly", "latest", "--student-id", studentID, "--lookback", "0"},
		{"monthly", "latest", "--student-id", studentID, "--lookback", "121"},
		{"monthly", "latest", "--student-id", studentID, "--through", "2000-01"},
	} {
		if _, err := Parse(args); !errors.Is(err, ErrArgument) {
			t.Fatalf("%v: %v", args, err)
		}
	}
}

func TestLatestSkipsTemplatesAndProjectsMilestones(t *testing.T) {
	var months []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/reports/students/"+studentID+"/monthly-service-reports" || r.Header.Get("Authorization") != "Bearer synthetic" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		month := r.URL.Query().Get("month")
		months = append(months, month)
		var id any
		if month == "12" {
			id = reportID
		}
		period := r.URL.Query().Get("year") + "-" + month
		if month == "1" {
			period = r.URL.Query().Get("year") + "-01"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "studentId": studentID, "reportPeriod": period, "monthlySummary": map[string]any{"private": "not requested"}, "monthlyMilestonesSnapshot": []any{map[string]any{"key": "first_mentor_session", "status": "done", "notes": "已完成\n第一课"}}})
	}))
	defer server.Close()
	client := Client{API: api.Client{HTTP: server.Client()}, Origin: server.URL, Token: []byte("synthetic")}
	result, err := client.Run(context.Background(), Command{Action: "latest", StudentID: studentID, Through: "2026-01", Lookback: 3, Section: "milestones"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(months, []string{"1", "12"}) {
		t.Fatalf("months: %v", months)
	}
	data := result.(map[string]any)
	report := data["report"].(map[string]any)
	if report["reportPeriod"] != "2025-12" || report["monthlySummary"] != nil {
		t.Fatalf("unexpected projection: %v", report)
	}
	search := data["search"].(map[string]any)
	if search["from"] != "2025-11" || search["checkedMonths"] != 2 {
		t.Fatalf("unexpected bounds: %v", search)
	}
}

func TestReadFailuresAreNotMissingReports(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   int
		body     string
		want     error
		requests int
	}{
		{"missing", 200, `{"id":null,"studentId":"` + studentID + `","reportPeriod":"2026-11"}`, ErrNotFound, 1},
		{"wrong_student", 200, `{"id":"` + reportID + `","studentId":"other","reportPeriod":"2026-11"}`, api.ErrInvalidResponse, 1},
		{"wrong_period", 200, `{"id":"` + reportID + `","studentId":"` + studentID + `","reportPeriod":"2026-10"}`, api.ErrInvalidResponse, 1},
		{"missing_id", 200, `{"studentId":"` + studentID + `","reportPeriod":"2026-11"}`, api.ErrInvalidResponse, 1},
		{"unauthorized", 401, `{}`, api.ErrRejected, 1},
		{"forbidden", 403, `{}`, nil, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client := Client{API: api.Client{HTTP: server.Client()}, Origin: server.URL, Token: []byte("synthetic")}
			_, err := client.Run(context.Background(), Command{Action: "latest", StudentID: studentID, Through: "2026-11", Lookback: 1})
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if tc.want == nil {
				var business *api.BusinessError
				if !errors.As(err, &business) || business.Status != 403 {
					t.Fatalf("got %v", err)
				}
			}
			if requests != tc.requests {
				t.Fatalf("requests: %d", requests)
			}
		})
	}
}
