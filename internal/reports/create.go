package reports

import (
	"context"
	"errors"
	"strings"

	"github.com/victorx98/sa2-cli/internal/api"
)

var milestoneNames = map[string]string{
	"first_counselor_strategy_meeting": "首次顾问战略沟通会",
	"first_mentor_session":             "岗位差距分析会议",
	"parent_meeting":                   "家长沟通会",
	"job_search_prep_meeting":          "求职阶段部署会议",
	"resume_revision_finalization":     "简历修改与定稿",
	"cover_letter_finalization":        "求职信与第三人称介绍撰写与定稿",
	"self_intro_video_recording":       "自我介绍视频录制",
	"linkedin_profile_setup":           "LinkedIn页面搭建与Networking学习",
	"bq_script_review":                 "BQ 逐字稿撰写",
	"professional_skills_enhancement":  "专业技能提升",
	"interview_skills_enhancement":     "面试技能提升",
	"networking":                       "人脉拓展",
	"job_application":                  "岗位投递",
	"mock_interview":                   "模拟面试",
}

func displayName(item map[string]any) string {
	if label, ok := item["label"].(string); ok && strings.TrimSpace(label) != "" {
		return strings.TrimSpace(label)
	}
	key, _ := item["key"].(string)
	return milestoneNames[key]
}

func (c Client) create(ctx context.Context, cmd Command) (any, error) {
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
	items := report["monthlyMilestonesSnapshot"].([]any)
	for _, raw := range items {
		if displayName(raw.(map[string]any)) == cmd.Name {
			return nil, &Failure{"MILESTONE_NAME_EXISTS", "Update the existing milestone instead of creating another one."}
		}
	}
	after := map[string]any{"label": cmd.Name, "status": cmd.Status}
	if cmd.Notes != nil {
		after["notes"] = *cmd.Notes
	}
	result := map[string]any{"studentId": cmd.StudentID, "reportId": cmd.ReportID, "reportPeriod": cmd.Period, "name": cmd.Name, "expectedUpdatedAt": cmd.ExpectedUpdatedAt, "after": after, "submitted": false}
	if !cmd.Confirm {
		return result, nil
	}
	body := map[string]any{"reportPeriod": cmd.Period, "expectedUpdatedAt": cmd.ExpectedUpdatedAt, "label": cmd.Name, "status": cmd.Status}
	if cmd.Notes != nil {
		body["notes"] = *cmd.Notes
	}
	var updated map[string]any
	err = c.API.Business(ctx, c.Origin, c.Token, "POST", "/api/reports/students/"+cmd.StudentID+"/monthly-service-reports/"+cmd.ReportID+"/milestones", nil, body, nil, &updated)
	if errors.Is(err, api.ErrOutcomeUnknown) {
		return nil, &Failure{"UPDATE_OUTCOME_UNKNOWN", "The creation outcome is unknown. Read the selected report before another submission."}
	}
	if err != nil {
		return nil, err
	}
	readback, err := c.Run(ctx, Command{Action: "get", StudentID: cmd.StudentID, Period: cmd.Period, Section: "milestones"})
	if err != nil {
		return nil, &Failure{"UPDATE_VERIFICATION_FAILED", "Creation may have succeeded but readback failed. Read the report before another submission."}
	}
	verified := readback.(map[string]any)
	if verified["id"] != cmd.ReportID {
		return nil, &Failure{"UPDATE_VERIFICATION_FAILED", "The created milestone could not be verified."}
	}
	var found map[string]any
	for _, raw := range verified["monthlyMilestonesSnapshot"].([]any) {
		item := raw.(map[string]any)
		if displayName(item) == cmd.Name {
			if found != nil {
				return nil, &Failure{"UPDATE_VERIFICATION_FAILED", "The created milestone name is ambiguous."}
			}
			found = item
		}
	}
	if found == nil || found["status"] != cmd.Status || (cmd.Notes != nil && noteText(found) != strings.TrimSpace(*cmd.Notes)) {
		return nil, &Failure{"UPDATE_VERIFICATION_FAILED", "The created milestone differs from the requested result."}
	}
	result["after"] = found
	result["submitted"] = true
	result["verified"] = true
	result["updatedAt"] = verified["updatedAt"]
	return result, nil
}
