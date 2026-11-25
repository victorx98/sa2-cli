package reports

import (
	"encoding/json"

	"github.com/victorx98/sa2-cli/contracts"
	"github.com/victorx98/sa2-cli/internal/api"
)

func MilestoneSchema() (any, error) {
	var document struct {
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(contracts.OpenAPI, &document); err != nil {
		return nil, api.ErrInvalidResponse
	}
	schema := document.Components.Schemas["PatchMonthlyServiceReportMilestoneDto"]
	if len(schema) == 0 {
		return nil, api.ErrInvalidResponse
	}
	return map[string]any{"requestBody": schema, "method": "PATCH", "path": "/api/reports/students/{studentId}/monthly-service-reports/{reportId}/milestones/{milestoneKey}"}, nil
}
