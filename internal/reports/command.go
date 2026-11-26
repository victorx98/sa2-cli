package reports

import (
	"context"
	"errors"
	"flag"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/victorx98/sa2-cli/internal/api"
)

var ErrArgument = errors.New("invalid monthly report argument")
var ErrHelp = errors.New("monthly report help")
var ErrNotFound = errors.New("no stored monthly report in the requested period or search range")

const Help = `sa2 reports monthly get --student-id ID --period YYYY-MM [--section milestones]
sa2 reports monthly latest --student-id ID [--through YYYY-MM] [--lookback 1-120] [--section milestones]
latest searches backwards from --through (default: current local month), including that month.
The default lookback is 24 months. Search bounds are returned with the result.
sa2 reports monthly milestone update --student-id ID --period YYYY-MM --report-id ID --key KEY --expected-updated-at RFC3339_MILLIS [--status STATUS] [--notes TEXT | --notes-file FILE | --clear-notes] [--confirm]
sa2 reports monthly milestone create --student-id ID --period YYYY-MM --report-id ID --name NAME --expected-updated-at RFC3339_MILLIS --status STATUS [--notes TEXT | --notes-file FILE] [--confirm]
sa2 reports monthly milestone schema
Update and create default to a read-only preview. --confirm submits one conditional write.
Omitted status/notes are preserved; --clear-notes explicitly clears notes.
No commands generate reports or replace entire milestone arrays.
`

type Command struct {
	Action, StudentID, Period, Through, Section                          string
	Lookback                                                             int
	ReportID, Key, Name, ExpectedUpdatedAt, Status, NotesFile, NotesText string
	ClearNotes, Confirm                                                  bool
	Notes                                                                *string
}

var studentIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func Parse(args []string) (Command, error) {
	var cmd Command
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		return cmd, ErrHelp
	}
	if len(args) < 2 || args[0] != "monthly" {
		return cmd, ErrArgument
	}
	if args[1] == "--help" || args[1] == "-h" {
		return cmd, ErrHelp
	}
	cmd.Action = args[1]
	remaining := args[2:]
	if cmd.Action == "milestone" {
		if len(remaining) == 0 || remaining[0] == "--help" {
			return cmd, ErrHelp
		}
		if remaining[0] == "schema" && len(remaining) == 1 {
			cmd.Action = "schema"
			return cmd, nil
		}
		if remaining[0] != "update" && remaining[0] != "create" {
			return cmd, ErrArgument
		}
		cmd.Action = remaining[0]
		remaining = remaining[1:]
	}
	if cmd.Action != "get" && cmd.Action != "latest" && cmd.Action != "update" && cmd.Action != "create" {
		return cmd, ErrArgument
	}
	fs := flag.NewFlagSet("monthly", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cmd.StudentID, "student-id", "", "")
	fs.StringVar(&cmd.Section, "section", "", "")
	if cmd.Action == "get" || cmd.Action == "update" || cmd.Action == "create" {
		fs.StringVar(&cmd.Period, "period", "", "")
	} else {
		fs.StringVar(&cmd.Through, "through", time.Now().Format("2006-01"), "")
		fs.IntVar(&cmd.Lookback, "lookback", 24, "")
	}
	if cmd.Action == "update" || cmd.Action == "create" {
		fs.StringVar(&cmd.ReportID, "report-id", "", "")
		if cmd.Action == "update" {
			fs.StringVar(&cmd.Key, "key", "", "")
			fs.BoolVar(&cmd.ClearNotes, "clear-notes", false, "")
		} else {
			fs.StringVar(&cmd.Name, "name", "", "")
		}
		fs.StringVar(&cmd.ExpectedUpdatedAt, "expected-updated-at", "", "")
		fs.StringVar(&cmd.Status, "status", "", "")
		fs.StringVar(&cmd.NotesText, "notes", "", "")
		fs.StringVar(&cmd.NotesFile, "notes-file", "", "")
		fs.BoolVar(&cmd.Confirm, "confirm", false, "")
	}
	if err := fs.Parse(remaining); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return cmd, ErrHelp
		}
		return cmd, ErrArgument
	}
	if fs.NArg() != 0 || !studentIDPattern.MatchString(cmd.StudentID) || (cmd.Section != "" && cmd.Section != "milestones") {
		return cmd, ErrArgument
	}
	period := cmd.Period
	if cmd.Action == "latest" {
		period = cmd.Through
		if cmd.Lookback < 1 || cmd.Lookback > 120 {
			return cmd, ErrArgument
		}
	}
	date, err := time.Parse("2006-01", period)
	if err != nil || date.Year() < 2000 || date.Year() > 2100 {
		return cmd, ErrArgument
	}
	if cmd.Action == "latest" && date.AddDate(0, -(cmd.Lookback-1), 0).Year() < 2000 {
		return cmd, ErrArgument
	}
	if cmd.Action == "update" || cmd.Action == "create" {
		emptyStatus, notesSet, notesFileSet := false, false, false
		fs.Visit(func(f *flag.Flag) {
			if f.Name == "status" && f.Value.String() == "" {
				emptyStatus = true
			}
			if f.Name == "notes" {
				notesSet = true
			}
			if f.Name == "notes-file" {
				notesFileSet = true
			}
		})
		if emptyStatus {
			return cmd, ErrArgument
		}
		if !studentIDPattern.MatchString(cmd.ReportID) || cmd.Section != "" || (cmd.Status != "" && !validStatus(cmd.Status)) {
			return cmd, ErrArgument
		}
		if cmd.Action == "update" && !milestoneKeyPattern.MatchString(cmd.Key) {
			return cmd, ErrArgument
		}
		if cmd.Action == "create" && (strings.TrimSpace(cmd.Name) != cmd.Name || cmd.Name == "" || len([]rune(cmd.Name)) > 200 || !validStatus(cmd.Status)) {
			return cmd, ErrArgument
		}
		stamp, err := time.Parse("2006-01-02T15:04:05.000Z", cmd.ExpectedUpdatedAt)
		if err != nil || stamp.Format("2006-01-02T15:04:05.000Z") != cmd.ExpectedUpdatedAt {
			return cmd, ErrArgument
		}
		if (notesSet && (notesFileSet || cmd.ClearNotes)) || (notesFileSet && cmd.ClearNotes) {
			return cmd, ErrArgument
		}
		if cmd.Action == "update" && cmd.Status == "" && !notesSet && !notesFileSet && !cmd.ClearNotes {
			return cmd, ErrArgument
		}
		if notesSet {
			if strings.TrimSpace(cmd.NotesText) == "" || !validNotes(cmd.NotesText) {
				return cmd, ErrArgument
			}
			cmd.Notes = &cmd.NotesText
		}
		if cmd.ClearNotes {
			empty := ""
			cmd.Notes = &empty
		}
		if notesFileSet {
			notes, err := readNotes(cmd.NotesFile)
			if err != nil {
				return cmd, ErrArgument
			}
			cmd.Notes = &notes
		}
		cmd.ReportID = strings.ToLower(cmd.ReportID)
	}
	cmd.StudentID = strings.ToLower(cmd.StudentID)
	return cmd, nil
}

type Client struct {
	API    api.Client
	Origin string
	Token  []byte
}

func (c Client) Run(ctx context.Context, cmd Command) (any, error) {
	if cmd.Action == "update" {
		return c.update(ctx, cmd)
	}
	if cmd.Action == "create" {
		return c.create(ctx, cmd)
	}
	period, count := cmd.Period, 1
	if cmd.Action == "latest" {
		period, count = cmd.Through, cmd.Lookback
	}
	through, _ := time.Parse("2006-01", period)
	for i := 0; i < count; i++ {
		date := through.AddDate(0, -i, 0)
		q := url.Values{"year": {strconv.Itoa(date.Year())}, "month": {strconv.Itoa(int(date.Month()))}}
		var report map[string]any
		err := c.API.Business(ctx, c.Origin, c.Token, "GET", "/api/reports/students/"+cmd.StudentID+"/monthly-service-reports", q, nil, nil, &report)
		if err != nil {
			return nil, err
		}
		if report == nil || report["studentId"] != cmd.StudentID || report["reportPeriod"] != date.Format("2006-01") {
			return nil, api.ErrInvalidResponse
		}
		id, exists := report["id"]
		if !exists {
			return nil, api.ErrInvalidResponse
		}
		if id == nil {
			continue
		}
		if value, ok := id.(string); !ok || !studentIDPattern.MatchString(value) {
			return nil, api.ErrInvalidResponse
		}
		if cmd.Section == "milestones" {
			items, ok := report["monthlyMilestonesSnapshot"].([]any)
			if !ok {
				return nil, api.ErrInvalidResponse
			}
			report = map[string]any{"id": id, "studentId": report["studentId"], "reportPeriod": report["reportPeriod"], "updatedAt": report["updatedAt"], "monthlyMilestonesSnapshot": items}
		}
		if cmd.Action == "latest" {
			return map[string]any{"report": report, "search": map[string]any{"through": period, "from": through.AddDate(0, -(count - 1), 0).Format("2006-01"), "checkedMonths": i + 1}}, nil
		}
		return report, nil
	}
	return nil, ErrNotFound
}
