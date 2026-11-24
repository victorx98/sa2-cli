// Package booking implements the sessions CLI protocol.
package booking

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
)

var ErrArgument = errors.New("invalid sessions command")
var ErrHelp = errors.New("sessions help")

const Help = `sa2 sessions find --kind student|mentor --text TEXT
sa2 sessions options --student-id ID --mentor-id ID [--contract-id ID]
sa2 sessions slots --student-id ID --mentor-id ID --from YYYY-MM-DD --to YYYY-MM-DD --timezone ZONE [--duration MIN]
sa2 sessions list [--student-id ID] [--mentor-id ID] [--from YYYY-MM-DD --to YYYY-MM-DD] [--status STATUS] [--page N] [--page-size 1-100]
sa2 sessions get --id ID
sa2 sessions plan create --student-id ID --mentor-id ID --contract-id ID --type-id ID --entitlement Internal|External --start RFC3339 --timezone ZONE --duration MIN --title TEXT [--notes TEXT] [--notes-visibility shared|mentor-only] --out FILE
sa2 sessions plan reschedule --id ID --start RFC3339 --timezone ZONE --duration MIN --out FILE
sa2 sessions plan cancel --id ID --reason TEXT --out FILE
sa2 sessions create|reschedule|cancel --plan FILE --confirm
sa2 sessions operation --request-id ID
sa2 sessions wait --request-id ID [--timeout 60s]
Ranges are half open. List dates are UTC; slots dates use the specified time zone.
`

type Command struct {
	Name, Action string
	Values       map[string]string
	Timeout      time.Duration
}

var idPattern = regexp.MustCompile("(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$")
var validZones = map[string]bool{"America/Los_Angeles": true, "America/Denver": true, "America/Chicago": true, "America/New_York": true, "Europe/London": true, "Asia/Shanghai": true, "Australia/Sydney": true}

func Parse(args []string) (Command, error) {
	cmd := Command{Values: map[string]string{}, Timeout: time.Minute}
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		return cmd, ErrHelp
	}
	cmd.Name = args[0]
	args = args[1:]
	var required, optional string
	switch cmd.Name {
	case "find":
		required = "kind text"
	case "options":
		required = "student-id mentor-id"
		optional = "contract-id"
	case "slots":
		required = "student-id mentor-id from to timezone"
		optional = "duration"
		cmd.Values["duration"] = "60"
	case "list":
		optional = "student-id mentor-id from to status page page-size"
	case "get":
		required = "id"
	case "operation":
		required = "request-id"
	case "wait":
		required = "request-id"
		optional = "timeout"
	case "plan":
		if len(args) == 0 || args[0] == "--help" {
			return cmd, ErrHelp
		}
		cmd.Action = args[0]
		args = args[1:]
		switch cmd.Action {
		case "create":
			required = "student-id mentor-id contract-id type-id entitlement start timezone duration title out"
			optional = "notes notes-visibility"
			cmd.Values["notes-visibility"] = "shared"
		case "reschedule":
			required = "id start timezone duration out"
		case "cancel":
			required = "id reason out"
		default:
			return cmd, ErrArgument
		}
	case "create", "reschedule", "cancel":
		cmd.Action = cmd.Name
		required = "plan"
	default:
		return cmd, ErrArgument
	}
	flags := flag.NewFlagSet(cmd.Name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	params := map[string]*string{}
	for _, k := range strings.Fields(required + " " + optional) {
		params[k] = flags.String(k, cmd.Values[k], "")
	}
	commit := cmd.Name == cmd.Action
	confirm := false
	if commit {
		flags.BoolVar(&confirm, "confirm", false, "")
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return cmd, ErrHelp
		}
		return cmd, ErrArgument
	}
	if flags.NArg() != 0 || commit && !confirm {
		return cmd, ErrArgument
	}
	for k, v := range params {
		cmd.Values[k] = strings.TrimSpace(*v)
	}
	for _, k := range strings.Fields(required) {
		if cmd.Values[k] == "" {
			return cmd, ErrArgument
		}
	}
	for k, v := range cmd.Values {
		if v == "" {
			continue
		}
		switch {
		case k == "id" || strings.HasSuffix(k, "-id"):
			if !idPattern.MatchString(v) {
				return cmd, ErrArgument
			}
		case k == "kind":
			if v != "student" && v != "mentor" {
				return cmd, ErrArgument
			}
		case k == "timezone":
			if !validZones[v] {
				return cmd, ErrArgument
			}
		case k == "entitlement":
			if v != "Internal" && v != "External" {
				return cmd, ErrArgument
			}
		case k == "notes-visibility":
			if v != "shared" && v != "mentor-only" {
				return cmd, ErrArgument
			}
		case k == "duration":
			n, e := strconv.Atoi(v)
			if e != nil || n < 15 || int64(n) > 2147483647 {
				return cmd, ErrArgument
			}
		case k == "page" || k == "page-size":
			n, e := strconv.Atoi(v)
			if e != nil || n < 1 || k == "page-size" && n > 100 {
				return cmd, ErrArgument
			}
		case k == "status":
			switch v {
			case "pending_meeting", "scheduled", "completed", "cancelled", "deleted", "meeting_failed":
			default:
				return cmd, ErrArgument
			}
		case k == "from" || k == "to":
			if _, e := time.Parse(time.DateOnly, v); e != nil {
				return cmd, ErrArgument
			}
		case k == "timeout":
			d, e := time.ParseDuration(v)
			if e != nil || d <= 0 {
				return cmd, ErrArgument
			}
			cmd.Timeout = d
		}
	}
	v := cmd.Values
	if v["from"] != "" && v["to"] != "" {
		f, _ := time.Parse(time.DateOnly, v["from"])
		t, _ := time.Parse(time.DateOnly, v["to"])
		if !t.After(f) || cmd.Name == "slots" && t.Sub(f) > 7*24*time.Hour {
			return cmd, ErrArgument
		}
	}
	if v["start"] != "" {
		t, e := time.Parse(time.RFC3339, v["start"])
		if e != nil {
			return cmd, ErrArgument
		}
		loc, e := time.LoadLocation(v["timezone"])
		if e != nil {
			return cmd, ErrArgument
		}
		_, off := t.Zone()
		_, actual := t.In(loc).Zone()
		if !strings.HasSuffix(v["start"], "Z") && off != actual {
			return cmd, ErrArgument
		}
	}
	return cmd, nil
}
func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	x := hex.EncodeToString(b[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", x[:8], x[8:12], x[12:16], x[16:20], x[20:]), nil
}
