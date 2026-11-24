package booking

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/victorx98/sa2-cli/internal/api"
	"github.com/victorx98/sa2-cli/internal/session"
)

const prefix = "/api/services/session-bookings"

type Client struct {
	API    api.Client
	Origin string
	Actor  string
	Token  []byte
}
type Failure struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	RequestID  string `json:"requestId,omitempty"`
	SessionID  string `json:"sessionId,omitempty"`
	LastStatus string `json:"lastStatus,omitempty"`
}

func (e *Failure) Error() string { return e.Code }

type plan struct {
	Version      int `json:"protocolVersion"`
	Capabilities struct {
		Durable bool `json:"durableReplay"`
		Lookup  bool `json:"operationLookup"`
	} `json:"capabilities"`
	RequestID  string         `json:"requestId"`
	Action     string         `json:"action"`
	CanCommit  bool           `json:"canCommit"`
	Normalized map[string]any `json:"normalized"`
	Effects    map[string]any `json:"effects"`
	Warnings   []string       `json:"warnings"`
	Blockers   []string       `json:"blockers"`
	ExpiresAt  string         `json:"expiresAt"`
	Revision   string         `json:"revision"`
	Token      string         `json:"planToken,omitempty"`
	Actor      string         `json:"actor,omitempty"`
	Origin     string         `json:"origin,omitempty"`
}

func (c Client) get(ctx context.Context, path string, q url.Values, out any) error {
	return c.API.Business(ctx, c.Origin, c.Token, "GET", path, q, nil, nil, out)
}
func (c Client) Run(ctx context.Context, cmd Command) (any, error) {
	v := cmd.Values
	switch cmd.Name {
	case "find":
		q := url.Values{"text": {v["text"]}, "purpose": {"session-booking"}}
		if v["kind"] == "mentor" {
			q.Set("status", "active")
		}
		var rows []map[string]any
		err := c.get(ctx, "/api/"+v["kind"]+"s/find", q, &rows)
		return projectRows(rows, "id", "nameZh", "nameEn", "type"), err
	case "options":
		return c.options(ctx, v)
	case "slots":
		q := url.Values{"studentId": {v["student-id"]}, "mentorId": {v["mentor-id"]}, "durationMins": {v["duration"]}, "from": {v["from"]}, "to": {v["to"]}, "timezone": {v["timezone"]}}
		var data map[string]any
		err := c.get(ctx, prefix+"/availability", q, &data)
		return data, err
	case "get":
		var row map[string]any
		err := c.get(ctx, "/api/services/sessions/"+v["id"], nil, &row)
		return summary(row), err
	case "list":
		q := url.Values{}
		for name, k := range map[string]string{
			"studentId": "student-id", "mentorId": "mentor-id", "startDate": "from", "endDate": "to",
			"status": "status", "page": "page", "pageSize": "page-size",
		} {
			if v[k] != "" {
				q.Set(name, v[k])
			}
		}
		var page struct {
			Data       []map[string]any `json:"data"`
			Total      int              `json:"total"`
			Page       int              `json:"page"`
			PageSize   int              `json:"pageSize"`
			TotalPages int              `json:"totalPages"`
		}
		err := c.get(ctx, "/api/services/sessions", q, &page)
		for i, r := range page.Data {
			page.Data[i] = summary(r)
		}
		return map[string]any{"sessions": page.Data, "total": page.Total, "page": page.Page, "pageSize": page.PageSize, "totalPages": page.TotalPages, "timezone": "UTC"}, err
	case "plan":
		return c.preview(ctx, cmd)
	case "create", "reschedule", "cancel":
		return c.commit(ctx, cmd)
	case "operation", "wait":
		return c.operation(ctx, cmd)
	}
	return nil, ErrArgument
}
func project(r map[string]any, keys ...string) map[string]any {
	out := map[string]any{}
	for _, k := range keys {
		if value, ok := r[k]; ok {
			out[k] = value
		}
	}
	return out
}
func projectRows(rows []map[string]any, keys ...string) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, project(r, keys...))
	}
	return out
}
func summary(r map[string]any) map[string]any {
	return project(r, "id", "sessionId", "sourceType", "readOnly", "studentUserId", "mentorUserId", "contractId", "sessionType", "sessionTypeId", "entitlementServiceType", "title", "status", "scheduledAt", "durationMins", "bookingTimeZone", "visibleToMentorOnly", "revision", "updatedAt")
}
func (c Client) options(ctx context.Context, v map[string]string) (any, error) {
	var ms, cs, types, balances []map[string]any
	if err := c.get(ctx, "/api/mentors/find", url.Values{"mentorId": {v["mentor-id"]}, "purpose": {"session-booking"}, "status": {"active"}}, &ms); err != nil {
		return nil, err
	}
	if len(ms) != 1 || ms[0]["id"] != v["mentor-id"] {
		return nil, &Failure{Code: "MENTOR_NOT_FOUND", Message: "No active mentor matched this ID."}
	}
	mt, _ := ms[0]["type"].(string)
	mt = strings.ToLower(mt)
	group := ""
	if strings.Contains(mt, "internal") {
		group = "Internal"
	} else if strings.Contains(mt, "external") {
		group = "External"
	} else {
		return nil, &Failure{Code: "MENTOR_TYPE_UNKNOWN", Message: "The mentor type has no bookable session group."}
	}
	if err := c.get(ctx, "/api/contracts/students/"+v["student-id"], nil, &cs); err != nil {
		return nil, err
	}
	if err := c.get(ctx, "/api/services/session-types", url.Values{"serviceType": {group}}, &types); err != nil {
		return nil, err
	}
	chosen := v["contract-id"]
	if chosen != "" {
		found := false
		for _, row := range cs {
			if row["id"] == chosen {
				found = true
				break
			}
		}
		if !found {
			return nil, &Failure{Code: "CONTRACT_NOT_FOUND", Message: "The contract is unavailable for this student."}
		}
		if err := c.get(ctx, "/api/students/"+v["student-id"]+"/contracts", url.Values{"category": {"session"}, "contractId": {chosen}}, &balances); err != nil {
			return nil, err
		}
	}
	return map[string]any{"mentor": project(ms[0], "id", "nameEn", "nameZh", "type"), "contracts": projectRows(cs, "id", "title", "status", "product"), "contractId": chosen, "sessionTypes": types, "balances": balances}, nil
}
func (c Client) preview(ctx context.Context, cmd Command) (any, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}
	v := cmd.Values
	input := map[string]any{}
	for flag, key := range map[string]string{"student-id": "studentId", "mentor-id": "mentorId", "contract-id": "contractId", "type-id": "sessionTypeId", "entitlement": "entitlementServiceType", "title": "title", "notes": "description", "id": "sessionId", "reason": "reason", "start": "scheduledAt", "timezone": "bookingTimeZone"} {
		if v[flag] != "" {
			input[key] = v[flag]
		}
	}
	if v["duration"] != "" {
		input["durationMins"], _ = strconv.Atoi(v["duration"])
	}
	if cmd.Action == "create" {
		input["visibleToMentorOnly"] = v["notes-visibility"] == "mentor-only"
		input["meetingProvider"] = "feishu"
	}
	var p plan
	if err = c.API.Business(ctx, c.Origin, c.Token, "POST", prefix+"/preview", nil, map[string]any{"action": cmd.Action, "requestId": id, "input": input}, nil, &p); err != nil {
		if errors.Is(err, api.ErrBookingAPINotDeployed) {
			return nil, &Failure{Code: "BOOKING_API_NOT_DEPLOYED", Message: err.Error(), RequestID: id}
		}
		return nil, err
	}
	if p.Version != 1 || !p.Capabilities.Durable || !p.Capabilities.Lookup || p.RequestID != id || p.Action != cmd.Action || p.Normalized == nil {
		return nil, api.ErrInvalidResponse
	}
	if p.CanCommit {
		if p.Token == "" || len(p.Blockers) != 0 {
			return nil, api.ErrInvalidResponse
		}
		if _, err = time.Parse(time.RFC3339, p.ExpiresAt); err != nil {
			return nil, api.ErrInvalidResponse
		}
		p.Actor, p.Origin = c.Actor, c.Origin
		raw, err := json.Marshal(p)
		if err != nil {
			return nil, err
		}
		defer clear(raw)
		if err = session.WritePlan(v["out"], raw); err != nil {
			return nil, err
		}
	}
	p.Actor, p.Origin, p.Token = "", "", ""
	delete(p.Normalized, "description")
	return p, nil
}
func (c Client) commit(ctx context.Context, cmd Command) (any, error) {
	raw, err := session.ReadPlan(cmd.Values["plan"])
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	var p plan
	if json.Unmarshal(raw, &p) != nil || !idPattern.MatchString(p.RequestID) || !p.CanCommit || p.Token == "" || p.Version != 1 || !p.Capabilities.Durable || !p.Capabilities.Lookup || p.Action != cmd.Name || p.Origin != c.Origin || p.Actor != c.Actor {
		return nil, session.ErrPlanFile
	}
	h := map[string]string{"X-Idempotency-Key": p.RequestID}
	if p.Action != "create" {
		if p.Revision == "" {
			return nil, session.ErrPlanFile
		}
		h["If-Match"] = "\"" + p.Revision + "\""
	}
	out := map[string]any{}
	err = c.API.Business(ctx, c.Origin, c.Token, "POST", prefix+"/commit", nil, map[string]any{"action": p.Action, "normalized": p.Normalized, "planToken": p.Token}, h, &out)
	if err == nil && (out["requestId"] != p.RequestID || out["action"] != p.Action || out["sessionId"] == nil) {
		err = api.ErrOutcomeUnknown
	}
	if errors.Is(err, api.ErrOutcomeUnknown) {
		return nil, &Failure{Code: "OUTCOME_UNKNOWN", Message: "Look up the original request ID before retrying.", RequestID: p.RequestID}
	}
	if errors.Is(err, api.ErrBookingAPINotDeployed) {
		return nil, &Failure{Code: "BOOKING_API_NOT_DEPLOYED", Message: err.Error(), RequestID: p.RequestID}
	}
	return out, err
}
func (c Client) operation(ctx context.Context, cmd Command) (any, error) {
	id := cmd.Values["request-id"]
	wait := cmd.Name == "wait"
	if wait {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cmd.Timeout)
		defer cancel()
	}
	var state, sid string
	delay := 250 * time.Millisecond
	for {
		var out map[string]any
		err := c.get(ctx, prefix+"/operations/"+id, nil, &out)
		if errors.Is(err, api.ErrBookingAPINotDeployed) {
			return nil, &Failure{Code: "BOOKING_API_NOT_DEPLOYED", Message: err.Error(), RequestID: id}
		}
		if err == nil {
			got, _ := out["requestId"].(string)
			sid, _ = out["sessionId"].(string)
			state, _ = out["syncStatus"].(string)
			if got != id || sid == "" {
				return nil, api.ErrInvalidResponse
			}
			switch state {
			case "succeeded", "not_required":
				return out, nil
			case "failed", "needs_review":
				if wait {
					return nil, &Failure{Code: "SYNC_" + strings.ToUpper(state), Message: "The local operation was accepted. Check the synchronization result.", RequestID: id, SessionID: sid, LastStatus: state}
				}
				return out, nil
			case "pending":
			default:
				return nil, api.ErrInvalidResponse
			}
		}
		if !wait {
			return out, err
		}
		var he *api.BusinessError
		if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, api.ErrUnavailable) && !(errors.As(err, &he) && (he.Status == 404 || he.Status >= 500)) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) {
				return nil, ctx.Err()
			}
			return nil, &Failure{Code: "WAIT_TIMEOUT", Message: "The booking outcome is still unresolved.", RequestID: id, SessionID: sid, LastStatus: state}
		case <-time.After(delay):
		}
		if delay < 2*time.Second {
			delay *= 2
		}
	}
}
