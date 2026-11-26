package reports

import (
	"context"
	"errors"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/victorx98/sa2-cli/internal/api"
)

var milestoneKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,200}$`)

type Failure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Failure) Error() string { return e.Code }

func validStatus(value string) bool {
	switch value {
	case "not_started", "done", "on_track", "needs_attention", "at_risk", "critical":
		return true
	}
	return false
}

func readNotes(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", ErrArgument
	}
	raw, err := io.ReadAll(io.LimitReader(f, 400001))
	if err != nil || len(raw) > 400000 || !utf8.Valid(raw) {
		return "", ErrArgument
	}
	if !validNotes(string(raw)) {
		return "", ErrArgument
	}
	return string(raw), nil
}

func validNotes(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	// Keep supplementary characters within the API length limit.
	units := 0
	for _, r := range value {
		units++
		if r > 0xffff {
			units++
		}
	}
	return units <= 100000
}

func findMilestone(report map[string]any, key string) (map[string]any, error) {
	items, ok := report["monthlyMilestonesSnapshot"].([]any)
	if !ok {
		return nil, api.ErrInvalidResponse
	}
	var found map[string]any
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			return nil, api.ErrInvalidResponse
		}
		if item["key"] == key {
			if found != nil {
				return nil, &Failure{"AMBIGUOUS_MILESTONE_KEY", "The report contains duplicate milestone keys."}
			}
			found = item
		}
	}
	if found == nil {
		return nil, &Failure{"MILESTONE_NOT_FOUND", "Read the report and use an existing milestone key."}
	}
	return found, nil
}

func (c Client) update(ctx context.Context, cmd Command) (any, error) {
	data, err := c.Run(ctx, Command{Action: "get", StudentID: cmd.StudentID, Period: cmd.Period, Section: "milestones"})
	if err != nil {
		return nil, err
	}
	report := data.(map[string]any)
	if report["id"] != cmd.ReportID {
		return nil, &Failure{"REPORT_ID_MISMATCH", "The report ID does not match the selected student and month."}
	}
	if report["updatedAt"] != cmd.ExpectedUpdatedAt {
		return nil, &Failure{"STALE_REPORT", "Read the current report and review the change again."}
	}
	before, err := findMilestone(report, cmd.Key)
	if err != nil {
		return nil, err
	}
	status, ok := before["status"].(string)
	if !ok || !validStatus(status) {
		return nil, api.ErrInvalidResponse
	}
	after := map[string]any{}
	for k, v := range before {
		after[k] = v
	}
	body := map[string]any{"reportPeriod": cmd.Period, "expectedUpdatedAt": cmd.ExpectedUpdatedAt}
	if cmd.Status != "" {
		body["status"] = cmd.Status
		after["status"] = cmd.Status
	}
	if cmd.Notes != nil {
		body["notes"] = *cmd.Notes
		after["notes"] = *cmd.Notes
	}
	result := map[string]any{"studentId": cmd.StudentID, "reportId": cmd.ReportID, "reportPeriod": cmd.Period, "milestoneKey": cmd.Key, "expectedUpdatedAt": cmd.ExpectedUpdatedAt, "before": before, "after": after, "submitted": false}
	if !cmd.Confirm {
		return result, nil
	}
	var updated map[string]any
	err = c.API.Business(ctx, c.Origin, c.Token, "PATCH", "/api/reports/students/"+cmd.StudentID+"/monthly-service-reports/"+cmd.ReportID+"/milestones/"+cmd.Key, nil, body, nil, &updated)
	if errors.Is(err, api.ErrOutcomeUnknown) {
		return nil, &Failure{"UPDATE_OUTCOME_UNKNOWN", "The update outcome is unknown. Read the selected report before any further submission."}
	}
	if err != nil {
		return nil, err
	}
	if updated["id"] != cmd.ReportID || updated["studentId"] != cmd.StudentID || updated["reportPeriod"] != cmd.Period {
		return nil, &Failure{"UPDATE_OUTCOME_UNKNOWN", "The update response could not be verified. Read the selected report."}
	}
	// Read back the stored report rather than trusting the write response alone.
	readback, err := c.Run(ctx, Command{Action: "get", StudentID: cmd.StudentID, Period: cmd.Period, Section: "milestones"})
	if err != nil {
		return nil, &Failure{"UPDATE_VERIFICATION_FAILED", "The update succeeded but readback failed. Read the report; do not blindly resubmit."}
	}
	verified := readback.(map[string]any)
	observed, err := findMilestone(verified, cmd.Key)
	if err != nil || verified["id"] != cmd.ReportID || observed["status"] != after["status"] || noteText(observed) != strings.TrimSpace(noteText(after)) {
		return nil, &Failure{"UPDATE_VERIFICATION_FAILED", "The observed milestone differs from the requested result. Read the current report before further changes."}
	}
	result["submitted"] = true
	result["verified"] = true
	result["after"] = observed
	result["updatedAt"] = verified["updatedAt"]
	return result, nil
}

func noteText(item map[string]any) string {
	text, _ := item["notes"].(string)
	return text
}
